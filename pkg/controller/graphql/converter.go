package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// stringPtrToString converts *string to string
func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

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
		Responses:           []*graphql1.Response{}, // Resolved by field resolver
		CreatedAt:           risk.CreatedAt,
		UpdatedAt:           risk.UpdatedAt,
	}
}

// toGraphQLResponse converts domain Response to GraphQL Response
func toGraphQLResponse(response *model.Response) *graphql1.Response {
	// Store ResponderIDs temporarily in Responders for field resolver
	responders := make([]*graphql1.SlackUser, len(response.ResponderIDs))
	for i, id := range response.ResponderIDs {
		responders[i] = &graphql1.SlackUser{ID: id}
	}

	return &graphql1.Response{
		ID:          int(response.ID),
		Title:       response.Title,
		Description: response.Description,
		Responders:  responders, // Will be enriched by field resolver
		URL:         &response.URL,
		Status:      toGraphQLResponseStatus(response.Status),
		Risks:       []*graphql1.Risk{}, // Resolved by field resolver
		CreatedAt:   response.CreatedAt,
		UpdatedAt:   response.UpdatedAt,
	}
}

// toGraphQLResponseStatus converts domain ResponseStatus to GraphQL ResponseStatus
func toGraphQLResponseStatus(status types.ResponseStatus) graphql1.ResponseStatus {
	switch status {
	case types.ResponseStatusBacklog:
		return graphql1.ResponseStatusBacklog
	case types.ResponseStatusTodo:
		return graphql1.ResponseStatusTodo
	case types.ResponseStatusInProgress:
		return graphql1.ResponseStatusInProgress
	case types.ResponseStatusBlocked:
		return graphql1.ResponseStatusBlocked
	case types.ResponseStatusCompleted:
		return graphql1.ResponseStatusCompleted
	case types.ResponseStatusAbandoned:
		return graphql1.ResponseStatusAbandoned
	default:
		return graphql1.ResponseStatusBacklog
	}
}

// toDomainResponseStatus converts GraphQL ResponseStatus to domain ResponseStatus
func toDomainResponseStatus(status graphql1.ResponseStatus) types.ResponseStatus {
	switch status {
	case graphql1.ResponseStatusBacklog:
		return types.ResponseStatusBacklog
	case graphql1.ResponseStatusTodo:
		return types.ResponseStatusTodo
	case graphql1.ResponseStatusInProgress:
		return types.ResponseStatusInProgress
	case graphql1.ResponseStatusBlocked:
		return types.ResponseStatusBlocked
	case graphql1.ResponseStatusCompleted:
		return types.ResponseStatusCompleted
	case graphql1.ResponseStatusAbandoned:
		return types.ResponseStatusAbandoned
	default:
		return types.ResponseStatusBacklog
	}
}

// toGraphQLKnowledge converts domain Knowledge to GraphQL Knowledge
func toGraphQLKnowledge(k *model.Knowledge) *graphql1.Knowledge {
	return &graphql1.Knowledge{
		ID:        string(k.ID),
		RiskID:    int(k.RiskID),
		SourceID:  string(k.SourceID),
		SourceURL: k.SourceURL,
		Title:     k.Title,
		Summary:   k.Summary,
		SourcedAt: k.SourcedAt,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
	}
}

// enrichResponse enriches a Response with responder and risk information
func enrichResponse(ctx context.Context, uc *usecase.UseCases, response *graphql1.Response) *graphql1.Response {
	// Enrich responders
	slackSvc := uc.SlackService()
	if slackSvc != nil && len(response.Responders) > 0 {
		responderIDs := make([]string, len(response.Responders))
		for i, responder := range response.Responders {
			responderIDs[i] = responder.ID
		}

		users, err := slackSvc.ListUsers(ctx)
		if err == nil {
			userMap := make(map[string]*graphql1.SlackUser)
			for _, user := range users {
				var imageURL *string
				if user.ImageURL != "" {
					imageURL = &user.ImageURL
				}
				userMap[user.ID] = &graphql1.SlackUser{
					ID:       user.ID,
					Name:     user.Name,
					RealName: user.RealName,
					ImageURL: imageURL,
				}
			}

			enrichedResponders := make([]*graphql1.SlackUser, 0, len(responderIDs))
			for _, id := range responderIDs {
				if user, ok := userMap[id]; ok {
					enrichedResponders = append(enrichedResponders, user)
				}
			}
			response.Responders = enrichedResponders
		}
	}

	// Enrich risks using dataloader if available
	loaders := GetDataLoaders(ctx)
	if loaders != nil && loaders.RisksByResponseLoader != nil {
		risks, err := loaders.RisksByResponseLoader.Load(ctx, int64(response.ID))
		if err == nil {
			gqlRisks := make([]*graphql1.Risk, len(risks))
			for i, risk := range risks {
				gqlRisks[i] = toGraphQLRisk(risk)
			}
			response.Risks = gqlRisks
		}
	} else {
		// Fallback to direct query if DataLoader is not available
		risks, err := uc.Response.GetRisksByResponse(ctx, int64(response.ID))
		if err == nil {
			gqlRisks := make([]*graphql1.Risk, len(risks))
			for i, risk := range risks {
				gqlRisks[i] = toGraphQLRisk(risk)
			}
			response.Risks = gqlRisks
		}
	}

	return response
}
