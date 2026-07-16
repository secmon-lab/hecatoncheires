package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestNewCaseMultiCaseAdapter_NilUseCase(t *testing.T) {
	gt.Value(t, usecase.NewCaseMultiCaseAdapter(nil)).Nil()
}

// TestCaseMultiCaseAdapter_CreateCase drives the adapter's CreateCase through
// a real CaseUseCase backed by memory.New() and reads the persisted Case back
// via the repository, verifying the fields the casemulti tool sets survive
// the round trip.
func TestCaseMultiCaseAdapter_CreateCase(t *testing.T) {
	repo := memory.New()
	caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	adapter := usecase.NewCaseMultiCaseAdapter(caseUC)
	gt.Value(t, adapter).NotNil().Required()

	// The workspace-agent host establishes the mentioning user as the ctx
	// auth token before any casemulti call (see wsagent.RunTurn); mirror that
	// here so CreateCase's reporter-from-auth-context logic behaves as it
	// does in production.
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "U-CREATOR"})

	created, err := adapter.CreateCase(ctx, testWorkspaceID, "Cross-case title", "Cross-case description", nil, nil, true)
	gt.NoError(t, err).Required()

	stored, err := repo.Case().Get(ctx, testWorkspaceID, created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.Title).Equal("Cross-case title")
	gt.Value(t, stored.Description).Equal("Cross-case description")
	gt.Bool(t, stored.IsPrivate).True()
	gt.Value(t, stored.ReporterID).Equal("U-CREATOR")
}

func TestNewCaseMultiActionAdapter_NilUseCase(t *testing.T) {
	repo := memory.New()
	actionUC := usecase.NewActionUseCase(repo, nil, nil, "", nil)
	stepUC := usecase.NewActionStepUseCase(repo, nil, nil)

	gt.Value(t, usecase.NewCaseMultiActionAdapter(nil, nil)).Nil()
	gt.Value(t, usecase.NewCaseMultiActionAdapter(actionUC, nil)).Nil()
	gt.Value(t, usecase.NewCaseMultiActionAdapter(nil, stepUC)).Nil()
}

// casemultiActionFixture wires a real CaseUseCase / ActionUseCase /
// ActionStepUseCase against memory.New() and pre-creates one Case and one
// Action, so the casemulti action-adapter tests exercise real persistence
// rather than a hand-rolled double.
type casemultiActionFixture struct {
	repo     *memory.Memory
	actionUC *usecase.ActionUseCase
	stepUC   *usecase.ActionStepUseCase
	adapter  casemulti.ActionUsecase
	caseID   int64
	actionID int64
}

func newCasemultiActionFixture(t *testing.T) *casemultiActionFixture {
	t.Helper()

	repo := memory.New()
	caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	actionUC := usecase.NewActionUseCase(repo, nil, nil, "", nil)
	stepUC := usecase.NewActionStepUseCase(repo, nil, nil)

	// Case creation needs a reporter (channel-mode Case), so the setup runs
	// under an authenticated ctx. The actual adapter calls under test use
	// their own explicit actorID argument instead.
	setupCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "U-CREATOR"})
	c, err := caseUC.CreateCase(setupCtx, testWorkspaceID, "Cross-case action host", "", nil, nil, false, false, "", "")
	gt.NoError(t, err).Required()

	action, err := actionUC.CreateAction(setupCtx, testWorkspaceID, c.ID, "Original title", "original description", "", "", types.ActionStatusTodo, nil)
	gt.NoError(t, err).Required()

	adapter := usecase.NewCaseMultiActionAdapter(actionUC, stepUC)
	gt.Value(t, adapter).NotNil().Required()

	return &casemultiActionFixture{
		repo:     repo,
		actionUC: actionUC,
		stepUC:   stepUC,
		adapter:  adapter,
		caseID:   c.ID,
		actionID: action.ID,
	}
}

