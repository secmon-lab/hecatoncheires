package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// ---- Sentinel errors -------------------------------------------------------

// ErrImportSessionNotFound is returned by ImportUseCase.Get when no
// session matches the given ID or the caller is not the creator.
var ErrImportSessionNotFound = goerr.New("import session not found")

// ErrImportSessionInvalidState is returned by ImportUseCase.Execute when
// the target session is not in pending state.
var ErrImportSessionInvalidState = goerr.New("import session is not in pending state")

// ErrImportValidation is returned by ImportUseCase.Execute when the
// session has unresolved error-severity issues from preview.
var ErrImportValidation = goerr.New("import session has validation errors")

// ErrImportFieldSchemaStale is returned by ImportUseCase.Execute when
// the workspace field schema has changed since createCaseImport. The
// session.Issues will be appended with per-field details.
var ErrImportFieldSchemaStale = goerr.New("workspace field schema has changed since import was created")

// ---- ImportUseCase --------------------------------------------------------

// ImportUseCase owns the "YAML → Case/Action" wizard. It delegates the
// actual Case / Action creation to the existing CaseUseCase /
// ActionUseCase so business rules (history events, private-case
// invariants, Slack-suppression because the Case is in DRAFT, etc.) stay
// in one place.
type ImportUseCase struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	caseUC            *CaseUseCase
	actionUC          *ActionUseCase
}

// NewImportUseCase constructs an ImportUseCase.
func NewImportUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, caseUC *CaseUseCase, actionUC *ActionUseCase) *ImportUseCase {
	return &ImportUseCase{
		repo:              repo,
		workspaceRegistry: registry,
		caseUC:            caseUC,
		actionUC:          actionUC,
	}
}

// ---- YAML wire types ------------------------------------------------------

// importYAML is the strongly-typed shape of the uploaded YAML file.
// Fields use `yaml:""` tags so they can stay close to user-facing
// camelCase / snake_case mixes without affecting Go naming.
type importYAML struct {
	Version int              `yaml:"version"`
	Cases   []importYAMLCase `yaml:"cases"`
}

type importYAMLCase struct {
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	IsPrivate   bool                   `yaml:"isPrivate"`
	AssigneeIDs []string               `yaml:"assigneeIDs"`
	Fields      map[string]interface{} `yaml:"fields"`
	Actions     []importYAMLAction     `yaml:"actions"`
}

type importYAMLAction struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	AssigneeID  string `yaml:"assigneeID"`
	DueDate     string `yaml:"dueDate"`
}

// ---- Create / Get / Execute ----------------------------------------------

// Create parses, validates, and persists a new ImportSession in pending
// state. Caller must be authenticated; CreatorUserID is captured from
// the auth context.
func (uc *ImportUseCase) Create(ctx context.Context, workspaceID, content, originalFileName string) (*model.ImportSession, error) {
	creatorID := creatorFromContext(ctx)
	if creatorID == "" {
		return nil, goerr.New("authenticated user is required for import",
			goerr.V("workspace_id", workspaceID))
	}

	digest := sha256.Sum256([]byte(content))

	snapshot, sessionIssues, schemaHash, parseErr := uc.parseAndNormalize(ctx, workspaceID, content)
	if parseErr != nil {
		// Hard parse failure (YAML invalid). We still want to record the
		// session so the user can see the error at the detail URL, but
		// only if we already have a usable snapshot stub. When the YAML
		// is unparseable we lift the error into a session-level issue
		// and persist an empty snapshot.
		sessionIssues = append(sessionIssues, model.ImportIssue{
			Path:     "",
			Message:  parseErr.Error(),
			Severity: model.ImportIssueError,
		})
		snapshot = model.ImportSnapshot{Version: 0}
	}

	now := time.Now().UTC()
	session := &model.ImportSession{
		ID:            model.NewImportSessionID(),
		WorkspaceID:   workspaceID,
		CreatorUserID: creatorID,
		Status:        model.ImportSessionPending,
		Source: model.ImportSource{
			OriginalFileName: originalFileName,
			ContentDigest:    hex.EncodeToString(digest[:]),
			SizeBytes:        len(content),
		},
		Snapshot:        snapshot,
		Issues:          sessionIssues,
		FieldSchemaHash: schemaHash,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	created, err := uc.repo.Import().Create(ctx, workspaceID, session)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to persist import session",
			goerr.V("workspace_id", workspaceID))
	}
	return created, nil
}

