package authz_test

import (
	"encoding/json"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
)

// The JSON keys are the contract policy authors write against, so they are
// pinned here rather than left to the struct tags alone.
func TestWorkspaceInput_JSONShape(t *testing.T) {
	in := authz.WorkspaceInput{
		Workspace: authz.WorkspaceRef{ID: "security", Name: "Security"},
		User: authz.WorkspaceUser{
			ID:          "U012ABCDEF",
			Email:       "alice@example.com",
			Name:        "alice",
			DisplayName: "Alice",
		},
	}
	raw, err := json.Marshal(in)
	gt.NoError(t, err)

	var got map[string]map[string]string
	gt.NoError(t, json.Unmarshal(raw, &got))
	gt.Value(t, got).Equal(map[string]map[string]string{
		"workspace": {"id": "security", "name": "Security"},
		"user": {
			"id":           "U012ABCDEF",
			"email":        "alice@example.com",
			"name":         "alice",
			"display_name": "Alice",
		},
	})
}

func TestWorkspaceResult_MissingAllowDecodesAsFalse(t *testing.T) {
	var res authz.WorkspaceResult
	gt.NoError(t, json.Unmarshal([]byte(`{"other": 1}`), &res))
	gt.Value(t, res.Allow).Equal(false)

	gt.NoError(t, json.Unmarshal([]byte(`{"allow": true}`), &res))
	gt.Value(t, res.Allow).Equal(true)
}
