package usecase_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

func TestAssistUseCase_BuildAssistSystemPrompt(t *testing.T) {
	t.Run("renders template with all sections including DueDate", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create a case
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Security Incident", "Suspicious login detected", []string{}, nil, false, false, "", "")
		gt.NoError(t, err).Required()

		// Create actions with and without DueDate
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "", nil)
		dueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Investigate logs", "Check auth logs", "U001", "", types.ActionStatusInProgress, &dueDate)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Update firewall", "Block suspicious IP", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Create workspace entry with assist prompt
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Check deadlines and follow up on pending items.",
		})

		assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		// Verify template renders correctly
		gt.Value(t, strings.Contains(prompt, "Security Incident")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Suspicious login detected")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Investigate logs")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Update firewall")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Due: 2026-03-15")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Check deadlines and follow up on pending items.")).Equal(true)
	})

	t.Run("renders template with no actions or messages", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Empty Case", "No actions yet", []string{}, nil, false, false, "", "")
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Monitor this case.",
		})

		assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "Empty Case")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Monitor this case.")).Equal(true)
		// Actions section should not appear
		gt.Value(t, strings.Contains(prompt, "## Actions")).Equal(false)
	})

	t.Run("renders template with assist logs", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Test", []string{}, nil, false, false, "", "")
		gt.NoError(t, err).Required()

		// Create an assist log
		assistLog := &model.AssistLog{
			CaseID:    c.ID,
			Summary:   "Reviewed deadlines and followed up",
			Actions:   "Checked deadlines",
			Reasoning: "Two actions were overdue",
			NextSteps: "Follow up next week",
		}
		_, err = repo.AssistLog().Create(ctx, testWorkspaceID, c.ID, assistLog)
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Assist this case.",
		})

		assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "Reviewed deadlines and followed up")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Checked deadlines")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Two actions were overdue")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Follow up next week")).Equal(true)
	})
}

func TestAssistUseCase_BuildAssistSystemPrompt_Language(t *testing.T) {
	t.Run("includes language instruction when language is set", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, false, "", "")
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:      model.Workspace{ID: testWorkspaceID, Name: "Test"},
			AssistPrompt:   "Check deadlines.",
			AssistLanguage: "Japanese",
		})

		assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "## Language")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "You MUST respond and write all messages in Japanese.")).Equal(true)
	})

	t.Run("omits language section when language is empty", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, false, "", "")
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test"},
			AssistPrompt: "Check deadlines.",
		})

		assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "## Language")).Equal(false)
	})
}

// wireAssistRuntime reproduces cmdAssist's wiring order — register, build, bind —
// so RunAssist has an agent runtime to spawn onto. The Process store is
// in-process, like the real command's.
func wireAssistRuntime(t *testing.T, assistUC *usecase.AssistUseCase, repo interfaces.Repository,
	registry *model.WorkspaceRegistry, llm gollem.LLMClient, bot slack.Service,
) {
	t.Helper()
	history := agentarchive.NewMemoryHistoryStore()
	reg := agentkit.NewRegistry()
	models := testAgentModelPolicy(t)
	gt.NoError(t, assistUC.Register(reg, testAgentRootBudget.Limiter(models.Resolve), history)).Required()

	k, err := agentkernel.Build(agentkernel.Deps{
		Repo:    agentprocmemory.New(),
		History: history,
		LLM:     llm,
		Trace:   agentarchive.NewMemoryTraceRepository(),
		Budgets: agentkernel.Budgets{Root: testAgentRootBudget, Task: testAgentBudget},
		Models:  models,
		Agents:  reg,
		Tools:   agentkernel.ToolDeps{Repo: repo, Registry: registry, SlackBot: bot},
	})
	gt.NoError(t, err).Required()
	assistUC.Bind(k)
}

// assistLLM answers the agent turn with agentText and the summary turn with the
// supplied JSON. The two are told apart by call order: the summary is produced in
// the completion handler, after the agent run committed.
func assistLLM(agentText, summaryJSON string) gollem.LLMClient {
	var n atomic.Int32
	return &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					if int(n.Add(1)) == 1 {
						return &gollem.Response{Texts: []string{agentText}}, nil
					}
					return &gollem.Response{Texts: []string{summaryJSON}}, nil
				},
			}, nil
		},
	}
}