// Get returns the session if it exists AND the caller is the creator.
// Returns ErrImportSessionNotFound otherwise so callers cannot probe
// other users' session IDs.
func (uc *ImportUseCase) Get(ctx context.Context, workspaceID string, id model.ImportSessionID) (*model.ImportSession, error) {
	s, err := uc.repo.Import().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrImportSessionNotFound, "import session not found",
			goerr.V("workspace_id", workspaceID), goerr.V("import_id", id))
	}
	creatorID := creatorFromContext(ctx)
	if creatorID == "" || s.CreatorUserID != creatorID {
		return nil, goerr.Wrap(ErrImportSessionNotFound, "import session not found for this caller",
			goerr.V("workspace_id", workspaceID), goerr.V("import_id", id))
	}
	return s, nil
}

// Execute runs the persisted snapshot against the workspace, creating
// Case (DRAFT) and Action records via the existing usecases. The first
// failure halts the loop; subsequent items become skipped. No rollback
// is performed (DRAFTs are harmless to leave around — they have no
// Slack side effects).
func (uc *ImportUseCase) Execute(ctx context.Context, workspaceID string, id model.ImportSessionID) (*model.ImportSession, error) {
	session, err := uc.Get(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if session.Status != model.ImportSessionPending {
		return nil, goerr.Wrap(ErrImportSessionInvalidState,
			"only pending sessions can be executed",
			goerr.V("import_id", id),
			goerr.V("current_status", string(session.Status)))
	}
	if session.HasErrorIssues() {
		return nil, goerr.Wrap(ErrImportValidation,
			"import session has unresolved error issues",
			goerr.V("import_id", id))
	}

	// Field schema staleness check. We do not recompute the snapshot;
	// instead we attach issues describing precisely which Case/field
	// combinations are no longer valid under the current schema, then
	// refuse to run.
	currentHash := uc.fieldSchemaHash(workspaceID)
	if currentHash != session.FieldSchemaHash {
		uc.recordStaleSchemaIssues(ctx, workspaceID, session)
		session.UpdatedAt = time.Now().UTC()
		if _, updErr := uc.repo.Import().Update(ctx, workspaceID, session); updErr != nil {
			logging.From(ctx).Warn("failed to persist stale-schema issues on import session",
				"error", updErr,
				"import_id", string(id))
		}
		return nil, goerr.Wrap(ErrImportFieldSchemaStale,
			"workspace field schema has changed since import was created",
			goerr.V("import_id", id),
			goerr.V("session_hash", session.FieldSchemaHash),
			goerr.V("current_hash", currentHash))
	}

	// Run every Case independently. A failure on one Case (or one
	// Action) does NOT stop the rest — Import's contract is "try every
	// item, surface the ones that failed". Within a Case the Actions
	// can only attempt if the Case itself was created (no parent → no
	// caseID to attach them to); otherwise the Actions are recorded as
	// skipped, but the loop still moves on to the next Case.
	for ci := range session.Snapshot.Cases {
		c := &session.Snapshot.Cases[ci]

		createdCase, ccErr := uc.caseUC.CreateDraft(
			ctx, workspaceID,
			c.Title, c.Description, c.AssigneeIDs, c.FieldValues, c.IsPrivate,
		)
		if ccErr != nil {
			c.Result = model.ImportCaseResult{
				Status: model.ImportItemFailed,
				Error: &model.ImportIssue{
					Path:     fmt.Sprintf("cases[%d]", c.Index),
					Message:  ccErr.Error(),
					Severity: model.ImportIssueError,
				},
			}
			// Actions cannot run without a parent Case — they are
			// marked skipped, but only for this one Case. The outer
			// loop keeps going.
			for ai := range c.Actions {
				c.Actions[ai].Result = model.ImportActionResult{Status: model.ImportItemSkipped}
			}
			continue
		}
		c.Result = model.ImportCaseResult{
			Status:        model.ImportItemCreated,
			CreatedCaseID: ptrInt64(createdCase.ID),
		}

		// Note: Action records are intentionally NOT created here. The
		// imported Case is a DRAFT, and Actions cannot be edited on
		// DRAFT cases — creating them up front would produce
		// unmodifiable rows attached to a half-finished Case. Any
		// `actions:` block in the YAML is surfaced as a warning at
		// preview time (see normalizeCase) and silently dropped from
		// the persisted snapshot.
	}

	now := time.Now().UTC()
	session.ExecutedAt = &now
	session.UpdatedAt = now
	session.RecomputeCounts()
	if session.FailedCount > 0 {
		session.Status = model.ImportSessionFailed
	} else {
		session.Status = model.ImportSessionApplied
	}

	updated, updErr := uc.repo.Import().Update(ctx, workspaceID, session)
	if updErr != nil {
		return nil, goerr.Wrap(updErr, "failed to update import session after execute",
			goerr.V("import_id", id))
	}
	return updated, nil
}

// ---- helpers --------------------------------------------------------------

func creatorFromContext(ctx context.Context) string {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return ""
	}
	return token.Sub
}

