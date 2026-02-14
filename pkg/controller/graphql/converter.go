package graphql

import (
	"sort"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// toGraphQLCase converts a domain Case to GraphQL Case
func toGraphQLCase(c *model.Case, workspaceID string) *graphql1.Case {
	// Ensure non-null list fields are never nil (schema: [String!]!)
	assigneeIDs := c.AssigneeIDs
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}

	// Treat empty status as OPEN for backward compatibility
	status := c.Status
	if status == "" {
		status = types.CaseStatusOpen
	}

	return &graphql1.Case{
		ID:             int(c.ID),
		WorkspaceID:    workspaceID,
		Title:          c.Title,
		Description:    c.Description,
		Status:         status,
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

	// Ensure non-null list fields are never nil (schema: [String!]!)
	assigneeIDs := a.AssigneeIDs
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}

	return &graphql1.Action{
		ID:             int(a.ID),
		WorkspaceID:    workspaceID,
		CaseID:         int(a.CaseID),
		Title:          a.Title,
		Description:    a.Description,
		AssigneeIDs:    assigneeIDs,
		SlackMessageTs: &slackMessageTS,
		Status:         a.Status,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

// toGraphQLKnowledge converts a domain Knowledge to GraphQL Knowledge
func toGraphQLKnowledge(k *model.Knowledge, workspaceID string) *graphql1.Knowledge {
	return &graphql1.Knowledge{
		ID:          string(k.ID),
		WorkspaceID: workspaceID,
		CaseID:      int(k.CaseID),
		SourceID:    string(k.SourceID),
		SourceURL:   k.SourceURL,
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
