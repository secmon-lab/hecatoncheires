package wsagent_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
)

func newWsSession() *model.Session {
	return &model.Session{
		ID:          "s-ws-" + time.Now().Format("150405.000000"),
		ChannelID:   "C-WORKSPACE",
		ThreadTS:    "1700000000.000300",
		WorkspaceID: "acme",
		CaseID:      0, // workspace-scoped: not bound to any single case
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func newWsWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace:               model.Workspace{ID: "acme", Name: "Acme Corp"},
		CaseMode:                model.CaseModeChannel,
		SlackWorkspaceChannelID: "C-WORKSPACE",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

// newWsCaseStatusSet is the board status set a thread-mode workspace carries
// (its Kanban columns), with "done" configured as the closed status.
func newWsCaseStatusSet(t *testing.T) *model.ActionStatusSet {
	t.Helper()
	set, err := model.NewActionStatusSet("todo", []string{"done"}, []model.ActionStatusDefinition{
		{ID: "todo", Name: "To Do"},
		{ID: "doing", Name: "Doing"},
		{ID: "done", Name: "Done"},
	})
	gt.NoError(t, err).Required()
	return set
}

// newWsThreadWorkspace is the thread-mode counterpart of newWsWorkspace: the
// workspace agent runs in the monitored channel, and cases are threads there.
func newWsThreadWorkspace(t *testing.T) *model.WorkspaceEntry {
	t.Helper()
	ws := newWsWorkspace()
	ws.CaseMode = model.CaseModeThread
	ws.SlackWorkspaceChannelID = ""
	ws.SlackMonitorChannelID = "C-WORKSPACE"
	ws.CaseStatusSet = newWsCaseStatusSet(t)
	return ws
}

// A turn missing any of these cannot run: the Session is the turn's subject, the
// workspace supplies the prompt, and without the actor the usecase layer reads
// the run as a system context and bypasses private-case access control.
//
// Which TOOLS the run then gets — the thread-mode / channel-mode split that used
// to be decided per turn here — is now the kernel tool factory's contract, and is
// pinned in pkg/agent/kernel/tools_test.go.
func TestValidateRequest(t *testing.T) {
	validSession := newWsSession()
	validWorkspace := newWsWorkspace()

	t.Run("NilRequest", func(t *testing.T) {
		gt.Error(t, wsagent.ValidateRequestForTest(nil))
	})

	t.Run("NilSession", func(t *testing.T) {
		req := &wsagent.TurnRequest{Workspace: validWorkspace, ActorID: "U-ASKER"}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("NilWorkspace", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, ActorID: "U-ASKER"}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("EmptyActorID", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, Workspace: validWorkspace}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("FullyPopulatedRequestIsValid", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, Workspace: validWorkspace, ActorID: "U-ASKER"}
		gt.NoError(t, wsagent.ValidateRequestForTest(req))
	})
}
