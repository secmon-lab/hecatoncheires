package graphql_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// TestToGraphQLCase_Reporter pins the reporter mapping at the converter layer:
// a reporterless thread-mode Case (an integration-bot intake post that named no
// human) must surface ReporterID as a nil pointer so the schema's nullable
// `reporter` field resolves to null instead of erroring.
func TestToGraphQLCase_Reporter(t *testing.T) {
	t.Run("empty reporter id maps to a nil pointer", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLCaseForTest(&model.Case{
			ID:            1,
			Title:         "Bot-relayed, no reporter",
			SlackThreadTS: "1700000000.000100",
		}, "ws")
		gt.Value(t, g.ReporterID).Nil()
	})

	t.Run("non-empty reporter id maps to a pointer to that id", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLCaseForTest(&model.Case{
			ID:         2,
			Title:      "Has reporter",
			ReporterID: "U123ABC",
		}, "ws")
		gt.Value(t, g.ReporterID).NotNil().Required()
		gt.Value(t, *g.ReporterID).Equal("U123ABC")
	})
}

// TestToGraphQLFieldType_Markdown pins the domain → GraphQL enum bridge for the
// markdown field type, which (unlike CaseStatus) has no direct gqlgen binding
// and relies on the hand-written converter switch.
func TestToGraphQLFieldType_Markdown(t *testing.T) {
	gt.Value(t, graphqlctrl.ToGraphQLFieldTypeForTest(types.FieldTypeMarkdown)).
		Equal(graphql1.FieldTypeMarkdown)
	gt.Value(t, graphqlctrl.ToGraphQLFieldTypeForTest(types.FieldTypeText)).
		Equal(graphql1.FieldTypeText)
}
