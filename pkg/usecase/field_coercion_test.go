package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestCoerceFieldValue(t *testing.T) {
	t.Run("markdown accepts a string", func(t *testing.T) {
		got, ok := usecase.CoerceFieldValueForTest("# Heading\n\ntext", types.FieldTypeMarkdown)
		gt.Bool(t, ok).True()
		gt.Value(t, got).Equal("# Heading\n\ntext")
	})

	t.Run("markdown rejects a non-string", func(t *testing.T) {
		// The string-group coercion returns the zero value ("") with ok=false;
		// callers must gate on ok and ignore the value, so only ok matters here.
		_, ok := usecase.CoerceFieldValueForTest(123, types.FieldTypeMarkdown)
		gt.Bool(t, ok).False()
	})

	t.Run("markdown accepts an empty string", func(t *testing.T) {
		got, ok := usecase.CoerceFieldValueForTest("", types.FieldTypeMarkdown)
		gt.Bool(t, ok).True()
		gt.Value(t, got).Equal("")
	})

	// Sanity: the pre-existing scalar-string and number contracts are
	// unchanged by adding markdown to the same coercion group.
	t.Run("text still accepts a string", func(t *testing.T) {
		got, ok := usecase.CoerceFieldValueForTest("plain", types.FieldTypeText)
		gt.Bool(t, ok).True()
		gt.Value(t, got).Equal("plain")
	})

	t.Run("number coerces int to float64", func(t *testing.T) {
		got, ok := usecase.CoerceFieldValueForTest(5, types.FieldTypeNumber)
		gt.Bool(t, ok).True()
		gt.Value(t, got).Equal(float64(5))
	})
}
