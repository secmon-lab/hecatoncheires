package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/llm/claude"
	"github.com/gollem-dev/gollem/llm/gemini"
	"github.com/gollem-dev/gollem/llm/openai"
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
	goslack "github.com/slack-go/slack"
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

// Thread-mode manages no Actions, so the planner is offered no core (action)
// toolset; the read-only Slack toolset stands in.
const tcInvestigatePlan = `{"message":"investigate","tasks":[{"id":"t-1","title":"Review","description":"Review the thread","acceptance_criteria":"done","tools":["slack_ro"]}]}`

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

	// The create agent committed a case bound to the thread, carrying the
	// validated fields (creation was deferred until a valid create decision).
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.Description).Equal("Users cannot log in.")
	gt.Value(t, c.BoardStatus).Equal("TRIAGE")
	gt.Value(t, c.ReporterID).Equal("U-REPORTER")
	gt.Value(t, c.SlackThreadTS).Equal("1700000000.000100")
	gt.Value(t, c.FieldValues["severity"].Value).Equal("high")

	// The Block Kit summary (carrying the web-UI link) was posted to the thread.
	foundSummary := false
	for _, m := range slackMock.postedMessages {
		if strings.Contains(m.Text, "https://app.test/ws/support/cases/") {
			foundSummary = true
		}
	}
	gt.Bool(t, foundSummary).True()
}

func TestFirstSlackUserMention(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		ignore []string
		want   string
	}{
		{"plain mention", "ping <@U123ABC> please", nil, "U123ABC"},
		{"mention with label", "Reporter: <@U06KHSXQW4V|ahyan/HP> here", nil, "U06KHSXQW4V"},
		{"enterprise W id", "owner <@W99TEAM01|grid>", nil, "W99TEAM01"},
		{"first of several", "<@U001> cc <@U002>", nil, "U001"},
		{"none", "no users here, just text", nil, ""},
		{"empty", "", nil, ""},
		// The first mention is the bot itself; it is skipped so the requester
		// named next becomes the reporter.
		{"ignore first (bot mentioned before requester)", "<@UBOT001> request from <@U002>", []string{"UBOT001"}, "U002"},
		{"ignore all", "<@U001> cc <@U002>", []string{"U001", "U002"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gt.Value(t, usecase.FirstSlackUserMentionForTest(tc.text, tc.ignore...)).Equal(tc.want)
		})
	}
}

// A channel-root post relayed by an integration bot (no human author) creates a
// case attributed to the human named in the body — the first Slack mention.
func TestThreadCase_Creation_BotRelayedReporter(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	llm := newScriptedClient([]string{
		tcInvestigatePlan,
		"The form requests an API risk review.",
		tcReplanDone,
		`{"kind":"materialize","title":"Backlog API risk review","description":"Review the Backlog API usage.","fields":[{"field_id":"severity","value":"low"}]}`,
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

	// Bot-authored form post: empty user ID. The body @-mentions the bot itself
	// (UBOT001, the agentTestSlackService bot user) BEFORE the requester, so the
	// resolver must skip the bot and attribute the case to the requester.
	threadTS := "1700000000.000700"
	msg := slackmodel.NewMessageFromData(
		threadTS, "C-MONITOR", "", "T1", "", "",
		"<@UBOT001> RISK NAVIGATOR request\nReporter: <@U06KHSXQW4V|ahyan>\nReview the Backlog API usage.",
		threadTS, time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, msg, entry)).Required()
	async.Wait()

	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", threadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Backlog API risk review")
	// The reporter was resolved from the body mention, skipping the bot's own ID
	// and the (bot) author.
	gt.Value(t, c.ReporterID).Equal("U06KHSXQW4V")
	gt.Value(t, c.SlackThreadTS).Equal(threadTS)
}

