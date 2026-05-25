package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// importResolveContext bundles everything the converter needs to expand
// raw IDs into human-readable names for the GraphQL view:
//   - fieldByID maps a workspace FieldDefinition.ID to its definition
//     so SELECT / MULTI_SELECT values can be displayed as option names
//   - optionNameByID is a flattened lookup for select-option IDs across
//     all fields in the workspace
//   - userByID resolves Slack user IDs to their domain SlackUser record
//     so assignees and USER / MULTI_USER fields show display names
type importResolveContext struct {
	fieldByID      map[string]*config.FieldDefinition
	optionNameByID map[string]map[string]string // fieldID -> optionID -> optionName
	userByID       map[string]*model.SlackUser
}

func newImportResolveContext() *importResolveContext {
	return &importResolveContext{
		fieldByID:      map[string]*config.FieldDefinition{},
		optionNameByID: map[string]map[string]string{},
		userByID:       map[string]*model.SlackUser{},
	}
}

// buildImportResolveContext gathers every Slack user ID referenced by
// the given session (assignees + USER / MULTI_USER fields) and resolves
// them in one batch. It also indexes the workspace's field schema.
// Failures are logged-and-skipped: a missing SlackUser simply means the
// frontend will see the raw ID, which is strictly better than blocking
// the whole detail page.
func buildImportResolveContext(
	ctx context.Context,
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	session *model.ImportSession,
) *importResolveContext {
	rc := newImportResolveContext()

	// Workspace field schema (id -> definition + option-name lookup).
	if registry != nil {
		if entry, err := registry.Get(session.WorkspaceID); err == nil && entry != nil && entry.FieldSchema != nil {
			for i := range entry.FieldSchema.Fields {
				fd := &entry.FieldSchema.Fields[i]
				rc.fieldByID[fd.ID] = fd
				if len(fd.Options) > 0 {
					m := make(map[string]string, len(fd.Options))
					for _, opt := range fd.Options {
						name := opt.Name
						if name == "" {
							name = opt.ID
						}
						m[opt.ID] = name
					}
					rc.optionNameByID[fd.ID] = m
				}
			}
		}
	}

	// Collect every Slack user ID we'd want to resolve, batch-fetch in
	// one shot, and stash the result. Unresolved IDs are simply absent
	// from the map; the caller's display helpers fall back to the ID.
	ids := collectImportSlackUserIDs(session, rc.fieldByID)
	if len(ids) > 0 && repo != nil {
		uids := make([]model.SlackUserID, 0, len(ids))
		for _, id := range ids {
			uids = append(uids, model.SlackUserID(id))
		}
		if found, err := repo.SlackUser().GetByIDs(ctx, uids); err == nil {
			for id, u := range found {
				rc.userByID[string(id)] = u
			}
		}
	}
	return rc
}