func ptrInt64(v int64) *int64 { return &v }

// parseAndNormalize parses the raw YAML into the wire types, then normalizes
// per-Case field values against the workspace's field schema. Errors detected
// here are returned as ImportIssue values so the user sees them in the
// preview; a non-nil error return means the YAML itself failed to parse.
func (uc *ImportUseCase) parseAndNormalize(ctx context.Context, workspaceID, content string) (model.ImportSnapshot, []model.ImportIssue, string, error) {
	var doc importYAML
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return model.ImportSnapshot{}, nil, uc.fieldSchemaHash(workspaceID), goerr.Wrap(err, "YAML parse failed")
	}

	sessionIssues := []model.ImportIssue{}
	if doc.Version != 1 {
		sessionIssues = append(sessionIssues, model.ImportIssue{
			Path:     "version",
			Message:  fmt.Sprintf("unsupported version %d (only 1 is supported)", doc.Version),
			Severity: model.ImportIssueError,
		})
	}

	validator := uc.fieldValidator(workspaceID)
	schema := uc.fieldSchema(workspaceID)

	// Collect every Slack user ID referenced anywhere in the YAML
	// (assigneeIDs + USER / MULTI_USER field values) and resolve them
	// against the workspace's SlackUser registry in one batch. Each Case
	// then checks its own references against the resolved set so the
	// preview can list every unknown user with the exact YAML path.
	knownUsers := uc.resolveSlackUsers(ctx, collectSlackUserIDs(doc, schema))

	snapshot := model.ImportSnapshot{Version: doc.Version}
	for ci, yc := range doc.Cases {
		snapshot.Cases = append(snapshot.Cases, normalizeCase(ci, yc, validator, schema, knownUsers))
	}

	if len(snapshot.Cases) == 0 && doc.Version == 1 {
		sessionIssues = append(sessionIssues, model.ImportIssue{
			Path:     "cases",
			Message:  "no cases were specified in the file",
			Severity: model.ImportIssueError,
		})
	}

	return snapshot, sessionIssues, uc.fieldSchemaHash(workspaceID), nil
}