// A bot-authored post with no human mention in the body still creates a case;
// the thread-mode case is simply persisted with an empty reporter.
func TestThreadCase_Creation_BotNoReporterEmpty(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	llm := newScriptedClient([]string{
		tcInvestigatePlan,
		"An automated heartbeat with no named requester.",
		tcReplanDone,
		`{"kind":"materialize","title":"Heartbeat case","description":"Automated, no requester.","fields":[{"field_id":"severity","value":"low"}]}`,
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

	// Bot-authored post: empty user ID and no Slack mention anywhere in the body.
	threadTS := "1700000000.000800"
	msg := slackmodel.NewMessageFromData(
		threadTS, "C-MONITOR", "", "T1", "", "",
		"automated heartbeat, no requester named", threadTS, time.Now(), nil)

	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, msg, entry)).Required()
	async.Wait()

	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", threadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Heartbeat case")
	// No human could be resolved, so the reporter is empty (allowed in thread mode).
	gt.Value(t, c.ReporterID).Equal("")
	gt.Value(t, c.SlackThreadTS).Equal(threadTS)
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

// TestLifecycle_ThreadCaseCreate_QuestionResume drives the full deferred-create
// lifecycle through the public entry points: the initial post makes the agent
// ask a question (no case yet), then a thread reply resumes the create agent
// which commits the case. History continuity is exercised implicitly (the same
// thread session spans both turns).
func TestLifecycle_ThreadCaseCreate_QuestionResume(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	llm := newScriptedClient([]string{
		// Turn 1 (create): plan -> sub-agent -> replan asks a question.
		tcInvestigatePlan,
		"Need to know the severity before creating the case.",
		`{"message":"need info","question":{"reason":"What severity?","items":[{"id":"q1","text":"Severity?","type":"select","options":["high","low"]}]}}`,
		// Turn 2 (resume): plan -> sub-agent -> replan done -> create.
		tcInvestigatePlan,
		"The reporter says it is high severity.",
		tcReplanDone,
		`{"title":"Login outage","description":"Users cannot log in.","fields":[{"field_id":"severity","value":"high"}]}`,
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

	// Turn 1: the initial post. The agent should ask a question and NOT create.
	post := slackmodel.NewMessageFromData(
		"1700000000.000100", "C-MONITOR", "", "T1", "U-REPORTER", "alice",
		"Something is wrong with the portal", "1700000000.000100", time.Now(), nil)
	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, post, entry)).Required()
	async.Wait()

	noCase, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, noCase).Nil() // deferred: no case while the question is pending

	ssn, err := repo.Session().GetByThread(ctx, "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.LastAction).Equal(model.SessionEndedWithQuestion)
	sessionIDTurn1 := ssn.ID

	// Turn 2: a thread reply answers the question and resumes the create agent.
	reply := slackmodel.NewMessageFromData(
		"1700000000.000201", "C-MONITOR", "1700000000.000100", "T1", "U-REPORTER", "alice",
		"high", "1700000000.000201", time.Now(), nil)
	gt.NoError(t, agentUC.ResumeThreadCaseCreation(ctx, reply, entry)).Required()
	async.Wait()

	// The case now exists with the validated fields.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.ReporterID).Equal("U-REPORTER")
	gt.Value(t, c.FieldValues["severity"].Value).Equal("high")

	// History continuity: the same thread session was reused across both turns,
	// and it is now bound to the created case (Session.ID unchanged).
	ssn2, err := repo.Session().GetByThread(ctx, "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, ssn2.ID).Equal(sessionIDTurn1)
	gt.Number(t, ssn2.CaseID).Equal(c.ID)
}

// realLLMForThreadCreate builds a real LLM client for the gated thread-create
// test. The dedicated gate is TEST_THREAD_CREATE; the client itself is built
// from the same TEST_LLM_* env vars the eval harness uses.
func realLLMForThreadCreate(t *testing.T) gollem.LLMClient {
	t.Helper()
	if os.Getenv("TEST_THREAD_CREATE") == "" {
		t.Skip("TEST_THREAD_CREATE not set; skipping real-LLM thread-create test")
	}
	ctx := context.Background()
	model := os.Getenv("TEST_LLM_MODEL")
	switch os.Getenv("TEST_LLM_PROVIDER") {
	case "openai":
		key := os.Getenv("TEST_LLM_OPENAI_API_KEY")
		gt.Value(t, key).NotEqual("")
		var opts []openai.Option
		if model != "" {
			opts = append(opts, openai.WithModel(model))
		}
		c, err := openai.New(ctx, key, opts...)
		gt.NoError(t, err).Required()
		return c
	case "claude":
		key := os.Getenv("TEST_LLM_CLAUDE_API_KEY")
		project := os.Getenv("TEST_LLM_GEMINI_PROJECT_ID")
		switch {
		case key != "":
			var opts []claude.Option
			if model != "" {
				opts = append(opts, claude.WithModel(model))
			}
			c, err := claude.New(ctx, key, opts...)
			gt.NoError(t, err).Required()
			return c
		case project != "":
			location := os.Getenv("TEST_LLM_GEMINI_LOCATION")
			gt.Value(t, location).NotEqual("")
			var opts []claude.VertexOption
			if model != "" {
				opts = append(opts, claude.WithVertexModel(model))
			}
			c, err := claude.NewWithVertex(ctx, location, project, opts...)
			gt.NoError(t, err).Required()
			return c
		default:
			t.Skip("claude provider needs TEST_LLM_CLAUDE_API_KEY or TEST_LLM_GEMINI_PROJECT_ID")
			return nil
		}
	case "gemini":
		project := os.Getenv("TEST_LLM_GEMINI_PROJECT_ID")
		location := os.Getenv("TEST_LLM_GEMINI_LOCATION")
		gt.Value(t, project).NotEqual("")
		gt.Value(t, location).NotEqual("")
		var opts []gemini.Option
		if model != "" {
			opts = append(opts, gemini.WithModel(model))
		}
		c, err := gemini.New(ctx, project, location, opts...)
		gt.NoError(t, err).Required()
		return c
	default:
		t.Skip("TEST_LLM_PROVIDER must be openai | claude | gemini")
		return nil
	}
}

// TestRealLLM_ThreadCaseCreate_VagueToCreate reproduces the target use case
// against a real model: a vague initial message, after which the agent does
// light investigation, asks the user to clarify intent, then (given the answer)
// commits a validated case. The required `severity` field can only be filled
// from the user's answer, so a well-behaved agent must ask before it can
// create. The test asserts the *wiring contract* — a valid case is eventually
// created and a summary is posted — not the wording quality.
func TestRealLLM_ThreadCaseCreate_VagueToCreate(t *testing.T) {
	llm := realLLMForThreadCreate(t)
	ctx := context.Background()

	repo := memory.New()
	set, err := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	gt.NoError(t, err).Required()
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		CaseStatusSet:         set,
		CaseCreatePrompt:      "If the severity is unclear from the message, ask the reporter before creating the case.",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Description: "How severe the issue is.", Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	})
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
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

	const channel = "C-MONITOR"
	const rootTS = "1700000000.000100"
	post := slackmodel.NewMessageFromData(rootTS, channel, "", "T1", "U-REPORTER", "alice",
		"hey, something seems off with the portal, can you take a look?", rootTS, time.Now(), nil)
	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, post, entry)).Required()
	async.Wait()

	// Up to a few resume turns: answer any pending question with a concrete
	// reply that supplies the severity, until the case is created.
	const maxTurns = 4
	for turn := 0; turn < maxTurns; turn++ {
		if c, _ := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS); c != nil {
			break
		}
		ssn, serr := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, serr).Required()
		if ssn == nil || ssn.LastAction != model.SessionEndedWithQuestion {
			break
		}
		replyTS := time.Now().Format("1700000000.000201")
		reply := slackmodel.NewMessageFromData(replyTS, channel, rootTS, "T1", "U-REPORTER", "alice",
			"It's high severity — the production portal login returns a 503 for everyone.", replyTS, time.Now(), nil)
		gt.NoError(t, agentUC.ResumeThreadCaseCreation(ctx, reply, entry)).Required()
		async.Wait()
	}

	c, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.String(t, c.Title).NotEqual("")
	// The required field must be satisfied and within the allowed option set.
	gt.Value(t, c.FieldValues["severity"].Value).Equal("high")

	// A summary carrying the web-UI link was posted.
	foundSummary := false
	for _, m := range slackMock.postedMessages {
		if strings.Contains(m.Text, "https://app.test/ws/support/cases/") {
			foundSummary = true
		}
	}
	gt.Bool(t, foundSummary).True()
}

