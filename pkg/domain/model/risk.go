package model

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type Risk struct {
	ID                  int64
	Name                string
	Description         string
	CategoryIDs         []types.CategoryID
	SpecificImpact      string
	LikelihoodID        types.LikelihoodID
	ImpactID            types.ImpactID
	ResponseTeamIDs     []types.TeamID
	AssigneeIDs         []string
	DetectionIndicators string
	SlackChannelID      string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
