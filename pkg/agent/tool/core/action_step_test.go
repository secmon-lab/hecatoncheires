package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// mockActionStepMutator records every call so we can verify (a) the tool
// extracts arguments correctly, (b) the tool routes through the mutator
// instead of poking the repository directly, and (c) the underlying
// usecase fakes exercise the same surface.
type mockActionStepMutator struct {
	listFn    func(ctx context.Context, ws string, actionID int64) ([]*model.ActionStep, error)
	addFn     func(ctx context.Context, ws string, actionID int64, title string) (*model.ActionStep, error)
	setDoneFn func(ctx context.Context, ws string, actionID int64, stepID string, done bool) (*model.ActionStep, error)
	renameFn  func(ctx context.Context, ws string, actionID int64, stepID string, title string) (*model.ActionStep, error)
	deleteFn  func(ctx context.Context, ws string, actionID int64, stepID string) error
}

func (m *mockActionStepMutator) List(ctx context.Context, ws string, actionID int64) ([]*model.ActionStep, error) {
	if m.listFn != nil {
		return m.listFn(ctx, ws, actionID)
	}
	return nil, nil
}

func (m *mockActionStepMutator) Add(ctx context.Context, ws string, actionID int64, title string) (*model.ActionStep, error) {
	if m.addFn != nil {
		return m.addFn(ctx, ws, actionID, title)
	}
	return &model.ActionStep{ID: "step-1", ActionID: actionID, Title: title}, nil
}

func (m *mockActionStepMutator) SetDone(ctx context.Context, ws string, actionID int64, stepID string, done bool) (*model.ActionStep, error) {
	if m.setDoneFn != nil {
		return m.setDoneFn(ctx, ws, actionID, stepID, done)
	}
	step := &model.ActionStep{ID: stepID, ActionID: actionID}
	if done {
		now := time.Now().UTC()
		step.DoneAt = &now
	}
	return step, nil
}

func (m *mockActionStepMutator) Rename(ctx context.Context, ws string, actionID int64, stepID string, title string) (*model.ActionStep, error) {
	if m.renameFn != nil {
		return m.renameFn(ctx, ws, actionID, stepID, title)
	}
	return &model.ActionStep{ID: stepID, ActionID: actionID, Title: title}, nil
}

func (m *mockActionStepMutator) Delete(ctx context.Context, ws string, actionID int64, stepID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, ws, actionID, stepID)
	}
	return nil
}

func depsForStepTest(stepUC core.ActionStepMutator) core.Deps {
	return core.Deps{
		Repo:         newMockRepo(nil),
		WorkspaceID:  testWorkspaceID,
		CaseID:       testCaseID,
		ActionUC:     &mockActionMutator{},
		ActionStepUC: stepUC,
	}
}

func TestListActionStepsTool(t *testing.T) {
	t.Run("returns ordered steps with progress counters", func(t *testing.T) {
		now := time.Now().UTC()
		mut := &mockActionStepMutator{
			listFn: func(ctx context.Context, ws string, actionID int64) ([]*model.ActionStep, error) {
				gt.Value(t, ws).Equal(testWorkspaceID)
				gt.Value(t, actionID).Equal(int64(42))
				return []*model.ActionStep{
					{ID: "s1", ActionID: 42, Title: "first", DoneAt: &now},
					{ID: "s2", ActionID: 42, Title: "second"},
				}, nil
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__list_action_steps")
		gt.Value(t, tl).NotNil()

		out, err := tl.Run(context.Background(), map[string]any{"action_id": int64(42)})
		gt.NoError(t, err).Required()
		gt.Value(t, out["done"]).Equal(1)
		gt.Value(t, out["total"]).Equal(2)
		gt.Value(t, out["complete"]).Equal(false)
		steps, ok := out["steps"].([]map[string]any)
		gt.Bool(t, ok).True()
		gt.Array(t, steps).Length(2)
		gt.Value(t, steps[0]["id"]).Equal("s1")
		gt.Value(t, steps[0]["done"]).Equal(true)
	})

	t.Run("fails loud when ActionStepUC is nil", func(t *testing.T) {
		deps := depsForStepTest(nil)
		tools := core.New(deps)
		tl := findTool(tools, "core__list_action_steps")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(42)})
		gt.Value(t, err).NotNil()
	})

	t.Run("missing action_id is rejected", func(t *testing.T) {
		tools := core.New(depsForStepTest(&mockActionStepMutator{}))
		tl := findTool(tools, "core__list_action_steps")
		_, err := tl.Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil()
	})
}

