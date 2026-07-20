package proposal

import (
	"bytes"
	"context"
	_ "embed"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

//go:embed prompts/subagent.md
var subAgentPromptTmpl string

var subAgentPromptTemplate = template.Must(template.New("draft_subagent").Parse(subAgentPromptTmpl))

// taskProgressMaxRunes bounds the LLM-message excerpt that is rendered into
// a per-task trace block during execution. The block is one Slack context
// line, so very long thoughts are summarised down to this character budget.
const taskProgressMaxRunes = 80

// runInvestigationsParallel dispatches a sub-agent goroutine per task,
// blocks until all complete, and returns results in the same order as the
// input plan. Errors from individual sub-agents are surfaced as failed
// investigationResults rather than propagated up — the planner is
// responsible for deciding whether a partial set of observations is
// enough to advance.
//
// Block creation is the parent's job: RegisterTasks reserves one trace
// block per task on the host BEFORE any sub-agent goroutine starts, so
// sub-agents can update only their own line via TraceTask without ever
// posting a fresh Slack message.
func (uc *UseCase) runInvestigationsParallel(ctx context.Context, p *planInvestigate, h Handler, resolver *agent.ToolSetResolver) []investigationResult {
	if p == nil {
		return nil
	}
	if p.Message != "" {
		h.Trace(ctx, i18n.T(ctx, i18n.MsgProposalTracePhase, p.Message))
	}

	if len(p.Tasks) == 0 {
		return nil
	}
	taskInfos := make([]TaskInfo, len(p.Tasks))
	for i, task := range p.Tasks {
		taskInfos[i] = TaskInfo{ID: task.ID, Title: task.Title}
	}
	h.RegisterTasks(ctx, taskInfos)

	results := make([]investigationResult, len(p.Tasks))
	var wg sync.WaitGroup
	for i := range p.Tasks {
		task := p.Tasks[i]
		wg.Add(1)
		async.Dispatch(ctx, func(c context.Context) error {
			defer wg.Done()
			results[i] = uc.runOneInvestigation(c, task, h, resolver)
			return nil
		})
	}
	wg.Wait()
	return results
}

// runOneInvestigation drives a single sub-agent. The sub-agent only ever
// updates its own task block via Handler.TraceTask: it never posts new
// Slack messages, never touches another task's block. Per-iteration
// progress (tool calls, LLM thoughts emitted alongside tool calls) is
// surfaced through gollem's MessageHook / ToolRequestHook, so the user
// sees concrete activity instead of a static "running…" placeholder.
func (uc *UseCase) runOneInvestigation(ctx context.Context, task planInvestigateTask, h Handler, resolver *agent.ToolSetResolver) investigationResult {
	started := time.Now()
	h.TraceTask(ctx, task.ID, i18n.T(ctx, i18n.MsgProposalTraceTaskRunning, task.Title))

	tools := resolver.Resolve(task.Tools)
	sysPrompt, err := buildSubAgentSystemPrompt(task)
	if err != nil {
		elapsed := time.Since(started).Round(time.Millisecond)
		h.TraceTask(ctx, task.ID, i18n.T(ctx, i18n.MsgProposalTraceTaskFailedPrompt, task.Title, elapsed, err))
		return investigationResult{
			TaskID: task.ID, Title: task.Title,
			AcceptanceCriteria: task.AcceptanceCriteria,
			Status:             investigationFailed,
			Error:              err.Error(),
			InnerLoopsUsed:     0,
			InnerLoopsMax:      uc.subAgentLoopMax,
		}
	}

	counter := agent.NewLLMCallCounter()
	progressMW := newProgressMiddleware(h, task.ID, task.Title)
	sub := gollem.New(uc.deps.LLMClient,
		gollem.WithSystemPrompt(sysPrompt),
		gollem.WithTools(tools...),
		gollem.WithLoopLimit(uc.subAgentLoopMax),
		gollem.WithTrace(counter),
		gollem.WithContentBlockMiddleware(progressMW),
		gollem.WithPromptCache(true),
	)
	resp, execErr := sub.Execute(ctx, gollem.Text(task.Description))
	elapsed := time.Since(started).Round(time.Millisecond)
	used := counter.LLMCalls()

	if execErr != nil {
		h.TraceTask(ctx, task.ID, i18n.T(ctx, i18n.MsgProposalTraceTaskFailed,
			task.Title, elapsed, used, uc.subAgentLoopMax, execErr))
		return investigationResult{
			TaskID: task.ID, Title: task.Title,
			AcceptanceCriteria: task.AcceptanceCriteria,
			Status:             investigationFailed,
			Error:              execErr.Error(),
			InnerLoopsUsed:     used,
			InnerLoopsMax:      uc.subAgentLoopMax,
		}
	}

	summary := strings.Join(resp.Texts, "\n")
	if len(summary) > subAgentSummaryMaxBytes {
		// Walk back to the nearest UTF-8 rune boundary so a multi-byte
		// character (e.g. CJK) isn't sliced mid-codepoint.
		cut := subAgentSummaryMaxBytes
		for cut > 0 && !utf8.RuneStart(summary[cut]) {
			cut--
		}
		summary = summary[:cut] + "\n…[truncated]"
	}
	h.TraceTask(ctx, task.ID, i18n.T(ctx, i18n.MsgProposalTraceTaskDone,
		task.Title, elapsed, used, uc.subAgentLoopMax))
	return investigationResult{
		TaskID: task.ID, Title: task.Title,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Status:             investigationCompleted,
		Summary:            summary,
		InnerLoopsUsed:     used,
		InnerLoopsMax:      uc.subAgentLoopMax,
	}
}

// buildSubAgentSystemPrompt renders prompts/subagent.md with the per-task
// fields. Returns an error only when the template execution fails (which
// should never happen with valid struct data, but is guarded so a malformed
// task does not silently produce an empty prompt).
func buildSubAgentSystemPrompt(task planInvestigateTask) (string, error) {
	var buf bytes.Buffer
	if err := subAgentPromptTemplate.Execute(&buf, task); err != nil {
		return "", goerr.Wrap(err, "render sub-agent system prompt")
	}
	return buf.String(), nil
}

// newProgressMiddleware returns a gollem ContentBlockMiddleware that
// updates the per-task trace block on every LLM round. Each round, the
// middleware observes the LLM's response after the inner-loop call has
// returned and pushes the most recent piece of activity (LLM thought
// excerpt, or "calling <tool>" when the response carries a tool call) to
// the host's TraceTask handler. Tool-call lines win over message lines
// when both are present in the same response — they describe a more
// concrete next step the sub-agent is about to take.
//
// The middleware never alters req or resp; it is purely an observer.
func newProgressMiddleware(h Handler, taskID, taskTitle string) gollem.ContentBlockMiddleware {
	return func(next gollem.ContentBlockHandler) gollem.ContentBlockHandler {
		return func(ctx context.Context, req *gollem.ContentRequest) (*gollem.ContentResponse, error) {
			resp, err := next(ctx, req)
			if err != nil || resp == nil {
				return resp, err
			}
			// Surface the LLM's accompanying thought first so the user
			// gets some signal even when the response is text-only.
			for _, txt := range resp.Texts {
				excerpt := oneLineExcerpt(txt, taskProgressMaxRunes)
				if excerpt != "" {
					h.TraceTask(ctx, taskID, i18n.T(ctx, i18n.MsgProposalTraceTaskRunningMessage, taskTitle, excerpt))
					break
				}
			}
			// If the LLM is also asking to call a tool, that is the most
			// informative thing to show — overwrite the message line.
			if len(resp.FunctionCalls) > 0 && resp.FunctionCalls[0] != nil {
				h.TraceTask(ctx, taskID, i18n.T(ctx, i18n.MsgProposalTraceTaskRunningTool, taskTitle, resp.FunctionCalls[0].Name))
			}
			return resp, nil
		}
	}
}

// oneLineExcerpt collapses whitespace in s and truncates to maxRunes
// characters, appending an ellipsis when truncation actually happened.
// Returns an empty string for input that is empty after trimming so the
// caller can short-circuit (the trace UI does not benefit from blank
// updates). strings.Fields splits on any unicode whitespace (newlines,
// tabs, runs of spaces) and drops empty tokens, so a single Join with
// " " gives us the collapsed-on-one-line form in one pass.
func oneLineExcerpt(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