// TestThreadCase_QuestionSubmit drives the interactive question form: the
// create agent asks a question (posting a Block Kit form + persisting the
// snapshot), the user submits an answer, and the resumed agent commits the
// case. Asserts the submit handler clears PendingQuestion, rewrites the form
// into the answered view, and creates the case from the answer.
func TestThreadCase_QuestionSubmit(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	llm := newScriptedClient([]string{
		// Turn 1 (create): plan -> sub-agent -> ask a question.
		tcInvestigatePlan,
		"Need the severity.",
		`{"message":"need info","question":{"reason":"What severity?","items":[{"id":"q-sev","text":"Severity?","type":"select","options":["high","low"]}]}}`,
		// Turn 2 (resume after submit): plan -> sub-agent -> replan done -> create.
		tcInvestigatePlan,
		"Reporter said high.",
		tcReplanDone,
		`{"title":"Login outage","description":"Users cannot log in.","fields":[{"field_id":"severity","value":"high"}]}`,
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

	const channel = "C-MONITOR"
	const rootTS = "1700000000.000100"
	post := slackmodel.NewMessageFromData(rootTS, channel, "", "T1", "U-REPORTER", "alice",
		"portal seems broken", rootTS, time.Now(), nil)
	gt.NoError(t, agentUC.HandleThreadCaseCreation(ctx, post, entry)).Required()
	async.Wait()

	// No case yet; the form was posted and the snapshot persisted.
	noCase, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, noCase).Nil()
	ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn.PendingQuestion).NotNil().Required()
	formTS := ssn.PendingQuestion.PostedMessageTS
	gt.String(t, formTS).NotEqual("")

	// The user submits "high".
	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U-REPORTER"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channel}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: rootTS}},
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
					usecase.ActionIDDraftQuestionChoice: {SelectedOption: goslack.OptionBlockObject{Value: "high"}},
				},
			},
		},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: rootTS}},
		},
	}
	gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	// The case was created from the answer; PendingQuestion cleared.
	c, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.FieldValues["severity"].Value).Equal("high")

	ssn2, err := repo.Session().GetByThread(ctx, channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn2.PendingQuestion).Nil()
	gt.Number(t, ssn2.CaseID).Equal(c.ID)

	// The form message was rewritten (answered view) via UpdateMessage.
	gt.Bool(t, len(slackMock.updatedMessages) >= 1).True()
}

// TestThreadCase_QuestionSubmit_StaleAfterCreate verifies a late submit on a
// thread whose case already exists is dropped (the form is marked stale, no
// second case).
func TestThreadCase_QuestionSubmit_StaleAfterCreate(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          newScriptedClient(nil),
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})

	const channel = "C-MONITOR"
	const rootTS = "1700000000.000100"
	// Pre-existing case for the thread.
	_, err := caseUC.CreateThreadCase(ctx, "support", channel, rootTS, "U-REPORTER", "seed", "seed")
	gt.NoError(t, err).Required()

	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U-REPORTER"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channel}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: "1700000000.000900", ThreadTimestamp: rootTS}},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: rootTS}},
		},
	}
	gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	// The stale form was rewritten; no duplicate case.
	gt.Bool(t, len(slackMock.updatedMessages) >= 1).True()
}
