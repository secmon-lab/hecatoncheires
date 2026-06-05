package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// newScriptedClient returns a mock LLM that pops the supplied responses in
// order across every Generate call (planner + sub-agent + final phase).
func newScriptedClient(scripts []string) gollem.LLMClient {
	var mu sync.Mutex
	idx := 0
	return &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					mu.Lock()
					defer mu.Unlock()
					if idx >= len(scripts) {
						return nil, errors.New("no more scripted responses")
					}
					out := scripts[idx]
					idx++
					return &gollem.Response{Texts: []string{out}}, nil
				},
			}, nil
		},
	}
}

const tcInvestigatePlan = `{"message":"investigate","tasks":[{"id":"t-1","title":"Review","description":"Review the thread","acceptance_criteria":"done","tools":["core_ro"]}]}`

const tcReplanDone = `{"message":"done","tasks":[]}`

func newThreadWorkspaceRegistry() *model.WorkspaceRegistry {
	set, _ := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		CaseStatusSet:         set,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	})
	return reg
}

func TestThreadCase_Creation(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	llm := newScriptedClient([]string{
		tcInvestigatePlan,
		"The post reports a login outage.",
		tcReplanDone,
		`{"kind":"materialize","title":"Login outage","description":"Users cannot log in.","fields":[{"field_id":"severity","value":"high"}]}`,
	})

	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})

	entry, err := reg.Get("support")
	gt.NoError(t, err).Required()

	msg := slackmodel.NewMessageFromData(
		"1700000000.000100", "C-MONITOR", "", "T1", "U-REPORTER", "alice",
		"Cannot log in to the portal", "1700000000.000100", time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, msg, entry)).Required()
	async.Wait()

	// A case was created, bound to the thread, with materialized fields.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.Description).Equal("Users cannot log in.")
	gt.Value(t, c.BoardStatus).Equal("TRIAGE")
	gt.Value(t, c.ReporterID).Equal("U-REPORTER")
	gt.Value(t, c.SlackThreadTS).Equal("1700000000.000100")
	gt.Value(t, c.FieldValues["severity"].Value).Equal("high")

	// The creation ack (with web-UI link) was posted to the thread.
	foundAck := false
	for _, m := range slackMock.postedMessages {
		if strings.Contains(m.Text, "https://app.test/ws/support/cases/") {
			foundAck = true
		}
	}
	gt.Bool(t, foundAck).True()
}

func TestThreadCase_Creation_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	entry, err := reg.Get("support")
	gt.NoError(t, err).Required()

	// Pre-create the thread case so the handler must short-circuit.
	_, err = caseUC.CreateThreadCase(ctx, "support", "C-MONITOR", "1700000000.000100", "U-REPORTER", "seed", "seed body")
	gt.NoError(t, err).Required()

	// An LLM that errors on first call proves materialize never runs.
	llm := newScriptedClient(nil)
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})

	msg := slackmodel.NewMessageFromData(
		"1700000000.000100", "C-MONITOR", "", "T1", "U-REPORTER", "alice",
		"duplicate delivery", "1700000000.000100", time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, msg, entry)).Required()
	async.Wait()
	gt.Array(t, slackMock.postedMessages).Length(0)
}

func TestThreadCase_MentionRespond(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	slackMock.getConversationRepliesFn = func(_ context.Context, _ string, _ string, _ int) ([]slack.ConversationMessage, error) {
		return []slack.ConversationMessage{
			{UserID: "U-REPORTER", UserName: "alice", Text: "Cannot log in", Timestamp: "1700000000.000100"},
		}, nil
	}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	entry, err := reg.Get("support")
	gt.NoError(t, err).Required()

	c, err := caseUC.CreateThreadCase(ctx, "support", "C-MONITOR", "1700000000.000100", "U-REPORTER", "Login outage", "body")
	gt.NoError(t, err).Required()

	llm := newScriptedClient([]string{
		tcInvestigatePlan,
		"Reviewed the thread.",
		tcReplanDone,
		`{"kind":"respond","message":"The team is investigating; ETA 1 hour."}`,
	})
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})

	msg := slackmodel.NewMessageFromData(
		"1700000005.000001", "C-MONITOR", "1700000000.000100", "T1", "U-ASKER", "bob",
		"<@UBOT001> any update?", "1700000005.000001", time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseMention(ctx, msg, entry, c)).Required()
	async.Wait()

	foundReply := false
	for _, m := range slackMock.postedMessages {
		if strings.Contains(m.Text, "ETA 1 hour") {
			foundReply = true
		}
	}
	gt.Bool(t, foundReply).True()
}

func TestThreadCase_MentionClose(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	slackMock.getConversationRepliesFn = func(_ context.Context, _ string, _ string, _ int) ([]slack.ConversationMessage, error) {
		return nil, nil
	}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	entry, err := reg.Get("support")
	gt.NoError(t, err).Required()

	c, err := caseUC.CreateThreadCase(ctx, "support", "C-MONITOR", "1700000000.000100", "U-REPORTER", "Login outage", "body")
	gt.NoError(t, err).Required()

	llm := newScriptedClient([]string{
		tcInvestigatePlan,
		"The thread says it is resolved.",
		tcReplanDone,
		`{"kind":"close","message":"Resolved.","close_status":"DONE"}`,
	})
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})

	msg := slackmodel.NewMessageFromData(
		"1700000006.000001", "C-MONITOR", "1700000000.000100", "T1", "U-ASKER", "bob",
		"<@UBOT001> please close", "1700000006.000001", time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseMention(ctx, msg, entry, c)).Required()
	async.Wait()

	closed, err := repo.Case().Get(ctx, "support", c.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, closed.BoardStatus).Equal("DONE")
	gt.Value(t, closed.Status).Equal(types.CaseStatusClosed)
}
