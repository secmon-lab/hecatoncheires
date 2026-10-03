package proposal_test

import (
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/hecatoncheires/pkg/usecase/agent/proposal"
)

// Draft.Validate is what stops a shapeless proposal reaching the human.
func TestDraftValidate(t *testing.T) {
	for name, d := range map[string]proposal.Draft{
		"no fields": {WorkspaceID: "risk", Title: "T", Description: "D"},
		"distinct fields": {WorkspaceID: "risk", Title: "T", Description: "D", Fields: []proposal.DraftFieldForTest{
			{FieldID: "severity", Value: "high"},
			{FieldID: "tags", Values: []string{"a", "b"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			gt.NoError(t, d.Validate())
		})
	}

	for name, d := range map[string]proposal.Draft{
		"no workspace":   {Title: "T", Description: "D"},
		"no title":       {WorkspaceID: "risk", Description: "D"},
		"blank title":    {WorkspaceID: "risk", Title: "   ", Description: "D"},
		"no description": {WorkspaceID: "risk", Title: "T"},
		"blank field id": {WorkspaceID: "risk", Title: "T", Description: "D", Fields: []proposal.DraftFieldForTest{
			{FieldID: "  ", Value: "high"},
		}},
		"duplicate field id": {WorkspaceID: "risk", Title: "T", Description: "D", Fields: []proposal.DraftFieldForTest{
			{FieldID: "severity", Value: "high"},
			{FieldID: "severity", Value: "low"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, d.Validate())
		})
	}
}

// The draft is the schema of the turn's terminal call. Before it was a list, a
// map field made gollem build a schema every provider refused, so the call
// failed on every run without ever reaching the model.
func TestDraftSchemaIsSendable(t *testing.T) {
	schema, err := gollem.ToSchema(proposal.Draft{})
	gt.NoError(t, err).Required()
	gt.NoError(t, schema.Validate())

	fields := schema.Properties["fields"]
	gt.Value(t, fields).NotNil().Required()
	gt.Value(t, fields.Type).Equal(gollem.TypeArray)
	gt.Value(t, fields.Items).NotNil().Required()
	gt.Value(t, fields.Items.AdditionalProperties).Nil()
	gt.Map(t, fields.Items.Properties).HasKey("field_id")
	gt.Map(t, fields.Items.Properties).HasKey("value")
	gt.Map(t, fields.Items.Properties).HasKey("values")
}
