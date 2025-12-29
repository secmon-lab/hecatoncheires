package config

// Category represents a risk category configuration
type Category struct {
	ID          string
	Name        string
	Description string
}

// LikelihoodLevel represents a likelihood level configuration
type LikelihoodLevel struct {
	ID          string
	Name        string
	Description string
	Score       int
}

// ImpactLevel represents an impact level configuration
type ImpactLevel struct {
	ID          string
	Name        string
	Description string
	Score       int
}

// Team represents a team configuration
type Team struct {
	ID   string
	Name string
}

// RiskConfig holds all risk-related configuration
type RiskConfig struct {
	Categories []Category
	Likelihood []LikelihoodLevel
	Impact     []ImpactLevel
	Teams      []Team
}
