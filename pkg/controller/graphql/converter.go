package graphql

import (
	"sort"

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

	return &graphql1.Case{
		ID:             int(c.ID),
		WorkspaceID:    workspaceID,
		Title:          c.Title,
		Description:    c.Description,
		Status:         c.Status.Normalize(),
		IsPrivate:      c.IsPrivate,
		AccessDenied:   c.AccessDenied,
		ChannelUserIDs: channelUserIDs,
		ReporterID:     reporterID,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: &c.SlackChannelID,
		Fields:         toGraphQLFieldValues(c.FieldValues),
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
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
		Status:         a.Status,
		DueDate:        a.DueDate,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// toGraphQLKnowledge converts a domain Knowledge to GraphQL Knowledge
func toGraphQLKnowledge(k *model.Knowledge, workspaceID string) *graphql1.Knowledge {
	sourceURLs := k.SourceURLs
	if sourceURLs == nil {
		sourceURLs = []string{}
	}

	return &graphql1.Knowledge{
		ID:          string(k.ID),
		WorkspaceID: workspaceID,
		CaseID:      int(k.CaseID),
		SourceID:    string(k.SourceID),
		SourceURLs:  sourceURLs,
		Title:       k.Title,
		Summary:     k.Summary,
		SourcedAt:   k.SourcedAt,
		CreatedAt:   k.CreatedAt,
		UpdatedAt:   k.UpdatedAt,
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
