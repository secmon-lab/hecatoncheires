package model

import "time"

// RiskResponse represents the many-to-many relationship between Risk and Response
type RiskResponse struct {
	RiskID     int64
	ResponseID int64
	CreatedAt  time.Time
}
