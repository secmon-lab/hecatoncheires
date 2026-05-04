package usecase

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

//go:embed prompts/draft_materializer.md
var draftMaterializerPromptSrc string

var (
	draftMaterializerTmplOnce sync.Once
	draftMaterializerTmpl     *template.Template
	draftMaterializerTmplErr  error
)

func draftMaterializerTemplate() (*template.Template, error) {
	draftMaterializerTmplOnce.Do(func() {
		draftMaterializerTmpl, draftMaterializerTmplErr = template.New("draft_materializer").
			Parse(draftMaterializerPromptSrc)
	})
	return draftMaterializerTmpl, draftMaterializerTmplErr
}

// draftMaterializerPromptInput is the typed input for the prompt template.
type draftMaterializerPromptInput struct {
	Now         string // current time, ISO-8601 UTC, for date-field grounding
	Workspace   draftMaterializerPromptWorkspace
	MentionText string
	Messages    []draftMaterializerPromptMessage
	Fields      []draftMaterializerPromptField
}

type draftMaterializerPromptWorkspace struct {
	ID               string
	Name             string
	Description      string
	EstimationReason string
	OtherCandidates  []draftMaterializerPromptWorkspaceRef
}

type draftMaterializerPromptWorkspaceRef struct {
	Name        string
	Description string
}

type draftMaterializerPromptMessage struct {
	TS     string
	UserID string
	Text   string
}

type draftMaterializerPromptField struct {
	ID          string
	Name        string
	Type        types.FieldType
	Required    bool
	Description string
	Options     []string
}

// MaterializeContext gives the materializer the per-call workspace context
// needed to render the prompt. Callers populate it with the estimated WS,
// the reason that WS was chosen, and other candidates the user could switch to.
type MaterializeContext struct {
	Workspace        *model.WorkspaceEntry
	EstimationReason string
	OtherCandidates  []*model.WorkspaceEntry
}

// DraftMaterializer turns a workspace-agnostic draft (raw Slack messages plus
// the user's mention text) into a workspace-specific Case payload (Title,
// Description, and every custom field defined in the workspace's FieldSchema).
//
// LLM is mandatory; any failure is propagated to the caller.
type DraftMaterializer struct {
	llm gollem.LLMClient
}

// NewDraftMaterializer constructs a Materializer.
func NewDraftMaterializer(llm gollem.LLMClient) *DraftMaterializer {
	return &DraftMaterializer{llm: llm}
}

// materializeMaxAttempts is the total number of LLM attempts (initial + 2 retries).
const materializeMaxAttempts = 3

// formatSlackTSAsISO converts a Slack message TS (epoch float in a string,
// e.g. "1777725124.249169") to an ISO-8601 UTC timestamp. Falls back to the
// raw string when the input doesn't parse, since the LLM should still see
// _something_ rather than an empty bracket.
func formatSlackTSAsISO(ts string) string {
	if ts == "" {
		return ""
	}
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return ts
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC().Format(time.RFC3339)
}

// Materialize calls the LLM to produce a WorkspaceMaterialization for the
// given draft and target workspace context. Any error along the path
// (LLM transport, empty response, JSON parse, fatal validation) triggers a
// retry up to materializeMaxAttempts total. Once exhausted the last error
// is returned to the caller, which is expected to surface it to the user.
func (m *DraftMaterializer) Materialize(
	ctx context.Context,
	draft *model.CaseDraft,
	mctx MaterializeContext,
) (*model.WorkspaceMaterialization, error) {
	if draft == nil {
		return nil, goerr.New("draft is nil")
	}
	if mctx.Workspace == nil || mctx.Workspace.FieldSchema == nil {
		return nil, goerr.New("MaterializeContext.Workspace and its FieldSchema are required")
	}
	if m.llm == nil {
		return nil, goerr.New("LLM client is nil")
	}

	logger := logging.From(ctx)
	var lastErr error
	for attempt := 1; attempt <= materializeMaxAttempts; attempt++ {
		mat, err := m.materializeOnce(ctx, draft, mctx)
		if err == nil {
			if attempt > 1 {
				logger.Info("materializer succeeded after retry",
					"attempt", attempt,
					"workspace_id", mctx.Workspace.Workspace.ID,
				)
			}
			return mat, nil
		}
		lastErr = err
		if attempt < materializeMaxAttempts {
			logger.Warn("materializer attempt failed; retrying",
				"attempt", attempt,
				"max_attempts", materializeMaxAttempts,
				"workspace_id", mctx.Workspace.Workspace.ID,
				"error", err.Error(),
			)
		}
	}
	return nil, goerr.Wrap(lastErr, "materializer exhausted retries",
		goerr.V("attempts", materializeMaxAttempts))
}