// A finished assist run must record its AssistLog, and RunAssist must not return
// until it has: the command's exit is what tells its scheduler the pass is over.
func TestAssistUseCase_RunAssistRecordsALogPerCase(t *testing.T) {
	repo := memory.New()
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Assist target", "needs help", []string{}, nil, false, false, "", "")
	gt.NoError(t, err).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:      model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		AssistPrompt:   "Check deadlines.",
		AssistLanguage: "Japanese",
	})

	llm := assistLLM("I reviewed the case and nothing is overdue.",
		`{"summary":"reviewed the case","actions":"","reasoning":"nothing overdue","next_steps":""}`)

	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry, LLM: llm})
	wireAssistRuntime(t, assistUC, repo, registry, llm, nil)

	gt.NoError(t, assistUC.RunAssist(ctx, usecase.AssistOption{})).Required()

	logs, _, err := repo.AssistLog().List(ctx, testWorkspaceID, c.ID, 10, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(1).Required()
	gt.String(t, logs[0].Summary).Equal("reviewed the case")
	gt.String(t, logs[0].Reasoning).Equal("nothing overdue")
	gt.String(t, logs[0].Actions).Equal("")
	gt.String(t, logs[0].NextSteps).Equal("")
	gt.Number(t, logs[0].CaseID).Equal(c.ID)
}

// A run whose agent fails must leave no AssistLog: a log records what a pass
// concluded, and a failed pass concluded nothing.
func TestAssistUseCase_RunAssistWritesNoLogForAFailedRun(t *testing.T) {
	repo := memory.New()
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Assist target", "needs help", []string{}, nil, false, false, "", "")
	gt.NoError(t, err).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		AssistPrompt: "Check deadlines.",
	})

	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return nil, goerr.New("the model is unreachable")
				},
			}, nil
		},
	}

	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry, LLM: llm})
	wireAssistRuntime(t, assistUC, repo, registry, llm, nil)
	// One attempt: the point here is the terminal failure, not agentkit's retry
	// schedule (which pkg/agent covers).
	usecase.SetAssistServeOptionsForTest(assistUC, agentkit.WithMaxStepAttempts(1))

	// errutil.Handle logs through the context logger, so capturing it is how the
	// report is observed. A failure that is merely counted and never reported is
	// the defect this guards: the command would print a clean pass.
	var logged bytes.Buffer
	ctx = logging.With(ctx, logging.New(&logged, slog.LevelInfo, logging.FormatJSON, false))

	// The pass itself succeeds — one failed case does not fail the command — and
	// drain still returns once the run reached its terminal state.
	gt.NoError(t, assistUC.RunAssist(ctx, usecase.AssistOption{})).Required()

	logs, _, err := repo.AssistLog().List(ctx, testWorkspaceID, c.ID, 10, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(0)

	report := logged.String()
	gt.String(t, report).Contains("assist run failed")
	gt.String(t, report).Contains("the model is unreachable")
	gt.String(t, report).Contains(fmt.Sprintf("\"case_id\":%d", c.ID))
}

// RunAssist must refuse before doing anything when the agent runtime was never
// bound: spawning is the only thing it does, so an unbound pass would silently
// process nothing.
func TestAssistUseCase_RunAssistRefusesWhenUnbound(t *testing.T) {
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry})

	gt.Error(t, assistUC.RunAssist(context.Background(), usecase.AssistOption{}))
}

func TestAssistUseCase_RunAssist_SkipsWorkspaceWithoutAssistConfig(t *testing.T) {
	repo := memory.New()
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	// Create workspace entry without assist prompt
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		// AssistPrompt is empty - should be skipped
	})

	llm := assistLLM("unused", "{}")
	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry, LLM: llm})
	wireAssistRuntime(t, assistUC, repo, registry, llm, nil)

	err := assistUC.RunAssist(ctx, usecase.AssistOption{})
	gt.NoError(t, err)
}

func TestAssistUseCase_RunAssist_DefaultOptions(t *testing.T) {
	repo := memory.New()
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	// Empty registry - no workspaces to process
	registry := model.NewWorkspaceRegistry()
	llm := assistLLM("unused", "{}")
	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry, LLM: llm})
	wireAssistRuntime(t, assistUC, repo, registry, llm, nil)

	err := assistUC.RunAssist(ctx, usecase.AssistOption{})
	gt.NoError(t, err)
}

func TestAssistUseCase_RunAssist_WorkspaceFilter(t *testing.T) {
	repo := memory.New()
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-1", Name: "Workspace 1"},
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-2", Name: "Workspace 2"},
	})

	llm := assistLLM("unused", "{}")
	assistUC := usecase.NewAssistUseCase(usecase.AssistDeps{Repo: repo, Registry: registry, LLM: llm})
	wireAssistRuntime(t, assistUC, repo, registry, llm, nil)

	// Filter to non-existent workspace should fail
	err := assistUC.RunAssist(ctx, usecase.AssistOption{WorkspaceID: "ws-nonexistent"})
	gt.Value(t, err).NotNil()

	// Filter to existing workspace (without assist prompt) should succeed and skip
	err = assistUC.RunAssist(ctx, usecase.AssistOption{WorkspaceID: "ws-1"})
	gt.NoError(t, err)
}
