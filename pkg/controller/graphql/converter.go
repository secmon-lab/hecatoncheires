package graphql

import (
	"context"
	"sort"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// toGraphQLSlackMessage converts a domain slack.Message to its GraphQL view.
func toGraphQLSlackMessage(m *slack.Message) *graphql1.SlackMessage {
	threadTS := m.ThreadTS()
	var threadTSPtr *string
	if threadTS != "" {
		threadTSPtr = &threadTS
	}
	files := make([]*graphql1.SlackFile, len(m.Files()))
	for j, f := range m.Files() {
		thumbURL := f.ThumbURL()
		var thumbURLPtr *string
		if thumbURL != "" {
			thumbURLPtr = &thumbURL
		}
		files[j] = &graphql1.SlackFile{
			ID:         f.ID(),
			Name:       f.Name(),
			Mimetype:   f.Mimetype(),
			Filetype:   f.Filetype(),
			Size:       f.Size(),
			URLPrivate: f.URLPrivate(),
			Permalink:  f.Permalink(),
			ThumbURL:   thumbURLPtr,
		}
	}
	return &graphql1.SlackMessage{
		ID:        m.ID(),
		ChannelID: m.ChannelID(),
		ThreadTs:  threadTSPtr,
		TeamID:    m.TeamID(),
		UserID:    m.UserID(),
		UserName:  m.UserName(),
		Text:      m.Text(),
		Files:     files,
		CreatedAt: m.CreatedAt(),
	}
}

// toGraphQLActionEvent converts a domain ActionEvent to its GraphQL view.
// The Actor sub-field is left nil here; the resolver fills it via the
// SlackUser dataloader to share the per-request batching layer.
func toGraphQLActionEvent(e *model.ActionEvent) *graphql1.ActionEvent {
	return &graphql1.ActionEvent{
		ID:        e.ID,
		ActionID:  int(e.ActionID),
		Kind:      graphql1.ActionEventKind(e.Kind),
		ActorID:   e.ActorID,
		OldValue:  e.OldValue,
		NewValue:  e.NewValue,
		CreatedAt: e.CreatedAt,
	}
}

// toGraphQLActionStep converts a domain ActionStep to its GraphQL view.
// Done is derived from DoneAt to keep the WebUI's archived/archivedAt
// pattern uniform across the schema (single source of truth on the model
// side, two convenience views on the wire).
func toGraphQLActionStep(s *model.ActionStep) *graphql1.ActionStep {
	var doneBy *string
	if s.DoneBy != "" {
		v := s.DoneBy
		doneBy = &v
	}
	return &graphql1.ActionStep{
		ID:        s.ID,
		ActionID:  int(s.ActionID),
		Title:     s.Title,
		Done:      s.IsDone(),
		DoneAt:    s.DoneAt,
		DoneBy:    doneBy,
		CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// toGraphQLCase converts a domain Case to GraphQL Case
func toGraphQLCase(c *model.Case, workspaceID string) *graphql1.Case {
	// Ensure non-null list fields are never nil (schema: [String!]!)
	assigneeIDs := c.AssigneeIDs
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}

	channelUserIDs := c.ChannelUserIDs
	if channelUserIDs == nil {
		channelUserIDs = []string{}
	}

	var reporterID *string
	if c.ReporterID != "" {
		reporterID = &c.ReporterID
	}

	agentSourceIDs := make([]string, 0, len(c.AgentSourceIDs))
	for _, id := range c.AgentSourceIDs {
		agentSourceIDs = append(agentSourceIDs, string(id))
	}

	// AccessDenied gates every private-case-sensitive field. We mask
	// AgentAdditionalPrompt here at the converter layer (rather than
	// behind a custom resolver) because the GraphQL Case.agentAddi-
	// tionalPrompt field's Go type matches the model exactly, which
	// makes gqlgen emit a direct struct-field read in the generated
	// resolver — a custom resolver method on Case would be silently
	// ignored. Other restricted fields (e.g. agentSources / channelUsers
	// / messages) live behind list-shape resolvers that gqlgen always
	// routes through, so they keep their per-field AccessDenied check.
	agentPrompt := c.AgentAdditionalPrompt
	if c.AccessDenied {
		agentPrompt = ""
	}

	var slackThreadTS *string
	if c.SlackThreadTS != "" {
		v := c.SlackThreadTS
		slackThreadTS = &v
	}
	var boardStatus *string
	if c.BoardStatus != "" {
		v := c.BoardStatus
		boardStatus = &v
	}

	return &graphql1.Case{
		ID:                    int(c.ID),
		WorkspaceID:           workspaceID,
		Title:                 c.Title,
		Description:           c.Description,
		Status:                c.Status.Normalize(),
		IsPrivate:             c.IsPrivate,
		IsTest:                c.IsTest,
		AccessDenied:          c.AccessDenied,
		ChannelUserIDs:        channelUserIDs,
		ReporterID:            reporterID,
		AssigneeIDs:           assigneeIDs,
		SlackChannelID:        &c.SlackChannelID,
		SlackThreadTS:         slackThreadTS,
		IsThreadBound:         c.IsThreadBound(),
		BoardStatus:           boardStatus,
		Fields:                toGraphQLFieldValues(c.FieldValues),
		AgentAdditionalPrompt: agentPrompt,
		AgentSourceIDs:        agentSourceIDs,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}
}

// toActionConfig converts a generic status set into the GraphQL ActionConfig
// shape. Reused for both the action status config and the thread-mode case
// status config (caseStatusConfig), which share the same wire type.
func toActionConfig(set *model.ActionStatusSet) *graphql1.ActionConfig {
	statusDefs := set.Statuses()
	gqlStatuses := make([]*graphql1.ActionStatusDefinition, 0, len(statusDefs))
	for _, def := range statusDefs {
		var description, color, emoji *string
		if def.Description != "" {
			v := def.Description
			description = &v
		}
		if def.Color != "" {
			v := def.Color
			color = &v
		}
		if def.Emoji != "" {
			v := def.Emoji
			emoji = &v
		}
		gqlStatuses = append(gqlStatuses, &graphql1.ActionStatusDefinition{
			ID:          def.ID,
			Name:        def.Name,
			Description: description,
			Color:       color,
			Emoji:       emoji,
		})
	}
	closedIDs := set.ClosedIDs()
	if closedIDs == nil {
		closedIDs = []string{}
	}
	return &graphql1.ActionConfig{
		Initial:  set.InitialID(),
		Closed:   closedIDs,
		Statuses: gqlStatuses,
	}
}

// toGraphQLAction converts a domain Action to GraphQL Action
func toGraphQLAction(a *model.Action, workspaceID string) *graphql1.Action {
	slackMessageTS := ""
	if a.SlackMessageTS != "" {
		slackMessageTS = a.SlackMessageTS
	}

	var assigneeID *string
	if a.AssigneeID != "" {
		s := a.AssigneeID
		assigneeID = &s
	}

	return &graphql1.Action{
		ID:             int(a.ID),
		WorkspaceID:    workspaceID,
		CaseID:         int(a.CaseID),
		Title:          a.Title,
		Description:    a.Description,
		AssigneeID:     assigneeID,
		SlackMessageTs: &slackMessageTS,
		Status:         string(a.Status),
		DueDate:        a.DueDate,
		Archived:       a.IsArchived(),
		ArchivedAt:     a.ArchivedAt,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// toGraphQLFieldValues converts domain FieldValues map to GraphQL FieldValue slice
func toGraphQLFieldValues(fieldValues map[string]model.FieldValue) []*graphql1.FieldValue {
	if fieldValues == nil {
		return []*graphql1.FieldValue{}
	}
	keys := make([]string, 0, len(fieldValues))
	for k := range fieldValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*graphql1.FieldValue, 0, len(fieldValues))
	for _, k := range keys {
		fv := fieldValues[k]
		result = append(result, &graphql1.FieldValue{
			FieldID: string(fv.FieldID),
			Value:   fv.Value,
		})
	}
	return result
}

// toDomainFieldValues converts GraphQL FieldValueInput slice to domain FieldValues map
func toDomainFieldValues(inputs []*graphql1.FieldValueInput) map[string]model.FieldValue {
	if inputs == nil {
		return nil
	}
	result := make(map[string]model.FieldValue, len(inputs))
	for _, input := range inputs {
		result[input.FieldID] = model.FieldValue{
			FieldID: types.FieldID(input.FieldID),
			Value:   input.Value,
		}
	}
	return result
}

// actionArchiveFilterToScope maps the optional GraphQL ActionArchiveFilter
// to the domain ActionArchiveScope. nil maps to ActiveOnly to match the
// schema-side default.
func actionArchiveFilterToScope(f *graphql1.ActionArchiveFilter) interfaces.ActionArchiveScope {
	if f == nil {
		return interfaces.ActionArchiveScopeActiveOnly
	}
	switch *f {
	case graphql1.ActionArchiveFilterArchived:
		return interfaces.ActionArchiveScopeArchivedOnly
	case graphql1.ActionArchiveFilterAll:
		return interfaces.ActionArchiveScopeAll
	default:
		return interfaces.ActionArchiveScopeActiveOnly
	}
}

// memoArchiveFilterToScope maps the optional GraphQL MemoArchiveFilter
// to the domain MemoArchiveScope. nil maps to ActiveOnly to match the
// schema-side default.
func memoArchiveFilterToScope(f *graphql1.MemoArchiveFilter) interfaces.MemoArchiveScope {
	if f == nil {
		return interfaces.MemoArchiveScopeActiveOnly
	}
	switch *f {
	case graphql1.MemoArchiveFilterArchived:
		return interfaces.MemoArchiveScopeArchivedOnly
	case graphql1.MemoArchiveFilterAll:
		return interfaces.MemoArchiveScopeAll
	default:
		return interfaces.MemoArchiveScopeActiveOnly
	}
}

// toGraphQLMemo converts a domain Memo to its GraphQL view.
func toGraphQLMemo(m *model.Memo, workspaceID string) *graphql1.Memo {
	return &graphql1.Memo{
		ID:          string(m.ID),
		WorkspaceID: workspaceID,
		CaseID:      int(m.CaseID),
		Title:       m.Title,
		Fields:      toGraphQLFieldValues(m.FieldValues),
		ArchivedAt:  m.ArchivedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// toGraphQLTag maps a domain Tag to its GraphQL representation. Name is exposed
// as a nullable scalar (empty string → nil) since the tag name is optional.
func toGraphQLTag(t *model.Tag) *graphql1.Tag {
	var name *string
	if t.Name != "" {
		n := t.Name
		name = &n
	}
	return &graphql1.Tag{
		ID:        string(t.ID),
		Name:      name,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// toTagIDs converts a slice of GraphQL ID strings into domain TagIDs.
func toTagIDs(ids []string) []model.TagID {
	out := make([]model.TagID, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.TagID(id))
	}
	return out
}

// tagByIDFor loads the workspace's tags into a lookup map for resolving
// Knowledge.tags. The tag vocabulary is small, so one List per request keeps
// the conversion free of N+1 lookups.
func (r *Resolver) tagByIDFor(ctx context.Context, workspaceID string) (map[model.TagID]*model.Tag, error) {
	tags, err := r.UseCases.Tag.ListTags(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	m := make(map[model.TagID]*model.Tag, len(tags))
	for _, t := range tags {
		m[t.ID] = t
	}
	return m, nil
}

// toGraphQLKnowledge maps a domain Knowledge to its GraphQL representation. The
// embedding vector is intentionally not exposed. TagIDs are resolved to Tag
// objects via tagByID; ids missing from the map are skipped (never a nil
// element) and the slice is never nil, to satisfy the [Tag!]! contract.
func toGraphQLKnowledge(k *model.Knowledge, tagByID map[model.TagID]*model.Tag) *graphql1.Knowledge {
	tags := make([]*graphql1.Tag, 0, len(k.TagIDs))
	for _, id := range k.TagIDs {
		if t, ok := tagByID[id]; ok {
			tags = append(tags, toGraphQLTag(t))
		}
	}
	return &graphql1.Knowledge{
		ID:        string(k.ID),
		Title:     k.Title,
		Claim:     k.Claim,
		Tags:      tags,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
	}
}

// toGraphQLFieldType converts a domain FieldType to GraphQL FieldType
func toGraphQLFieldType(ft types.FieldType) graphql1.FieldType {
	switch ft {
	case types.FieldTypeText:
		return graphql1.FieldTypeText
	case types.FieldTypeNumber:
		return graphql1.FieldTypeNumber
	case types.FieldTypeSelect:
		return graphql1.FieldTypeSelect
	case types.FieldTypeMultiSelect:
		return graphql1.FieldTypeMultiSelect
	case types.FieldTypeUser:
		return graphql1.FieldTypeUser
	case types.FieldTypeMultiUser:
		return graphql1.FieldTypeMultiUser
	case types.FieldTypeDate:
		return graphql1.FieldTypeDate
	case types.FieldTypeURL:
		return graphql1.FieldTypeURL
	case types.FieldTypeCaseRef:
		return graphql1.FieldTypeCaseRef
	case types.FieldTypeMultiCaseRef:
		return graphql1.FieldTypeMultiCaseRef
	case types.FieldTypeMarkdown:
		return graphql1.FieldTypeMarkdown
	default:
		return graphql1.FieldTypeText
	}
}

// toGraphQLCaseRef converts a domain CaseRef to its GraphQL view.
func toGraphQLCaseRef(ref model.CaseRef) *graphql1.CaseRef {
	return &graphql1.CaseRef{
		ID:          int(ref.ID),
		Title:       ref.Title,
		Status:      ref.Status.Normalize(),
		WorkspaceID: ref.WorkspaceID,
	}
}
