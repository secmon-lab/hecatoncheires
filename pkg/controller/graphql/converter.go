package graphql

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// toGraphQLRisk converts domain Risk to GraphQL Risk
func toGraphQLRisk(risk *model.Risk) *graphql1.Risk {
	categoryIDs := make([]string, len(risk.CategoryIDs))
	for i, id := range risk.CategoryIDs {
		categoryIDs[i] = string(id)
	}

	teamIDs := make([]string, len(risk.ResponseTeamIDs))
	for i, id := range risk.ResponseTeamIDs {
		teamIDs[i] = string(id)
	}

	return &graphql1.Risk{
		ID:                  int(risk.ID),
		Name:                risk.Name,
		Description:         risk.Description,
		CategoryIDs:         categoryIDs,
		Categories:          []*graphql1.Category{}, // Resolved by field resolver
		SpecificImpact:      risk.SpecificImpact,
		LikelihoodID:        string(risk.LikelihoodID),
		LikelihoodLevel:     nil, // Resolved by field resolver
		ImpactID:            string(risk.ImpactID),
		ImpactLevel:         nil, // Resolved by field resolver
		ResponseTeamIDs:     teamIDs,
		ResponseTeams:       []*graphql1.Team{}, // Resolved by field resolver
		AssigneeIDs:         risk.AssigneeIDs,
		Assignees:           []*graphql1.SlackUser{}, // Resolved by field resolver
		DetectionIndicators: risk.DetectionIndicators,
		CreatedAt:           risk.CreatedAt,
		UpdatedAt:           risk.UpdatedAt,
	}
}