// TestCaseMultiActionAdapter_UpdateAction_AttributesToSlackUser verifies the
// adapter's documented contract (casemulti_tool_adapter.go): every
// workspace-agent write is attributed to the mentioning Slack user via
// ActorRef{Kind: ActorKindSlackUser, ID: actorID} — never ActorKindSystem.
// recordActionEvents (pkg/usecase/action.go) writes ActionEvent.ActorID from
// exactly that ActorRef, leaving it empty ("") only for ActorKindSystem, so a
// non-empty, matching ActorID on the persisted event is the observable proof
// of SlackUser attribution.
func TestCaseMultiActionAdapter_UpdateAction_AttributesToSlackUser(t *testing.T) {
	f := newCasemultiActionFixture(t)

	// Mirrors the real turn context (auth.ContextWithToken with the
	// mentioning user) plus the same id passed as actorID, exactly as
	// wsagent.RunTurn wires it.
	const actorID = "U-HUMAN"
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: actorID})

	newTitle := "Updated by workspace agent"
	newStatus := types.ActionStatusInProgress
	updated, err := f.adapter.UpdateAction(ctx, testWorkspaceID, f.actionID, casemulti.ActionUpdate{
		Title:  &newTitle,
		Status: &newStatus,
	}, actorID)
	gt.NoError(t, err).Required()
	gt.Value(t, updated.Title).Equal(newTitle)
	gt.Value(t, updated.Status).Equal(newStatus)

	// Persisted state reflects the change.
	stored, err := f.repo.Action().Get(ctx, testWorkspaceID, f.actionID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.Title).Equal(newTitle)
	gt.Value(t, stored.Status).Equal(newStatus)

	// The change history (ActionEvent) attributes both diffs to the Slack
	// user actorID — never the empty string ActorKindSystem would record.
	events, _, err := f.repo.ActionEvent().List(ctx, testWorkspaceID, f.actionID, 10, "")
	gt.NoError(t, err).Required()

	foundTitleChanged := false
	foundStatusChanged := false
	for _, ev := range events {
		switch ev.Kind {
		case types.ActionEventTitleChanged:
			foundTitleChanged = true
			gt.Value(t, ev.ActorID).Equal(actorID)
			gt.Value(t, ev.ActorID).NotEqual("") // contrast: ActorKindSystem would record ""
			gt.Value(t, ev.OldValue).Equal("Original title")
			gt.Value(t, ev.NewValue).Equal(newTitle)
		case types.ActionEventStatusChanged:
			foundStatusChanged = true
			gt.Value(t, ev.ActorID).Equal(actorID)
			gt.Value(t, ev.ActorID).NotEqual("")
		}
	}
	gt.Bool(t, foundTitleChanged).True()
	gt.Bool(t, foundStatusChanged).True()
}

// TestCaseMultiActionAdapter_AddActionStep verifies the step is persisted and
// the actorID is recorded as the creator (ActionStep.CreatedBy), mirroring
// actionIdentifier's SlackUser-actor path.
func TestCaseMultiActionAdapter_AddActionStep(t *testing.T) {
	f := newCasemultiActionFixture(t)

	const actorID = "U-HUMAN"
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: actorID})

	step, err := f.adapter.AddActionStep(ctx, testWorkspaceID, f.actionID, "Investigate root cause", actorID)
	gt.NoError(t, err).Required()
	gt.Value(t, step.Title).Equal("Investigate root cause")
	gt.Value(t, step.CreatedBy).Equal(actorID)
	gt.Bool(t, step.IsDone()).False()

	stored, err := f.repo.ActionStep().Get(ctx, testWorkspaceID, f.actionID, step.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.Title).Equal("Investigate root cause")
	gt.Value(t, stored.CreatedBy).Equal(actorID)
	gt.Bool(t, stored.IsDone()).False()
}

// TestCaseMultiActionAdapter_SetActionStepDone verifies the adapter marks a
// step done/undone and records the actorID as DoneBy.
func TestCaseMultiActionAdapter_SetActionStepDone(t *testing.T) {
	f := newCasemultiActionFixture(t)

	const actorID = "U-HUMAN"
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: actorID})

	step, err := f.adapter.AddActionStep(ctx, testWorkspaceID, f.actionID, "Investigate root cause", actorID)
	gt.NoError(t, err).Required()

	done, err := f.adapter.SetActionStepDone(ctx, testWorkspaceID, f.actionID, step.ID, true, actorID)
	gt.NoError(t, err).Required()
	gt.Bool(t, done.IsDone()).True()
	gt.Value(t, done.DoneBy).Equal(actorID)

	stored, err := f.repo.ActionStep().Get(ctx, testWorkspaceID, f.actionID, step.ID)
	gt.NoError(t, err).Required()
	gt.Bool(t, stored.IsDone()).True()
	gt.Value(t, stored.DoneBy).Equal(actorID)

	// Toggling back to not-done clears DoneBy.
	reopened, err := f.adapter.SetActionStepDone(ctx, testWorkspaceID, f.actionID, step.ID, false, actorID)
	gt.NoError(t, err).Required()
	gt.Bool(t, reopened.IsDone()).False()
	gt.Value(t, reopened.DoneBy).Equal("")
}