func collectImportSlackUserIDs(s *model.ImportSession, fieldByID map[string]*config.FieldDefinition) []string {
	seen := map[string]struct{}{}
	add := func(id string) {
		if id == "" {
			return
		}
		seen[id] = struct{}{}
	}
	for _, c := range s.Snapshot.Cases {
		for _, uid := range c.AssigneeIDs {
			add(uid)
		}
		for k, v := range c.FieldValues {
			fd, ok := fieldByID[k]
			if !ok {
				continue
			}
			switch fd.Type {
			case types.FieldTypeUser:
				if s, ok := v.Value.(string); ok {
					add(s)
				}
			case types.FieldTypeMultiUser:
				switch xs := v.Value.(type) {
				case []string:
					for _, s := range xs {
						add(s)
					}
				case []any:
					for _, e := range xs {
						if s, ok := e.(string); ok {
							add(s)
						}
					}
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

func (rc *importResolveContext) displayUser(id string) string {
	if u, ok := rc.userByID[id]; ok {
		if u.RealName != "" {
			return u.RealName
		}
		if u.Name != "" {
			return u.Name
		}
	}
	return id
}

func (rc *importResolveContext) toGraphQLSlackUsers(ids []string) []*graphql1.SlackUser {
	out := make([]*graphql1.SlackUser, 0, len(ids))
	for _, id := range ids {
		if u, ok := rc.userByID[id]; ok {
			out = append(out, importToGraphQLSlackUser(u))
		}
	}
	return out
}

func importToGraphQLSlackUser(u *model.SlackUser) *graphql1.SlackUser {
	var imageURLPtr *string
	if u.ImageURL != "" {
		s := u.ImageURL
		imageURLPtr = &s
	}
	return &graphql1.SlackUser{
		ID:       string(u.ID),
		Name:     u.Name,
		RealName: u.RealName,
		ImageURL: imageURLPtr,
	}
}

// toGraphQLImportSession converts a domain ImportSession to its GraphQL
// counterpart. The converter is strict about non-null list fields: every
// list returned here is a non-nil slice (possibly empty) so the gqlgen
// resolver never has to special-case nil.
//
// `createdCase` / `createdAction` are left nil here; they are populated
// by the dataloader / field resolver that hydrates Case / Action lookups
// from the per-Case CreatedCaseID / per-Action CreatedActionID values.
func toGraphQLImportSession(ctx context.Context, repo interfaces.Repository, registry *model.WorkspaceRegistry, s *model.ImportSession) *graphql1.ImportSession {
	if s == nil {
		return nil
	}
	rc := buildImportResolveContext(ctx, repo, registry, s)
	return &graphql1.ImportSession{
		ID:              s.ID.String(),
		WorkspaceID:     s.WorkspaceID,
		CreatorUserID:   s.CreatorUserID,
		Status:          toGraphQLImportSessionStatus(s.Status),
		Source:          toGraphQLImportSource(s.Source),
		Snapshot:        toGraphQLImportSnapshotWithCtx(s.Snapshot, rc),
		Issues:          toGraphQLImportIssues(s.Issues),
		Valid:           s.Valid(),
		FieldSchemaHash: s.FieldSchemaHash,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		ExecutedAt:      s.ExecutedAt,
		CreatedCount:    s.CreatedCount,
		FailedCount:     s.FailedCount,
		SkippedCount:    s.SkippedCount,
	}
}

func toGraphQLImportSessionStatus(s model.ImportSessionStatus) graphql1.ImportSessionStatus {
	switch s {
	case model.ImportSessionApplied:
		return graphql1.ImportSessionStatusApplied
	case model.ImportSessionFailed:
		return graphql1.ImportSessionStatusFailed
	default:
		return graphql1.ImportSessionStatusPending
	}
}

func toGraphQLImportItemStatus(s model.ImportItemResultStatus) graphql1.ImportItemResultStatus {
	switch s {
	case model.ImportItemCreated:
		return graphql1.ImportItemResultStatusCreated
	case model.ImportItemFailed:
		return graphql1.ImportItemResultStatusFailed
	case model.ImportItemSkipped:
		return graphql1.ImportItemResultStatusSkipped
	default:
		return graphql1.ImportItemResultStatusPending
	}
}

func toGraphQLImportIssueSeverity(s model.ImportIssueSeverity) graphql1.ImportIssueSeverity {
	if s == model.ImportIssueError {
		return graphql1.ImportIssueSeverityError
	}
	return graphql1.ImportIssueSeverityWarning
}

func toGraphQLImportSource(src model.ImportSource) *graphql1.ImportSource {
	return &graphql1.ImportSource{
		OriginalFileName: src.OriginalFileName,
		SizeBytes:        src.SizeBytes,
	}
}

func toGraphQLImportIssues(issues []model.ImportIssue) []*graphql1.ImportIssue {
	out := make([]*graphql1.ImportIssue, 0, len(issues))
	for _, i := range issues {
		out = append(out, &graphql1.ImportIssue{
			Path:     i.Path,
			Message:  i.Message,
			Severity: toGraphQLImportIssueSeverity(i.Severity),
		})
	}
	return out
}

func toGraphQLImportSnapshotWithCtx(sn model.ImportSnapshot, rc *importResolveContext) *graphql1.ImportSnapshot {
	cases := make([]*graphql1.ImportSnapshotCase, 0, len(sn.Cases))
	for i := range sn.Cases {
		cases = append(cases, toGraphQLImportSnapshotCase(sn.Cases[i], rc))
	}
	return &graphql1.ImportSnapshot{
		Version: sn.Version,
		Cases:   cases,
	}
}

func toGraphQLImportSnapshotCase(c model.ImportSnapshotCase, rc *importResolveContext) *graphql1.ImportSnapshotCase {
	assigneeIDs := c.AssigneeIDs
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}
	actions := make([]*graphql1.ImportSnapshotAction, 0, len(c.Actions))
	for i := range c.Actions {
		actions = append(actions, toGraphQLImportSnapshotAction(c.Actions[i]))
	}
	var desc *string
	if c.Description != "" {
		d := c.Description
		desc = &d
	}
	return &graphql1.ImportSnapshotCase{
		Index:       c.Index,
		Title:       c.Title,
		Description: desc,
		IsPrivate:   c.IsPrivate,
		AssigneeIDs: assigneeIDs,
		Assignees:   rc.toGraphQLSlackUsers(assigneeIDs),
		Fields:      toGraphQLImportSnapshotFields(c.FieldValues, rc),
		Actions:     actions,
		Issues:      toGraphQLImportIssues(c.Issues),
		Result:      toGraphQLImportCaseResult(c.Result),
	}
}

func toGraphQLImportSnapshotAction(a model.ImportSnapshotAction) *graphql1.ImportSnapshotAction {
	var desc, assignee *string
	if a.Description != "" {
		d := a.Description
		desc = &d
	}
	if a.AssigneeID != "" {
		x := a.AssigneeID
		assignee = &x
	}
	return &graphql1.ImportSnapshotAction{
		Index:       a.Index,
		Title:       a.Title,
		Description: desc,
		AssigneeID:  assignee,
		DueDate:     a.DueDate,
		Issues:      toGraphQLImportIssues(a.Issues),
		Result:      toGraphQLImportActionResult(a.Result),
	}
}

func toGraphQLImportCaseResult(r model.ImportCaseResult) *graphql1.ImportCaseResult {
	out := &graphql1.ImportCaseResult{
		Status: toGraphQLImportItemStatus(r.Status),
	}
	if r.CreatedCaseID != nil {
		v := int(*r.CreatedCaseID)
		out.CreatedCaseID = &v
	}
	if r.Error != nil {
		out.Error = &graphql1.ImportIssue{
			Path:     r.Error.Path,
			Message:  r.Error.Message,
			Severity: toGraphQLImportIssueSeverity(r.Error.Severity),
		}
	}
	return out
}

func toGraphQLImportActionResult(r model.ImportActionResult) *graphql1.ImportActionResult {
	out := &graphql1.ImportActionResult{
		Status: toGraphQLImportItemStatus(r.Status),
	}
	if r.CreatedActionID != nil {
		v := int(*r.CreatedActionID)
		out.CreatedActionID = &v
	}
	if r.Error != nil {
		out.Error = &graphql1.ImportIssue{
			Path:     r.Error.Path,
			Message:  r.Error.Message,
			Severity: toGraphQLImportIssueSeverity(r.Error.Severity),
		}
	}
	return out
}

// toGraphQLImportSnapshotFields renders a Case's field values into the
// flat key/display list shown in the UI's preview. The Display string
// is resolved against the workspace metadata bundled in `rc`:
//
//   - SELECT / MULTI_SELECT values are looked up against the field's
//     options list so the user sees option names, not option IDs.
//   - USER / MULTI_USER values are looked up against the workspace's
//     SlackUser registry so the user sees a real display name, not a
//     "Uxxxxxxx" Slack ID.
//
// Anything else (TEXT, NUMBER, DATE, URL, unknown types) falls through
// to a generic stringify of the raw value.
func toGraphQLImportSnapshotFields(fv map[string]model.FieldValue, rc *importResolveContext) []*graphql1.ImportSnapshotField {
	if len(fv) == 0 {
		return []*graphql1.ImportSnapshotField{}
	}
	keys := make([]string, 0, len(fv))
	for k := range fv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*graphql1.ImportSnapshotField, 0, len(fv))
	for _, k := range keys {
		out = append(out, &graphql1.ImportSnapshotField{
			Key:     k,
			Display: resolveFieldDisplay(k, fv[k].Value, rc),
		})
	}
	return out
}

// resolveFieldDisplay turns the raw FieldValue.Value into the string
// shown in the import preview, using the workspace field schema /
// SlackUser registry to pretty-print IDs into human-readable names.
func resolveFieldDisplay(fieldID string, v any, rc *importResolveContext) string {
	if rc != nil {
		if fd, ok := rc.fieldByID[fieldID]; ok {
			switch fd.Type {
			case types.FieldTypeSelect:
				if s, ok := v.(string); ok {
					if optMap, ok := rc.optionNameByID[fieldID]; ok {
						if name, ok := optMap[s]; ok {
							return name
						}
					}
					return s
				}
			case types.FieldTypeMultiSelect:
				ids := stringSliceFromValue(v)
				if len(ids) == 0 {
					return ""
				}
				optMap := rc.optionNameByID[fieldID]
				out := make([]string, 0, len(ids))
				for _, id := range ids {
					if optMap != nil {
						if name, ok := optMap[id]; ok {
							out = append(out, name)
							continue
						}
					}
					out = append(out, id)
				}
				return strings.Join(out, ", ")
			case types.FieldTypeUser:
				if s, ok := v.(string); ok {
					return rc.displayUser(s)
				}
			case types.FieldTypeMultiUser:
				ids := stringSliceFromValue(v)
				if len(ids) == 0 {
					return ""
				}
				out := make([]string, 0, len(ids))
				for _, id := range ids {
					out = append(out, rc.displayUser(id))
				}
				return strings.Join(out, ", ")
			}
		}
	}
	return stringifyFieldValue(v)
}

func stringSliceFromValue(v any) []string {
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
	}
	return nil
}

func stringifyFieldValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []string:
		return strings.Join(x, ", ")
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, stringifyFieldValue(e))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}
