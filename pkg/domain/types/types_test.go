package types_test

import (
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestCategoryID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      types.CategoryID
		wantErr bool
	}{
		{"valid lowercase", "data-breach", false},
		{"valid single word", "security", false},
		{"valid with numbers", "risk-123", false},
		{"empty", "", true},
		{"uppercase", "Data-Breach", true},
		{"spaces", "data breach", true},
		{"underscore", "data_breach", true},
		{"starting with hyphen", "-data", true},
		{"ending with hyphen", "data-", true},
		{"double hyphen", "data--breach", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("CategoryID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLikelihoodID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      types.LikelihoodID
		wantErr bool
	}{
		{"valid lowercase", "very-low", false},
		{"valid single word", "low", false},
		{"empty", "", true},
		{"uppercase", "Very-Low", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("LikelihoodID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestImpactID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      types.ImpactID
		wantErr bool
	}{
		{"valid lowercase", "critical", false},
		{"valid with hyphen", "very-high", false},
		{"empty", "", true},
		{"uppercase", "Critical", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ImpactID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTeamID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      types.TeamID
		wantErr bool
	}{
		{"valid lowercase", "security-team", false},
		{"valid single word", "platform", false},
		{"empty", "", true},
		{"uppercase", "Security-Team", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TeamID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
