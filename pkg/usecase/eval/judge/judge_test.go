package judge_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/judge"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

type fakeCompleter struct{ out string }

func (f *fakeCompleter) Complete(_ context.Context, _, _ string, _ *gollem.Parameter) (string, error) {
	return f.out, nil
}

func art() evaltype.Artifact {
	return &evaltype.CaseArtifact{Case: &model.Case{Title: "Portal login 503"}}
}

var checks = []scenario.Check{
	{ID: "c1", Question: "title ok?"},
	{ID: "c2", Question: "severity high?"},
}

func TestEvaluate_MapsVerdictsInOrder(t *testing.T) {
	fc := &fakeCompleter{out: `{"verdicts":[
		{"id":"c2","passed":false,"reason":"no severity"},
		{"id":"c1","passed":true,"reason":"title good"}
	]}`}
	j := judge.New(fc, "en")
	vs, err := j.Evaluate(context.Background(), art(), checks)
	gt.NoError(t, err)
	gt.A(t, vs).Length(2)
	// Order follows the checks slice, not the model output order.
	gt.V(t, vs[0].ID).Equal("c1")
	gt.B(t, vs[0].Passed).True()
	gt.V(t, vs[1].ID).Equal("c2")
	gt.B(t, vs[1].Passed).False()
	gt.V(t, vs[1].Reason).Equal("no severity")
}

func TestEvaluate_MissingVerdictIsError(t *testing.T) {
	fc := &fakeCompleter{out: `{"verdicts":[{"id":"c1","passed":true,"reason":"ok"}]}`}
	j := judge.New(fc, "")
	_, err := j.Evaluate(context.Background(), art(), checks)
	gt.Error(t, err)
}

func TestEvaluate_ExtraVerdictIgnored(t *testing.T) {
	fc := &fakeCompleter{out: `{"verdicts":[
		{"id":"c1","passed":true,"reason":"ok"},
		{"id":"c2","passed":true,"reason":"ok"},
		{"id":"unknown","passed":false,"reason":"ignored"}
	]}`}
	j := judge.New(fc, "")
	vs, err := j.Evaluate(context.Background(), art(), checks)
	gt.NoError(t, err)
	gt.A(t, vs).Length(2)
}

func TestEvaluate_BadJSON(t *testing.T) {
	j := judge.New(&fakeCompleter{out: "nope"}, "")
	_, err := j.Evaluate(context.Background(), art(), checks)
	gt.Error(t, err)
}

func TestEvaluate_NoChecks(t *testing.T) {
	j := judge.New(&fakeCompleter{out: "{}"}, "")
	_, err := j.Evaluate(context.Background(), art(), nil)
	gt.Error(t, err)
}
