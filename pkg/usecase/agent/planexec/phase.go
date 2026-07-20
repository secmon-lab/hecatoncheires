package planexec

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// TaskPlan is one parallel investigation task within a PlanResult or
// ReplanResult. The fields follow warren bluebell's TaskPlan shape:
//   - ID: stable identifier the host uses to correlate progress lines
//   - Title: short label rendered to the user
//   - Description: full instruction handed to the sub-agent
//   - AcceptanceCriteria: the measurable bar against which the next
//     replan judges whether the goal has been met
//   - Tools: the allowed ToolResolver IDs for this task (subset of
//     RunRequest.KnownToolIDs)
type TaskPlan struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Tools              []string `json:"tools"`
}

// TaskStatus marks the outcome of a single sub-agent task.
type TaskStatus string

const (
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskResult is the per-task summary surfaced via Sink.TaskFinished and
// folded into the next planner round's observations.
type TaskResult struct {
	TaskID             string
	Title              string
	AcceptanceCriteria string
	Status             TaskStatus
	Summary            string
	Error              string

	// InnerLoopsUsed / InnerLoopsMax surface in the trace UI but are not
	// fed back into the planner (the planner doesn't need them for
	// decisions).
	InnerLoopsUsed int64
	InnerLoopsMax  int
}

// PhaseSummary aggregates the results of one executePhase invocation so
// the planner has structured observations on the next round.
type PhaseSummary struct {
	Phase   int
	Tasks   []TaskPlan
	Results []TaskResult
}

// subAgentSummaryMaxBytes bounds the sub-agent text fed back into the
// planner. Long summaries are truncated at a UTF-8 rune boundary to keep
// the planner-input token budget bounded.
const subAgentSummaryMaxBytes = 8 * 1024

// taskProgressMaxRunes is the per-line excerpt budget surfaced via
// Sink.TaskProgress during sub-agent execution.
const taskProgressMaxRunes = 80

//go:embed prompts/subagent.md
var subAgentPromptTmpl string

var subAgentPromptTemplate = template.Must(template.New("planexec_subagent").Parse(subAgentPromptTmpl))

// subAgentPromptInput is the data fed into prompts/subagent.md. It embeds
// the per-task fields and adds AllowWrites so the template can toggle the
// observation-only vs may-write instruction. AllowWrites mirrors the run's
// RunRequest.AllowSubAgentWrites.
type subAgentPromptInput struct {
	TaskPlan
	AllowWrites bool
}

// executePhase dispatches a sub-agent goroutine per task, blocks until
// all complete, and returns results in the same order as the input. Per-
// task failures surface as TaskStatusFailed TaskResults rather than being
// propagated — the planner decides on the next round whether the partial
// observations are enough to advance.
//
// The host MUST have already received Sink.PhaseStarted before this is
// called (Runner.Run does that); per-task progress comes through
// Sink.TaskProgress / TaskFinished.
func executePhase(
	ctx context.Context,
	tasks []TaskPlan,
	sink Sink,
	resolver ToolResolver,
	llm gollem.LLMClient,
	subAgentLoopMax int,
	hostTrace trace.Handler,
	allowWrites bool,
) []TaskResult {
	if len(tasks) == 0 {
		return nil
	}
	results := make([]TaskResult, len(tasks))
	var wg sync.WaitGroup
	for i := range tasks {
		wg.Add(1)
		async.Dispatch(ctx, func(c context.Context) error {
			defer wg.Done()
			// Recover a sub-agent panic HERE, inside the closure, so results[i]
			// is set before the goroutine unwinds. Without this the assignment
			// below never happens and results[i] stays a zero-value TaskResult
			// (empty Status), which formatObservationsAsUserTurn renders as a
			// contentless note — the planner never learns the task failed.
			// Dual-transmit: a failed observation to the planner AND errutil to
			// the operator/Sentry, because a panic is a genuine defect, not the
			// expected "tool returned an error" path.
			defer func() {
				if p := recover(); p != nil {
					perr := goerr.New("sub-agent panicked",
						goerr.V("task_id", tasks[i].ID),
						goerr.V("panic", p))
					results[i] = TaskResult{
						TaskID:             tasks[i].ID,
						Title:              tasks[i].Title,
						AcceptanceCriteria: tasks[i].AcceptanceCriteria,
						Status:             TaskStatusFailed,
						Error:              perr.Error(),
						InnerLoopsMax:      subAgentLoopMax,
					}
					errutil.Handle(c, perr, "planexec: sub-agent panic")
				}
			}()
			results[i] = runOneTask(c, tasks[i], sink, resolver, llm, subAgentLoopMax, hostTrace, allowWrites)
			return nil
		})
	}
	wg.Wait()
	return results
}

// runOneTask drives a single sub-agent. The sub-agent only ever updates
// its own task block via Sink.TaskProgress; it never posts new blocks or
// touches another task's. Per-iteration progress (tool calls, LLM
// thoughts emitted alongside tool calls) is surfaced through gollem's
// ContentBlockMiddleware so the user sees concrete activity instead of a
// static "running…" placeholder.
func runOneTask(
	ctx context.Context,
	task TaskPlan,
	sink Sink,
	resolver ToolResolver,
	llm gollem.LLMClient,
	subAgentLoopMax int,
	hostTrace trace.Handler,
	allowWrites bool,
) TaskResult {
	started := time.Now()
	sink.TaskProgress(ctx, task.ID, fmt.Sprintf("running: %s", task.Title))

	tools := resolver.Resolve(task.Tools)
	sysPrompt, err := buildSubAgentSystemPrompt(task, allowWrites)
	if err != nil {
		elapsed := time.Since(started).Round(time.Millisecond)
		sink.TaskProgress(ctx, task.ID, fmt.Sprintf("failed to render prompt (%s): %v", elapsed, err))
		res := TaskResult{
			TaskID:             task.ID,
			Title:              task.Title,
			AcceptanceCriteria: task.AcceptanceCriteria,
			Status:             TaskStatusFailed,
			Error:              err.Error(),
			InnerLoopsMax:      subAgentLoopMax,
		}
		sink.TaskFinished(ctx, res)
		return res
	}

	counter := agent.NewLLMCallCounter()
	progressMW := newTaskProgressMiddleware(sink, task.ID, task.Title)
	sub := gollem.New(llm,
		gollem.WithSystemPrompt(sysPrompt),
		gollem.WithTools(tools...),
		gollem.WithLoopLimit(subAgentLoopMax),
		// counter feeds the per-task loop count; hostTrace (when non-nil)
		// feeds the host's per-event timeline. combineTrace returns counter
		// alone when hostTrace is nil, preserving the proposal host path.
		gollem.WithTrace(combineTrace(counter, hostTrace)),
		gollem.WithContentBlockMiddleware(progressMW),
		gollem.WithPromptCache(true),
	)
	resp, execErr := sub.Execute(ctx, gollem.Text(task.Description))
	elapsed := time.Since(started).Round(time.Millisecond)
	used := counter.LLMCalls()

	if execErr != nil {
		sink.TaskProgress(ctx, task.ID, fmt.Sprintf("failed (%s, %d/%d loops): %v",
			elapsed, used, subAgentLoopMax, execErr))
		res := TaskResult{
			TaskID:             task.ID,
			Title:              task.Title,
			AcceptanceCriteria: task.AcceptanceCriteria,
			Status:             TaskStatusFailed,
			Error:              execErr.Error(),
			InnerLoopsUsed:     used,
			InnerLoopsMax:      subAgentLoopMax,
		}
		sink.TaskFinished(ctx, res)
		return res
	}

	summary := ""
	if resp != nil {
		summary = strings.Join(resp.Texts, "\n")
	}
	summary = truncateSummary(summary)
	sink.TaskProgress(ctx, task.ID, fmt.Sprintf("done (%s, %d/%d loops)",
		elapsed, used, subAgentLoopMax))
	res := TaskResult{
		TaskID:             task.ID,
		Title:              task.Title,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Status:             TaskStatusCompleted,
		Summary:            summary,
		InnerLoopsUsed:     used,
		InnerLoopsMax:      subAgentLoopMax,
	}
	sink.TaskFinished(ctx, res)
	return res
}

// buildSubAgentSystemPrompt renders prompts/subagent.md with the per-task
// fields. Returns an error only when template execution fails — should
// never happen with valid struct data, but the guard prevents a
// malformed task from silently producing an empty prompt.
func buildSubAgentSystemPrompt(task TaskPlan, allowWrites bool) (string, error) {
	var buf bytes.Buffer
	input := subAgentPromptInput{TaskPlan: task, AllowWrites: allowWrites}
	if err := subAgentPromptTemplate.Execute(&buf, input); err != nil {
		return "", goerr.Wrap(err, "render sub-agent system prompt",
			goerr.V("task_id", task.ID))
	}
	return buf.String(), nil
}

// truncateSummary walks back to the nearest UTF-8 rune boundary so a
// multi-byte character (e.g. CJK) is not sliced mid-codepoint.
func truncateSummary(s string) string {
	if len(s) <= subAgentSummaryMaxBytes {
		return s
	}
	cut := subAgentSummaryMaxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n…[truncated]"
}

// newTaskProgressMiddleware returns a gollem ContentBlockMiddleware that
// updates the per-task progress line on every LLM round. Each round the
// middleware observes the LLM's response and pushes the most recent
// piece of activity (LLM thought excerpt, or "calling <tool>" when the
// response carries a tool call) to Sink.TaskProgress. Tool-call lines
// win over message lines when both are present in the same response.
//
// The middleware never alters req or resp; it is purely an observer.
func newTaskProgressMiddleware(sink Sink, taskID, taskTitle string) gollem.ContentBlockMiddleware {
	return func(next gollem.ContentBlockHandler) gollem.ContentBlockHandler {
		return func(ctx context.Context, req *gollem.ContentRequest) (*gollem.ContentResponse, error) {
			resp, err := next(ctx, req)
			if err != nil || resp == nil {
				return resp, err
			}
			for _, txt := range resp.Texts {
				excerpt := oneLineExcerpt(txt, taskProgressMaxRunes)
				if excerpt != "" {
					sink.TaskProgress(ctx, taskID, fmt.Sprintf("%s — %s", taskTitle, excerpt))
					break
				}
			}
			if len(resp.FunctionCalls) > 0 && resp.FunctionCalls[0] != nil {
				sink.TaskProgress(ctx, taskID, fmt.Sprintf("%s — calling %s",
					taskTitle, resp.FunctionCalls[0].Name))
			}
			return resp, nil
		}
	}
}

// oneLineExcerpt collapses whitespace in s and truncates to maxRunes
// characters, appending an ellipsis when truncation actually happened.
// Empty input (after trimming) returns an empty string so callers can
// short-circuit — blank progress lines do not benefit the user.
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

// formatObservationsAsUserTurn renders the user-input string fed into the
// next planner round. The shape — a markdown document with one section
// per completed task — is taken verbatim from the proposal-side
// implementation (which itself follows the warren bluebell convention).
func formatObservationsAsUserTurn(tasks []TaskPlan, results []TaskResult) string {
	var b strings.Builder
	b.WriteString("# Observations from prior investigations\n\n")

	byID := make(map[string]TaskPlan, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}

	for _, res := range results {
		title := res.Title
		ac := res.AcceptanceCriteria
		if title == "" {
			if t, ok := byID[res.TaskID]; ok {
				title = t.Title
				if ac == "" {
					ac = t.AcceptanceCriteria
				}
			}
		}
		fmt.Fprintf(&b, "## %s: %s\n", res.TaskID, title)
		fmt.Fprintf(&b, "**Status**: %s\n", res.Status)
		if ac != "" {
			fmt.Fprintf(&b, "**Acceptance criteria**: %s\n", ac)
		}
		switch res.Status {
		case TaskStatusCompleted:
			fmt.Fprintf(&b, "**Result**:\n<task-output>\n%s\n</task-output>\n\n", res.Summary)
		case TaskStatusFailed:
			fmt.Fprintf(&b, "**Error**: %s\n\n", res.Error)
		default:
			fmt.Fprintf(&b, "**Note**: status=%s\n\n", res.Status)
		}
	}

	b.WriteString("Use these observations to decide the next action. Each task's `acceptance_criteria` is the bar against which you should evaluate whether the goal has been met or whether further investigation is needed.\n")
	return b.String()
}