func normalizeCase(index int, yc importYAMLCase, validator *model.FieldValidator, schema *config.FieldSchema, knownUsers map[string]bool) model.ImportSnapshotCase {
	out := model.ImportSnapshotCase{
		Index:       index,
		Title:       yc.Title,
		Description: yc.Description,
		IsPrivate:   yc.IsPrivate,
		AssigneeIDs: yc.AssigneeIDs,
		FieldValues: map[string]model.FieldValue{},
		Result:      model.ImportCaseResult{Status: model.ImportItemPending},
	}

	if yc.Title == "" {
		out.Issues = append(out.Issues, model.ImportIssue{
			Path:     fmt.Sprintf("cases[%d].title", index),
			Message:  "title is required",
			Severity: model.ImportIssueError,
		})
	}

	// Field values: route through the workspace validator if available
	// so we reuse the type checks the rest of the codebase already
	// enforces. Then separately enumerate required-field misses against
	// the schema so each missing field surfaces as its own ImportIssue
	// at the precise YAML path (cases[i].fields.<fieldID>) — the bare
	// "required field not provided" error from the validator does not
	// say *which* field is missing, which is exactly the information
	// the YAML author needs to fix the file.
	rawValues := map[string]model.FieldValue{}
	for k, v := range yc.Fields {
		rawValues[k] = model.FieldValue{FieldID: types.FieldID(k), Value: v}
	}
	if validator != nil {
		// Type checks only here: ValidateCaseFieldsPartial validates
		// supplied values and skips the required-field check. We
		// handle required-field misses below at a finer granularity.
		enriched, err := validator.ValidateCaseFieldsPartial(rawValues)
		if err != nil {
			out.Issues = append(out.Issues, model.ImportIssue{
				Path:     fmt.Sprintf("cases[%d].fields", index),
				Message:  err.Error(),
				Severity: model.ImportIssueError,
			})
			out.FieldValues = rawValues
		} else {
			out.FieldValues = enriched
		}
	} else {
		out.FieldValues = rawValues
	}

	// Required-field misses (each missing field as a separate issue with
	// the exact path so the user knows which key to add to the YAML).
	if schema != nil {
		for _, fd := range schema.Fields {
			if !fd.Required {
				continue
			}
			if _, present := rawValues[fd.ID]; present {
				continue
			}
			name := fd.Name
			if name == "" {
				name = fd.ID
			}
			out.Issues = append(out.Issues, model.ImportIssue{
				Path:     fmt.Sprintf("cases[%d].fields.%s", index, fd.ID),
				Message:  fmt.Sprintf("required field %q (%s) is missing", fd.ID, name),
				Severity: model.ImportIssueError,
			})
		}
	}

	// Slack user resolution. Every Slack user referenced in the YAML
	// must exist in the workspace's SlackUser registry; otherwise the
	// imported Case ends up pointing at IDs that the rest of the
	// application (channel invites, assignee dropdowns, agent prompts)
	// cannot resolve. We check both `assigneeIDs` and any USER /
	// MULTI_USER custom field values.
	if knownUsers != nil {
		for ai, uid := range yc.AssigneeIDs {
			if uid == "" {
				continue
			}
			if !knownUsers[uid] {
				out.Issues = append(out.Issues, model.ImportIssue{
					Path:     fmt.Sprintf("cases[%d].assigneeIDs[%d]", index, ai),
					Message:  fmt.Sprintf("unknown Slack user %q (not registered in this workspace)", uid),
					Severity: model.ImportIssueError,
				})
			}
		}
		if schema != nil {
			for _, fd := range schema.Fields {
				v, present := rawValues[fd.ID]
				if !present {
					continue
				}
				switch fd.Type {
				case types.FieldTypeUser:
					if uid, ok := v.Value.(string); ok && uid != "" && !knownUsers[uid] {
						out.Issues = append(out.Issues, model.ImportIssue{
							Path:     fmt.Sprintf("cases[%d].fields.%s", index, fd.ID),
							Message:  fmt.Sprintf("unknown Slack user %q (not registered in this workspace)", uid),
							Severity: model.ImportIssueError,
						})
					}
				case types.FieldTypeMultiUser:
					for j, uid := range extractStringSlice(v.Value) {
						if uid == "" {
							continue
						}
						if !knownUsers[uid] {
							out.Issues = append(out.Issues, model.ImportIssue{
								Path:     fmt.Sprintf("cases[%d].fields.%s[%d]", index, fd.ID, j),
								Message:  fmt.Sprintf("unknown Slack user %q (not registered in this workspace)", uid),
								Severity: model.ImportIssueError,
							})
						}
					}
				}
			}
		}
	}

	// Action records are NOT imported. DRAFT cases do not let users
	// edit attached Actions, so creating Actions here would leave the
	// user with unmodifiable rows. If the YAML carries an `actions:`
	// block we surface a single warning so the author knows it was
	// dropped, but the parsed Actions are NOT placed into Snapshot.
	if len(yc.Actions) > 0 {
		out.Issues = append(out.Issues, model.ImportIssue{
			Path: fmt.Sprintf("cases[%d].actions", index),
			Message: fmt.Sprintf(
				"%d action(s) ignored: actions cannot be imported into DRAFT cases. Submit the draft first, then add actions.",
				len(yc.Actions)),
			Severity: model.ImportIssueWarning,
		})
	}
	return out
}

