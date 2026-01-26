package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// DataLoaders holds all data loaders for batching queries
type DataLoaders struct {
	ResponsesByRiskLoader  *ResponsesByRiskLoader
	RisksByResponseLoader  *RisksByResponseLoader
	KnowledgesByRiskLoader *KnowledgesByRiskLoader
	SlackUsersLoader       *SlackUsersLoader
	CategoryLoader         *CategoryLoader
	LikelihoodLevelLoader  *LikelihoodLevelLoader
	ImpactLevelLoader      *ImpactLevelLoader
	TeamLoader             *TeamLoader
}

// NewDataLoaders creates a new instance of DataLoaders
// slackUsersCache is application-scoped and can be nil
// uc is required for Risk Configuration loaders
func NewDataLoaders(repo interfaces.Repository, uc *usecase.UseCases, slackUsersCache *SlackUsersCache) *DataLoaders {
	return &DataLoaders{
		ResponsesByRiskLoader:  NewResponsesByRiskLoader(repo),
		RisksByResponseLoader:  NewRisksByResponseLoader(repo),
		KnowledgesByRiskLoader: NewKnowledgesByRiskLoader(repo),
		SlackUsersLoader:       NewSlackUsersLoader(slackUsersCache),
		CategoryLoader:         NewCategoryLoader(uc.Risk),
		LikelihoodLevelLoader:  NewLikelihoodLevelLoader(uc.Risk),
		ImpactLevelLoader:      NewImpactLevelLoader(uc.Risk),
		TeamLoader:             NewTeamLoader(uc.Risk),
	}
}

// ResponsesByRiskLoader batches Response fetches by Risk ID
type ResponsesByRiskLoader struct {
	repo interfaces.Repository
}

// NewResponsesByRiskLoader creates a new ResponsesByRiskLoader
func NewResponsesByRiskLoader(repo interfaces.Repository) *ResponsesByRiskLoader {
	return &ResponsesByRiskLoader{
		repo: repo,
	}
}

// Load fetches responses for a single risk ID
func (l *ResponsesByRiskLoader) Load(ctx context.Context, riskID int64) ([]*model.Response, error) {
	return l.repo.RiskResponse().GetResponsesByRisk(ctx, riskID)
}

// LoadMany fetches responses for multiple risk IDs in a single batch
func (l *ResponsesByRiskLoader) LoadMany(ctx context.Context, riskIDs []int64) (map[int64][]*model.Response, error) {
	return l.repo.RiskResponse().GetResponsesByRisks(ctx, riskIDs)
}

// RisksByResponseLoader batches Risk fetches by Response ID
type RisksByResponseLoader struct {
	repo interfaces.Repository
}

// NewRisksByResponseLoader creates a new RisksByResponseLoader
func NewRisksByResponseLoader(repo interfaces.Repository) *RisksByResponseLoader {
	return &RisksByResponseLoader{
		repo: repo,
	}
}

// Load fetches risks for a single response ID
func (l *RisksByResponseLoader) Load(ctx context.Context, responseID int64) ([]*model.Risk, error) {
	return l.repo.RiskResponse().GetRisksByResponse(ctx, responseID)
}

// LoadMany fetches risks for multiple response IDs in a single batch
func (l *RisksByResponseLoader) LoadMany(ctx context.Context, responseIDs []int64) (map[int64][]*model.Risk, error) {
	return l.repo.RiskResponse().GetRisksByResponses(ctx, responseIDs)
}

// dataLoadersKey is the context key for data loaders
type dataLoadersKey struct{}

// WithDataLoaders adds data loaders to the context
func WithDataLoaders(ctx context.Context, loaders *DataLoaders) context.Context {
	return context.WithValue(ctx, dataLoadersKey{}, loaders)
}

// GetDataLoaders retrieves data loaders from the context
func GetDataLoaders(ctx context.Context) *DataLoaders {
	loaders, _ := ctx.Value(dataLoadersKey{}).(*DataLoaders)
	return loaders
}