// materializeOnce performs a single LLM call + parse + validation pass.
func (m *DraftMaterializer) materializeOnce(
	ctx context.Context,
	draft *model.CaseDraft,
	mctx MaterializeContext,
) (*model.WorkspaceMaterialization, error) {
	schema := mctx.Workspace.FieldSchema

	prompt, err := buildMaterializerPrompt(draft, mctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to render materializer prompt")
	}
	respSchema := buildMaterializerResponseSchema(schema)

	session, err := m.llm.NewSession(ctx,
		gollem.WithSessionContentType(gollem.ContentTypeJSON),
		gollem.WithSessionResponseSchema(respSchema),
	)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create LLM session for materializer")
	}

	resp, err := session.Generate(ctx, []gollem.Input{gollem.Text(prompt)})
	if err != nil {
		return nil, goerr.Wrap(err, "LLM Generate failed for materializer")
	}
	if len(resp.Texts) == 0 {
		return nil, goerr.New("LLM returned no content for materializer")
	}

	parsed, err := parseMaterializerResponse(ctx, resp.Texts[0], schema)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to parse materializer response",
			goerr.V("raw", resp.Texts[0]))
	}

	filtered, err := parsed.FilterToValid(schema)
	if err != nil {
		return nil, goerr.Wrap(err, "materialization failed validation")
	}
	filtered.GeneratedAt = time.Now().UTC()
	return filtered, nil
}

// --- prompt building ---

func buildMaterializerPrompt(draft *model.CaseDraft, mctx MaterializeContext) (string, error) {
	tmpl, err := draftMaterializerTemplate()
	if err != nil {
		return "", goerr.Wrap(err, "failed to parse draft_materializer template")
	}

	messages := make([]draftMaterializerPromptMessage, 0, len(draft.RawMessages))
	for _, msg := range draft.RawMessages {
		messages = append(messages, draftMaterializerPromptMessage{
			TS:     formatSlackTSAsISO(msg.TS),
			UserID: msg.UserID,
			Text:   msg.Text,
		})
	}

	schema := mctx.Workspace.FieldSchema
	fields := make([]draftMaterializerPromptField, 0, len(schema.Fields))
	for _, fd := range schema.Fields {
		opts := make([]string, 0, len(fd.Options))
		for _, opt := range fd.Options {
			opts = append(opts, opt.ID)
		}
		fields = append(fields, draftMaterializerPromptField{
			ID:          fd.ID,
			Name:        fd.Name,
			Type:        fd.Type,
			Required:    fd.Required,
			Description: fd.Description,
			Options:     opts,
		})
	}

	others := make([]draftMaterializerPromptWorkspaceRef, 0, len(mctx.OtherCandidates))
	for _, w := range mctx.OtherCandidates {
		if w == nil || w.Workspace.ID == mctx.Workspace.Workspace.ID {
			continue
		}
		others = append(others, draftMaterializerPromptWorkspaceRef{
			Name:        w.Workspace.Name,
			Description: w.Workspace.Description,
		})
	}

	input := draftMaterializerPromptInput{
		Now: time.Now().UTC().Format(time.RFC3339),
		Workspace: draftMaterializerPromptWorkspace{
			ID:               mctx.Workspace.Workspace.ID,
			Name:             mctx.Workspace.Workspace.Name,
			Description:      mctx.Workspace.Workspace.Description,
			EstimationReason: mctx.EstimationReason,
			OtherCandidates:  others,
		},
		MentionText: strings.TrimSpace(draft.MentionText),
		Messages:    messages,
		Fields:      fields,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, input); err != nil {
		return "", goerr.Wrap(err, "failed to execute draft_materializer template")
	}
	return buf.String(), nil
}

func buildMaterializerResponseSchema(schema *config.FieldSchema) *gollem.Parameter {
	customProps := make(map[string]*gollem.Parameter, len(schema.Fields))
	for _, fd := range schema.Fields {
		customProps[fd.ID] = fieldParameterFor(fd)
	}

	return &gollem.Parameter{
		Title:       "CaseDraftMaterialization",
		Description: "AI-generated Case payload for a specific workspace's FieldSchema",
		Type:        gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "Short single-line title for the Case",
				Required:    true,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "Concise summary of the case in plain text or markdown",
				Required:    true,
			},
			"custom_fields": {
				Type:        gollem.TypeObject,
				Description: "Object whose keys are custom field IDs. Omit fields you cannot confidently fill.",
				Properties:  customProps,
			},
		},
	}
}