func TestAddActionStepTool(t *testing.T) {
	t.Run("routes through mutator with extracted args", func(t *testing.T) {
		var captured struct {
			ws       string
			actionID int64
			title    string
		}
		mut := &mockActionStepMutator{
			addFn: func(ctx context.Context, ws string, actionID int64, title string) (*model.ActionStep, error) {
				captured.ws = ws
				captured.actionID = actionID
				captured.title = title
				return &model.ActionStep{ID: "s-new", ActionID: actionID, Title: title}, nil
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__add_action_step")

		out, err := tl.Run(context.Background(), map[string]any{
			"action_id": int64(7),
			"title":     "review logs",
		})
		gt.NoError(t, err).Required()
		gt.Value(t, captured.ws).Equal(testWorkspaceID)
		gt.Value(t, captured.actionID).Equal(int64(7))
		gt.Value(t, captured.title).Equal("review logs")
		gt.Value(t, out["title"]).Equal("review logs")
	})

	t.Run("missing title is rejected", func(t *testing.T) {
		tools := core.New(depsForStepTest(&mockActionStepMutator{}))
		tl := findTool(tools, "core__add_action_step")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(7)})
		gt.Value(t, err).NotNil()
	})

	t.Run("propagates mutator error", func(t *testing.T) {
		mut := &mockActionStepMutator{
			addFn: func(ctx context.Context, ws string, actionID int64, title string) (*model.ActionStep, error) {
				return nil, errors.New("boom")
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__add_action_step")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(7), "title": "x"})
		gt.Value(t, err).NotNil()
	})
}

func TestSetActionStepDoneTool(t *testing.T) {
	t.Run("passes through done flag", func(t *testing.T) {
		var got bool
		mut := &mockActionStepMutator{
			setDoneFn: func(ctx context.Context, ws string, actionID int64, stepID string, done bool) (*model.ActionStep, error) {
				got = done
				return &model.ActionStep{ID: stepID, ActionID: actionID}, nil
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__set_action_step_done")
		_, err := tl.Run(context.Background(), map[string]any{
			"action_id": int64(1),
			"step_id":   "abc",
			"done":      true,
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, got).True()
	})

	t.Run("missing step_id is rejected", func(t *testing.T) {
		tools := core.New(depsForStepTest(&mockActionStepMutator{}))
		tl := findTool(tools, "core__set_action_step_done")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(1), "done": true})
		gt.Value(t, err).NotNil()
	})

	t.Run("missing done is rejected", func(t *testing.T) {
		tools := core.New(depsForStepTest(&mockActionStepMutator{}))
		tl := findTool(tools, "core__set_action_step_done")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(1), "step_id": "abc"})
		gt.Value(t, err).NotNil()
	})
}

func TestRenameActionStepTool(t *testing.T) {
	t.Run("passes new title through", func(t *testing.T) {
		var newTitle string
		mut := &mockActionStepMutator{
			renameFn: func(ctx context.Context, ws string, actionID int64, stepID string, title string) (*model.ActionStep, error) {
				newTitle = title
				return &model.ActionStep{ID: stepID, ActionID: actionID, Title: title}, nil
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__rename_action_step")
		_, err := tl.Run(context.Background(), map[string]any{
			"action_id": int64(1),
			"step_id":   "abc",
			"title":     "renamed",
		})
		gt.NoError(t, err).Required()
		gt.Value(t, newTitle).Equal("renamed")
	})

	t.Run("missing title is rejected", func(t *testing.T) {
		tools := core.New(depsForStepTest(&mockActionStepMutator{}))
		tl := findTool(tools, "core__rename_action_step")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(1), "step_id": "abc"})
		gt.Value(t, err).NotNil()
	})
}

func TestDeleteActionStepTool(t *testing.T) {
	t.Run("returns deleted=true on success", func(t *testing.T) {
		var got string
		mut := &mockActionStepMutator{
			deleteFn: func(ctx context.Context, ws string, actionID int64, stepID string) error {
				got = stepID
				return nil
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__delete_action_step")
		out, err := tl.Run(context.Background(), map[string]any{
			"action_id": int64(1),
			"step_id":   "abc",
		})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal("abc")
		gt.Value(t, out["deleted"]).Equal(true)
		gt.Value(t, out["step_id"]).Equal("abc")
	})

	t.Run("propagates mutator error", func(t *testing.T) {
		mut := &mockActionStepMutator{
			deleteFn: func(ctx context.Context, ws string, actionID int64, stepID string) error {
				return errors.New("boom")
			},
		}
		tools := core.New(depsForStepTest(mut))
		tl := findTool(tools, "core__delete_action_step")
		_, err := tl.Run(context.Background(), map[string]any{"action_id": int64(1), "step_id": "abc"})
		gt.Value(t, err).NotNil()
	})
}
