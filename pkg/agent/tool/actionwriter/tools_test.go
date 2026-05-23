package actionwriter_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/actionwriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// TestActionWriter_Surface locks the surface of the writer subset: every
// tool that should be present is, and every destructive tool that should
// NOT be present is absent. The Job agent runs unattended; surfacing
// archive / delete tools from the writer subset would let a single bad
// LLM turn destroy work.
func TestActionWriter_Surface(t *testing.T) {
	tools := actionwriter.New(actionwriter.Deps{
		WorkspaceID:  "ws",
		CaseID:       1,
		ActionUC:     stubActionMutator{},
		ActionStepUC: stubActionStepMutator{},
	})

	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Spec().Name] = true
	}

	expectPresent := []string{
		"core__create_action",
		"core__update_action",
		"core__update_action_status",
		"core__set_action_assignee",
		"core__add_action_step",
		"core__set_action_step_done",
		"core__rename_action_step",
	}
	for _, n := range expectPresent {
		gt.Bool(t, names[n]).True()
	}

	expectAbsent := []string{
		"core__archive_action",
		"core__unarchive_action",
		"core__delete_action_step",
		"core__list_actions",
		"core__get_action",
		"core__list_action_steps",
	}
	for _, n := range expectAbsent {
		gt.Bool(t, names[n]).False()
	}

	gt.Array(t, tools).Length(len(expectPresent))
}

type stubActionMutator struct{}

func (stubActionMutator) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error) {
	panic("not used")
}
func (stubActionMutator) UpdateAction(ctx context.Context, workspaceID string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
	panic("not used")
}
func (stubActionMutator) ArchiveAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error) {
	panic("not used")
}
func (stubActionMutator) UnarchiveAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error) {
	panic("not used")
}

type stubActionStepMutator struct{}

func (stubActionStepMutator) List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error) {
	panic("not used")
}
func (stubActionStepMutator) Add(ctx context.Context, workspaceID string, actionID int64, title string) (*model.ActionStep, error) {
	panic("not used")
}
func (stubActionStepMutator) SetDone(ctx context.Context, workspaceID string, actionID int64, stepID string, done bool) (*model.ActionStep, error) {
	panic("not used")
}
func (stubActionStepMutator) Rename(ctx context.Context, workspaceID string, actionID int64, stepID string, title string) (*model.ActionStep, error) {
	panic("not used")
}
func (stubActionStepMutator) Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error {
	panic("not used")
}