func fieldParameterFor(fd config.FieldDefinition) *gollem.Parameter {
	desc := fd.Description
	if fd.Name != "" {
		if desc == "" {
			desc = fd.Name
		} else {
			desc = fd.Name + ": " + desc
		}
	}
	switch fd.Type {
	case types.FieldTypeText, types.FieldTypeURL, types.FieldTypeUser, types.FieldTypeDate:
		return &gollem.Parameter{Type: gollem.TypeString, Description: desc}
	case types.FieldTypeNumber:
		return &gollem.Parameter{Type: gollem.TypeNumber, Description: desc}
	case types.FieldTypeSelect:
		enum := make([]string, 0, len(fd.Options))
		for _, opt := range fd.Options {
			enum = append(enum, opt.ID)
		}
		return &gollem.Parameter{Type: gollem.TypeString, Description: desc, Enum: enum}
	case types.FieldTypeMultiSelect:
		enum := make([]string, 0, len(fd.Options))
		for _, opt := range fd.Options {
			enum = append(enum, opt.ID)
		}
		return &gollem.Parameter{
			Type:        gollem.TypeArray,
			Description: desc,
			Items:       &gollem.Parameter{Type: gollem.TypeString, Enum: enum},
		}
	case types.FieldTypeMultiUser:
		return &gollem.Parameter{
			Type:        gollem.TypeArray,
			Description: desc,
			Items:       &gollem.Parameter{Type: gollem.TypeString},
		}
	default:
		return &gollem.Parameter{Type: gollem.TypeString, Description: desc}
	}
}

// --- response parsing ---

type materializerResponseDTO struct {
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	CustomFields map[string]any `json:"custom_fields"`
}

func parseMaterializerResponse(ctx context.Context, raw string, schema *config.FieldSchema) (*model.WorkspaceMaterialization, error) {
	var dto materializerResponseDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal materializer JSON")
	}

	mat := &model.WorkspaceMaterialization{
		Title:             strings.TrimSpace(dto.Title),
		Description:       strings.TrimSpace(dto.Description),
		CustomFieldValues: map[string]model.FieldValue{},
	}

	defByID := make(map[string]config.FieldDefinition, len(schema.Fields))
	for _, fd := range schema.Fields {
		defByID[fd.ID] = fd
	}

	for fieldID, raw := range dto.CustomFields {
		fd, ok := defByID[fieldID]
		if !ok {
			// Field hallucinated outside schema — drop silently.
			continue
		}
		coerced, ok := coerceFieldValue(raw, fd.Type)
		if !ok {
			// Type mismatch on a single field is non-fatal: log and skip so the
			// human can fill it in via Edit modal.
			errutil.Handle(ctx, goerr.New("AI returned a value of unexpected type for field",
				goerr.V("field_id", fieldID),
				goerr.V("expected_type", fd.Type),
				goerr.V("raw_value", raw),
			), "materializer field coercion failed; skipping field")
			continue
		}
		mat.CustomFieldValues[fieldID] = model.FieldValue{
			FieldID: types.FieldID(fieldID),
			Type:    fd.Type,
			Value:   coerced,
		}
	}

	if mat.Title == "" && mat.Description == "" && len(mat.CustomFieldValues) == 0 {
		logging.From(ctx).Warn("materializer produced an entirely empty payload")
	}
	return mat, nil
}

// coerceFieldValue normalizes a JSON-decoded any value to the canonical Go
// representation expected for the given FieldType. Returns ok=false on any
// type mismatch (e.g., AI returned a boolean for a number field).
func coerceFieldValue(v any, t types.FieldType) (any, bool) {
	switch t {
	case types.FieldTypeText, types.FieldTypeURL, types.FieldTypeUser, types.FieldTypeDate, types.FieldTypeSelect:
		s, ok := v.(string)
		return s, ok
	case types.FieldTypeNumber:
		switch n := v.(type) {
		case float64:
			return n, true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		default:
			return nil, false
		}
	case types.FieldTypeMultiSelect, types.FieldTypeMultiUser:
		arr, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return v, true
	}
}
