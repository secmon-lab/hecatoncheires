package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// CategoryLoader loads categories from Risk Configuration
// Risk Configuration is loaded from TOML file at application startup (not from Firestore)
type CategoryLoader struct {
	riskUC *usecase.RiskUseCase
}

// NewCategoryLoader creates a new CategoryLoader
func NewCategoryLoader(riskUC *usecase.RiskUseCase) *CategoryLoader {
	return &CategoryLoader{
		riskUC: riskUC,
	}
}

// Load retrieves a single category by ID
func (l *CategoryLoader) Load(ctx context.Context, categoryID types.CategoryID) (*graphql1.Category, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	for _, cat := range cfg.Categories {
		if types.CategoryID(cat.ID) == categoryID {
			return &graphql1.Category{
				ID:          cat.ID,
				Name:        cat.Name,
				Description: cat.Description,
			}, nil
		}
	}

	return nil, nil
}

// LoadMany retrieves multiple categories by IDs
func (l *CategoryLoader) LoadMany(ctx context.Context, categoryIDs []types.CategoryID) ([]*graphql1.Category, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return make([]*graphql1.Category, 0), nil
	}

	// Create a map for fast lookup
	categoryMap := make(map[types.CategoryID]config.Category)
	for _, cat := range cfg.Categories {
		categoryMap[types.CategoryID(cat.ID)] = cat
	}

	// Build result preserving order
	result := make([]*graphql1.Category, 0, len(categoryIDs))
	for _, id := range categoryIDs {
		if cat, ok := categoryMap[id]; ok {
			result = append(result, &graphql1.Category{
				ID:          cat.ID,
				Name:        cat.Name,
				Description: cat.Description,
			})
		}
	}

	return result, nil
}

// LikelihoodLevelLoader loads likelihood levels from Risk Configuration
type LikelihoodLevelLoader struct {
	riskUC *usecase.RiskUseCase
}

// NewLikelihoodLevelLoader creates a new LikelihoodLevelLoader
func NewLikelihoodLevelLoader(riskUC *usecase.RiskUseCase) *LikelihoodLevelLoader {
	return &LikelihoodLevelLoader{
		riskUC: riskUC,
	}
}

// Load retrieves a single likelihood level by ID
func (l *LikelihoodLevelLoader) Load(ctx context.Context, levelID types.LikelihoodID) (*graphql1.LikelihoodLevel, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	for _, level := range cfg.Likelihood {
		if types.LikelihoodID(level.ID) == levelID {
			return &graphql1.LikelihoodLevel{
				ID:          level.ID,
				Name:        level.Name,
				Description: level.Description,
				Score:       level.Score,
			}, nil
		}
	}

	return nil, nil
}

// ImpactLevelLoader loads impact levels from Risk Configuration
type ImpactLevelLoader struct {
	riskUC *usecase.RiskUseCase
}

// NewImpactLevelLoader creates a new ImpactLevelLoader
func NewImpactLevelLoader(riskUC *usecase.RiskUseCase) *ImpactLevelLoader {
	return &ImpactLevelLoader{
		riskUC: riskUC,
	}
}

// Load retrieves a single impact level by ID
func (l *ImpactLevelLoader) Load(ctx context.Context, levelID types.ImpactID) (*graphql1.ImpactLevel, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	for _, level := range cfg.Impact {
		if types.ImpactID(level.ID) == levelID {
			return &graphql1.ImpactLevel{
				ID:          level.ID,
				Name:        level.Name,
				Description: level.Description,
				Score:       level.Score,
			}, nil
		}
	}

	return nil, nil
}

// TeamLoader loads teams from Risk Configuration
type TeamLoader struct {
	riskUC *usecase.RiskUseCase
}

// NewTeamLoader creates a new TeamLoader
func NewTeamLoader(riskUC *usecase.RiskUseCase) *TeamLoader {
	return &TeamLoader{
		riskUC: riskUC,
	}
}

// Load retrieves a single team by ID
func (l *TeamLoader) Load(ctx context.Context, teamID types.TeamID) (*graphql1.Team, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	for _, team := range cfg.Teams {
		if types.TeamID(team.ID) == teamID {
			return &graphql1.Team{
				ID:   team.ID,
				Name: team.Name,
			}, nil
		}
	}

	return nil, nil
}

// LoadMany retrieves multiple teams by IDs
func (l *TeamLoader) LoadMany(ctx context.Context, teamIDs []types.TeamID) ([]*graphql1.Team, error) {
	cfg, err := l.riskUC.GetRiskConfiguration()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return make([]*graphql1.Team, 0), nil
	}

	// Create a map for fast lookup
	teamMap := make(map[types.TeamID]config.Team)
	for _, team := range cfg.Teams {
		teamMap[types.TeamID(team.ID)] = team
	}

	// Build result preserving order
	result := make([]*graphql1.Team, 0, len(teamIDs))
	for _, id := range teamIDs {
		if team, ok := teamMap[id]; ok {
			result = append(result, &graphql1.Team{
				ID:   team.ID,
				Name: team.Name,
			})
		}
	}

	return result, nil
}
