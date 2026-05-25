package graphql

import (
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

	return &graphql1.Case{
		ID:                    int(c.ID),
		WorkspaceID:           workspaceID,
		Title:                 c.Title,
		Description:           c.Description,
		Status:                c.Status.Normalize(),
		IsPrivate:             c.IsPrivate,
		AccessDenied:          c.AccessDenied,
		ChannelUserIDs:        channelUserIDs,
		ReporterID:            reporterID,
		AssigneeIDs:           assigneeIDs,
		SlackChannelID:        &c.SlackChannelID,
		Fields:                toGraphQLFieldValues(c.FieldValues),
		AgentAdditionalPrompt: agentPrompt,
		AgentSourceIDs:        agentSourceIDs,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
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
	default:
		return graphql1.FieldTypeText
	}
}
