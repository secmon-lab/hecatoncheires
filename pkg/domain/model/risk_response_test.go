package model_test

import (
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestRiskResponse(t *testing.T) {
	now := time.Now()
	riskResponse := &model.RiskResponse{
		RiskID:     1,
		ResponseID: 2,
		CreatedAt:  now,
	}

	if riskResponse.RiskID != 1 {
		t.Errorf("RiskResponse.RiskID = %v, want 1", riskResponse.RiskID)
	}

	if riskResponse.ResponseID != 2 {
		t.Errorf("RiskResponse.ResponseID = %v, want 2", riskResponse.ResponseID)
	}

	if riskResponse.CreatedAt != now {
		t.Errorf("RiskResponse.CreatedAt = %v, want %v", riskResponse.CreatedAt, now)
	}
}

func TestRiskResponse_ZeroValues(t *testing.T) {
	riskResponse := &model.RiskResponse{}

	if riskResponse.RiskID != 0 {
		t.Errorf("RiskResponse.RiskID = %v, want 0", riskResponse.RiskID)
	}

	if riskResponse.ResponseID != 0 {
		t.Errorf("RiskResponse.ResponseID = %v, want 0", riskResponse.ResponseID)
	}

	if !riskResponse.CreatedAt.IsZero() {
		t.Errorf("RiskResponse.CreatedAt should be zero value")
	}
}
