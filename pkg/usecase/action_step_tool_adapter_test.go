package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestNewActionStepToolAdapter_NilUseCase(t *testing.T) {
	gt.Value(t, usecase.NewActionStepToolAdapter(nil)).Nil()
}

func TestActionStepToolAdapter_PinsSystemActor(t *testing.T) {
	i18n.Init(i18n.LangEN)

	f := newStepTestFixture(t, nil, false)
	stepUC := usecase.NewActionStepUseCase(f.repo, nil, nil)
	adapter := usecase.NewActionStepToolAdapter(stepUC)
	gt.Value(t, adapter).NotNil()

	// Bypass auth context entirely — this is the LLM tool path.
	ctx := context.Background()

	step, err := adapter.Add(ctx, testWorkspaceID, f.action.ID, "system step")
	gt.NoError(t, err).Required()
	// CreatedBy is empty when the actor is system (no token, ActorKindSystem).
	gt.Value(t, step.CreatedBy).Equal("")

	stored, err := f.repo.ActionStep().Get(ctx, testWorkspaceID, f.action.ID, step.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.CreatedBy).Equal("")

	// SetDone via adapter must also keep DoneBy empty.
	done, err := adapter.SetDone(ctx, testWorkspaceID, f.action.ID, step.ID, true)
	gt.NoError(t, err).Required()
	gt.Value(t, done.DoneBy).Equal("")
	gt.Bool(t, done.IsDone()).True()

	// Rename via adapter.
	renamed, err := adapter.Rename(ctx, testWorkspaceID, f.action.ID, step.ID, "renamed by system")
	gt.NoError(t, err).Required()
	gt.Value(t, renamed.Title).Equal("renamed by system")

	// List works through the adapter.
	listed, err := adapter.List(ctx, testWorkspaceID, f.action.ID)
	gt.NoError(t, err).Required()
	gt.Array(t, listed).Length(1)

	// Delete via adapter.
	gt.NoError(t, adapter.Delete(ctx, testWorkspaceID, f.action.ID, step.ID)).Required()
	listed, err = adapter.List(ctx, testWorkspaceID, f.action.ID)
	gt.NoError(t, err).Required()
	gt.Array(t, listed).Length(0)
}