// extractStringSlice normalises a YAML scalar/array value into a string
// slice. YAML `[a, b]` decodes to []any; we accept both []string and
// []any for safety.
func extractStringSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(x))
		out = append(out, x...)
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// collectSlackUserIDs gathers every Slack user ID referenced anywhere
// in the parsed YAML — assigneeIDs and the values of USER / MULTI_USER
// custom fields. Duplicates are removed. The returned slice is the
// input to a single batch GetByIDs call.
func collectSlackUserIDs(doc importYAML, schema *config.FieldSchema) []string {
	seen := map[string]struct{}{}
	add := func(id string) {
		if id == "" {
			return
		}
		seen[id] = struct{}{}
	}
	userFieldType := map[string]types.FieldType{}
	if schema != nil {
		for _, fd := range schema.Fields {
			if fd.Type == types.FieldTypeUser || fd.Type == types.FieldTypeMultiUser {
				userFieldType[fd.ID] = fd.Type
			}
		}
	}
	for _, yc := range doc.Cases {
		for _, uid := range yc.AssigneeIDs {
			add(uid)
		}
		for k, v := range yc.Fields {
			ft, ok := userFieldType[k]
			if !ok {
				continue
			}
			switch ft {
			case types.FieldTypeUser:
				if s, ok := v.(string); ok {
					add(s)
				}
			case types.FieldTypeMultiUser:
				for _, s := range extractStringSlice(v) {
					add(s)
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// resolveSlackUsers performs a single bulk lookup against
// SlackUserRepository.GetByIDs and returns the set of IDs that were
// successfully resolved. On error (or when no IDs were collected) the
// function returns a non-nil empty map so callers can distinguish
// "validation enabled, nothing resolved" from "validation disabled".
func (uc *ImportUseCase) resolveSlackUsers(ctx context.Context, ids []string) map[string]bool {
	resolved := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return resolved
	}
	sliceIDs := make([]model.SlackUserID, 0, len(ids))
	for _, id := range ids {
		sliceIDs = append(sliceIDs, model.SlackUserID(id))
	}
	found, err := uc.repo.SlackUser().GetByIDs(ctx, sliceIDs)
	if err != nil {
		// Best-effort: log and skip per-user validation. The import
		// still proceeds; assignee correctness will be re-checked at
		// SubmitDraft time. We do not surface a session-level issue
		// here because the failure is server-side, not user-fixable.
		logging.From(ctx).Warn("failed to resolve Slack users during import preview",
			"error", err,
			"id_count", len(ids))
		// Return a sentinel "everything is known" map so we do not
		// mass-flag every assignee as unknown when the lookup itself
		// failed. The map carries every requested ID as true.
		for _, id := range ids {
			resolved[id] = true
		}
		return resolved
	}
	for id := range found {
		resolved[string(id)] = true
	}
	return resolved
}

func (uc *ImportUseCase) fieldValidator(workspaceID string) *model.FieldValidator {
	if uc.workspaceRegistry == nil {
		return nil
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil || entry == nil || entry.FieldSchema == nil {
		return nil
	}
	return model.NewFieldValidator(entry.FieldSchema)
}

// fieldSchema returns the raw FieldSchema so callers can enumerate
// required fields by ID (used to produce per-field "missing required"
// issues at preview time).
func (uc *ImportUseCase) fieldSchema(workspaceID string) *config.FieldSchema {
	if uc.workspaceRegistry == nil {
		return nil
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil || entry == nil {
		return nil
	}
	return entry.FieldSchema
}

// fieldSchemaHash captures the workspace field schema as a JSON-encoded
// digest used to detect drift between preview and execute. The exact bytes
// don't matter; what matters is that the same schema reproduces the same
// hash. We sort fields by ID to keep the hash stable across map iteration
// order (the schema is a slice in config, so we iterate as given but
// re-sort by ID before hashing).
func (uc *ImportUseCase) fieldSchemaHash(workspaceID string) string {
	if uc.workspaceRegistry == nil {
		return ""
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil || entry == nil || entry.FieldSchema == nil {
		return ""
	}
	defs := make([]map[string]any, 0, len(entry.FieldSchema.Fields))
	for _, f := range entry.FieldSchema.Fields {
		opts := make([]string, 0, len(f.Options))
		for _, o := range f.Options {
			opts = append(opts, o.ID)
		}
		sort.Strings(opts)
		defs = append(defs, map[string]any{
			"id":       f.ID,
			"type":     string(f.Type),
			"required": f.Required,
			"options":  opts,
		})
	}
	sort.SliceStable(defs, func(i, j int) bool {
		return defs[i]["id"].(string) < defs[j]["id"].(string)
	})
	b, _ := json.Marshal(defs)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// recordStaleSchemaIssues re-runs field validation under the CURRENT
// schema and appends an error-severity ImportIssue for each Case whose
// field values no longer pass. The session is mutated in place; the
// caller is responsible for persisting it.
func (uc *ImportUseCase) recordStaleSchemaIssues(ctx context.Context, workspaceID string, session *model.ImportSession) {
	validator := uc.fieldValidator(workspaceID)
	schema := uc.fieldSchema(workspaceID)
	header := model.ImportIssue{
		Path:     "",
		Message:  "workspace field schema has changed since this import was created",
		Severity: model.ImportIssueError,
	}
	session.Issues = append(session.Issues, header)

	if validator == nil {
		// Schema disappeared entirely.
		return
	}

	for ci := range session.Snapshot.Cases {
		c := &session.Snapshot.Cases[ci]
		// Re-run type checks under the new schema (partial: required-
		// field misses are enumerated per-field below).
		raw := map[string]model.FieldValue{}
		for k, v := range c.FieldValues {
			raw[k] = model.FieldValue{FieldID: v.FieldID, Value: v.Value}
		}
		if _, err := validator.ValidateCaseFieldsPartial(raw); err != nil {
			c.Issues = append(c.Issues, model.ImportIssue{
				Path:     fmt.Sprintf("cases[%d].fields", c.Index),
				Message:  err.Error(),
				Severity: model.ImportIssueError,
			})
		}
		if schema != nil {
			for _, fd := range schema.Fields {
				if !fd.Required {
					continue
				}
				if _, present := raw[fd.ID]; present {
					continue
				}
				name := fd.Name
				if name == "" {
					name = fd.ID
				}
				c.Issues = append(c.Issues, model.ImportIssue{
					Path:     fmt.Sprintf("cases[%d].fields.%s", c.Index, fd.ID),
					Message:  fmt.Sprintf("required field %q (%s) is missing under the current schema", fd.ID, name),
					Severity: model.ImportIssueError,
				})
			}
		}
	}
}

// Compile-time guards so anyone moving the sentinel definitions keeps the
// errors.Is contract intact.
var _ = errors.Is(ErrImportSessionNotFound, ErrImportSessionNotFound)
