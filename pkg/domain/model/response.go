package model

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Response represents a response to one or more risks
type Response struct {
	ID           int64
	Title        string
	Description  string
	ResponderIDs []string // Slack User IDs
	URL          string
	Status       types.ResponseStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
