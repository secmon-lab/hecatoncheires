package usersim_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/usersim"
)

type fakeCompleter struct {
	out string
}

func (f *fakeCompleter) Complete(_ context.Context, _, _ string, _ *gollem.Parameter) (string, error) {
	return f.out, nil
}

func TestAnswer_SelectConstrainedToOptions(t *testing.T) {
	// Model returns an out-of-range value for a select item; it must be forced
	// to a valid option.
	fc := &fakeCompleter{out: `{"answers":[{"id":"q1","value":"bogus"}]}`}
	sim := usersim.New(fc, scenario.Persona{Description: "user"}, "en")

	ans, err := sim.Answer(context.Background(), evaltype.Question{
		Items: []evaltype.QuestionItem{
			{ID: "q1", Type: evaltype.QuestionSelect, Text: "severity?", Options: []string{"high", "low"}},
		},
	})
	gt.NoError(t, err)
	gt.A(t, ans.Items).Length(1)
	gt.V(t, ans.Items[0].ID).Equal("q1")
	gt.V(t, ans.Items[0].Value).Equal("high") // forced to first option
}

func TestAnswer_MultiSelectFiltersInvalid(t *testing.T) {
	fc := &fakeCompleter{out: `{"answers":[{"id":"q1","values":["a","x","b"]}]}`}
	sim := usersim.New(fc, scenario.Persona{}, "")
	ans, err := sim.Answer(context.Background(), evaltype.Question{
		Items: []evaltype.QuestionItem{
			{ID: "q1", Type: evaltype.QuestionMultiSelect, Options: []string{"a", "b", "c"}},
		},
	})
	gt.NoError(t, err)
	gt.A(t, ans.Items[0].Values).Equal([]string{"a", "b"})
}

func TestAnswer_FreeText(t *testing.T) {
	fc := &fakeCompleter{out: `{"answers":[{"id":"q1","value":"it started at 9am"}]}`}
	sim := usersim.New(fc, scenario.Persona{}, "")
	ans, err := sim.Answer(context.Background(), evaltype.Question{
		Items: []evaltype.QuestionItem{{ID: "q1", Type: evaltype.QuestionFreeText}},
	})
	gt.NoError(t, err)
	gt.V(t, ans.Items[0].Value).Equal("it started at 9am")
}

func TestAnswer_BadJSON(t *testing.T) {
	fc := &fakeCompleter{out: `not json`}
	sim := usersim.New(fc, scenario.Persona{}, "")
	_, err := sim.Answer(context.Background(), evaltype.Question{
		Items: []evaltype.QuestionItem{{ID: "q1", Type: evaltype.QuestionFreeText}},
	})
	gt.Error(t, err)
}
