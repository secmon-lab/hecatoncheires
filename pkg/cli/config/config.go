package config

import (
	"os"

	"github.com/m-mizutani/goerr/v2"
	"github.com/pelletier/go-toml/v2"
	domainConfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// AppConfig represents the application configuration
type AppConfig struct {
	Categories []Category        `toml:"category"`
	Likelihood []LikelihoodLevel `toml:"likelihood"`
	Impact     []ImpactLevel     `toml:"impact"`
	Teams      []Team            `toml:"team"`
}

// Category represents a risk category configuration
type Category struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// Validate checks if the Category is valid
func (c *Category) Validate() error {
	id := types.CategoryID(c.ID)
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid category ID")
	}
	if c.Name == "" {
		return goerr.New("category name is required", goerr.V("id", c.ID))
	}
	return nil
}

// LikelihoodLevel represents a likelihood level configuration
type LikelihoodLevel struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Score       int    `toml:"score"`
}

// Validate checks if the LikelihoodLevel is valid
func (l *LikelihoodLevel) Validate() error {
	id := types.LikelihoodID(l.ID)
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid likelihood ID")
	}
	if l.Name == "" {
		return goerr.New("likelihood name is required", goerr.V("id", l.ID))
	}
	if l.Score < 1 || l.Score > 5 {
		return goerr.New("likelihood score must be between 1 and 5", goerr.V("id", l.ID), goerr.V("score", l.Score))
	}
	return nil
}

// ImpactLevel represents an impact level configuration
type ImpactLevel struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Score       int    `toml:"score"`
}

// Validate checks if the ImpactLevel is valid
func (i *ImpactLevel) Validate() error {
	id := types.ImpactID(i.ID)
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid impact ID")
	}
	if i.Name == "" {
		return goerr.New("impact name is required", goerr.V("id", i.ID))
	}
	if i.Score < 1 || i.Score > 5 {
		return goerr.New("impact score must be between 1 and 5", goerr.V("id", i.ID), goerr.V("score", i.Score))
	}
	return nil
}

// Team represents a team configuration
type Team struct {
	ID   string `toml:"id"`
	Name string `toml:"name"`
}

// Validate checks if the Team is valid
func (t *Team) Validate() error {
	id := types.TeamID(t.ID)
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid team ID")
	}
	if t.Name == "" {
		return goerr.New("team name is required", goerr.V("id", t.ID))
	}
	return nil
}

// Validate checks if the AppConfig is valid
func (a *AppConfig) Validate() error {
	// Check category duplicates
	categoryIDs := make(map[string]bool)
	for _, cat := range a.Categories {
		if err := cat.Validate(); err != nil {
			return goerr.Wrap(err, "invalid category")
		}
		if categoryIDs[cat.ID] {
			return goerr.New("duplicate category ID", goerr.V("id", cat.ID))
		}
		categoryIDs[cat.ID] = true
	}

	// Check likelihood duplicates
	likelihoodIDs := make(map[string]bool)
	for _, lh := range a.Likelihood {
		if err := lh.Validate(); err != nil {
			return goerr.Wrap(err, "invalid likelihood level")
		}
		if likelihoodIDs[lh.ID] {
			return goerr.New("duplicate likelihood ID", goerr.V("id", lh.ID))
		}
		likelihoodIDs[lh.ID] = true
	}

	// Check impact duplicates
	impactIDs := make(map[string]bool)
	for _, imp := range a.Impact {
		if err := imp.Validate(); err != nil {
			return goerr.Wrap(err, "invalid impact level")
		}
		if impactIDs[imp.ID] {
			return goerr.New("duplicate impact ID", goerr.V("id", imp.ID))
		}
		impactIDs[imp.ID] = true
	}

	// Check team duplicates
	teamIDs := make(map[string]bool)
	for _, team := range a.Teams {
		if err := team.Validate(); err != nil {
			return goerr.Wrap(err, "invalid team")
		}
		if teamIDs[team.ID] {
			return goerr.New("duplicate team ID", goerr.V("id", team.ID))
		}
		teamIDs[team.ID] = true
	}

	return nil
}

// LoadAppConfiguration loads the application configuration from a TOML file
func LoadAppConfiguration(path string) (*AppConfig, error) {
	// #nosec G304 - path is expected to be provided by CLI argument
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to read config file", goerr.V("path", path))
	}

	var config AppConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, goerr.Wrap(err, "failed to parse TOML config", goerr.V("path", path))
	}

	if err := config.Validate(); err != nil {
		return nil, goerr.Wrap(err, "config validation failed", goerr.V("path", path))
	}

	return &config, nil
}

// ToDomainRiskConfig converts AppConfig to domain RiskConfig
func (a *AppConfig) ToDomainRiskConfig() *domainConfig.RiskConfig {
	categories := make([]domainConfig.Category, len(a.Categories))
	for i, cat := range a.Categories {
		categories[i] = domainConfig.Category{
			ID:          cat.ID,
			Name:        cat.Name,
			Description: cat.Description,
		}
	}

	likelihood := make([]domainConfig.LikelihoodLevel, len(a.Likelihood))
	for i, level := range a.Likelihood {
		likelihood[i] = domainConfig.LikelihoodLevel{
			ID:          level.ID,
			Name:        level.Name,
			Description: level.Description,
			Score:       level.Score,
		}
	}

	impact := make([]domainConfig.ImpactLevel, len(a.Impact))
	for i, level := range a.Impact {
		impact[i] = domainConfig.ImpactLevel{
			ID:          level.ID,
			Name:        level.Name,
			Description: level.Description,
			Score:       level.Score,
		}
	}

	teams := make([]domainConfig.Team, len(a.Teams))
	for i, team := range a.Teams {
		teams[i] = domainConfig.Team{
			ID:   team.ID,
			Name: team.Name,
		}
	}

	return &domainConfig.RiskConfig{
		Categories: categories,
		Likelihood: likelihood,
		Impact:     impact,
		Teams:      teams,
	}
}
