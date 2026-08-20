package graphql_test

import (
	"testing"
	"time"

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

// TestToGraphQLActionComment pins the ActionComment mapping: every stored field
// reaches the wire, `edited` is derived from the timestamps rather than carried
// separately, and `author` is deliberately left nil for the dataloader-backed
// sub-resolver to fill.
func TestToGraphQLActionComment(t *testing.T) {
	created := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

	t.Run("an unedited comment maps every field and reports edited=false", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLActionCommentForTest(&model.ActionComment{
			ID:        "c-1",
			ActionID:  42,
			AuthorID:  "U123ABC",
			Body:      "the alert matched a maintenance window",
			CreatedAt: created,
			UpdatedAt: created,
		})
		gt.Value(t, g.ID).Equal("c-1")
		gt.Value(t, g.ActionID).Equal(42)
		gt.Value(t, g.AuthorID).Equal("U123ABC")
		gt.Value(t, g.Body).Equal("the alert matched a maintenance window")
		gt.Value(t, g.CreatedAt).Equal(created)
		gt.Value(t, g.UpdatedAt).Equal(created)
		gt.Bool(t, g.Edited).False()
		gt.Value(t, g.Author).Nil()
	})

	t.Run("a later UpdatedAt reports edited=true", func(t *testing.T) {
		g := graphqlctrl.ToGraphQLActionCommentForTest(&model.ActionComment{
			ID:        "c-2",
			ActionID:  42,
			AuthorID:  "U123ABC",
			Body:      "revised",
			CreatedAt: created,
			UpdatedAt: created.Add(time.Minute),
		})
		gt.Bool(t, g.Edited).True()
	})
}
