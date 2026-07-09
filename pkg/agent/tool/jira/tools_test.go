package jira_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	jiratool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/jira"
)

// fakeToolSet records every Run call and returns canned specs/responses.
type fakeToolSet struct {
	specs    []gollem.ToolSpec
	specsErr error
	runCalls []fakeRunCall
	runResp  map[string]any
	runErr   error
}

type fakeRunCall struct {
	Name string
	Args map[string]any
}

func (f *fakeToolSet) Specs(_ context.Context) ([]gollem.ToolSpec, error) {
	return f.specs, f.specsErr
}

func (f *fakeToolSet) Run(_ context.Context, name string, args map[string]any) (map[string]any, error) {
	f.runCalls = append(f.runCalls, fakeRunCall{Name: name, Args: args})
	return f.runResp, f.runErr
}

func TestNew_NilToolSet(t *testing.T) {
	tools, err := jiratool.New(context.Background(), nil)
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(0)
}

func TestNew_ExpandsSpecsIntoTools(t *testing.T) {
	fake := &fakeToolSet{
		specs: []gollem.ToolSpec{
			{Name: "jira_list_projects", Description: "List projects"},
			{Name: "jira_search_issues", Description: "Search issues"},
			{Name: "jira_get_issues", Description: "Get issues"},
		},
	}

	tools, err := jiratool.New(context.Background(), fake)
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(3).Required()

	gt.Value(t, tools[0].Spec().Name).Equal("jira_list_projects")
	gt.Value(t, tools[0].Spec().Description).Equal("List projects")
	gt.Value(t, tools[1].Spec().Name).Equal("jira_search_issues")
	gt.Value(t, tools[1].Spec().Description).Equal("Search issues")
	gt.Value(t, tools[2].Spec().Name).Equal("jira_get_issues")
	gt.Value(t, tools[2].Spec().Description).Equal("Get issues")
}

func TestNew_SpecsError(t *testing.T) {
	fake := &fakeToolSet{specsErr: goerr.New("boom")}

	tools, err := jiratool.New(context.Background(), fake)
	gt.Value(t, tools).Nil()
	gt.Value(t, err).NotNil().Required()
	gt.Error(t, err).Is(fake.specsErr)
}

func TestToolRun_DelegatesToToolSetByName(t *testing.T) {
	fake := &fakeToolSet{
		specs: []gollem.ToolSpec{
			{Name: "jira_search_issues", Description: "Search issues"},
		},
		runResp: map[string]any{"items": []any{"ISSUE-1"}, "is_last": true},
	}

	tools, err := jiratool.New(context.Background(), fake)
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(1).Required()

	args := map[string]any{"jql": "project = ISSUE"}
	out, err := tools[0].Run(context.Background(), args)
	gt.NoError(t, err).Required()

	gt.Array(t, fake.runCalls).Length(1).Required()
	gt.Value(t, fake.runCalls[0].Name).Equal("jira_search_issues")
	gt.Value(t, fake.runCalls[0].Args).Equal(args)

	gt.Value(t, out["is_last"]).Equal(true)
	items, ok := out["items"].([]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, items).Length(1)
	gt.Value(t, items[0]).Equal("ISSUE-1")
}

func TestToolRun_WrapsError(t *testing.T) {
	fake := &fakeToolSet{
		specs:  []gollem.ToolSpec{{Name: "jira_get_issues", Description: "Get issues"}},
		runErr: goerr.New("upstream failure"),
	}

	tools, err := jiratool.New(context.Background(), fake)
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(1).Required()

	out, err := tools[0].Run(context.Background(), map[string]any{"issue_keys": []any{"ISSUE-1"}})
	gt.Value(t, out).Nil()
	gt.Value(t, err).NotNil().Required()
	gt.Error(t, err).Is(fake.runErr)
}
