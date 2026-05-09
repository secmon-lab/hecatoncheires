package draft

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

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

//go:embed prompts/subagent.md
var subAgentPromptTmpl string

var subAgentPromptTemplate = template.Must(template.New("draft_subagent").Parse(subAgentPromptTmpl))

// runInvestigationsParallel dispatches a sub-agent goroutine per task,
// blocks until all complete, and returns results in the same order as the
// input plan. Errors from individual sub-agents are surfaced as failed
// investigationResults rather than propagated up — the planner is
// responsible for deciding whether a partial set of observations is
// enough to advance.
func (uc *UseCase) runInvestigationsParallel(ctx context.Context, p *planInvestigate, h Handler, resolver *agent.ToolSetResolver) []investigationResult {
	if p == nil {
		return nil
	}
	if p.Message != "" {
		h.Trace(ctx, "🧭 "+p.Message)
	}
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

// runOneInvestigation drives a single sub-agent: builds its system prompt
// from the task spec, resolves the requested toolsets, attaches an LLM call
// counter (gollem trace.Handler) so the inner loop count is observable, and
// runs gollem.Execute. The returned summary is truncated to bound planner
// input.
func (uc *UseCase) runOneInvestigation(ctx context.Context, task planInvestigateTask, h Handler, resolver *agent.ToolSetResolver) investigationResult {
	started := time.Now()
	h.Trace(ctx, fmt.Sprintf("🔍 [%s] %s — starting", task.ID, task.Title))

	tools := resolver.Resolve(task.Tools)
	sysPrompt, err := buildSubAgentSystemPrompt(task)
	if err != nil {
		elapsed := time.Since(started).Round(time.Millisecond)
		h.Trace(ctx, fmt.Sprintf("❌ [%s] %s — failed (%s, build prompt): %v", task.ID, task.Title, elapsed, err))
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
	sub := gollem.New(uc.deps.LLMClient,
		gollem.WithSystemPrompt(sysPrompt),
		gollem.WithTools(tools...),
		gollem.WithLoopLimit(uc.subAgentLoopMax),
		gollem.WithTrace(counter),
	)
	resp, execErr := sub.Execute(ctx, gollem.Text(task.Description))
	elapsed := time.Since(started).Round(time.Millisecond)
	used := counter.LLMCalls()

	if execErr != nil {
		h.Trace(ctx, fmt.Sprintf("❌ [%s] %s — failed (%s, %d/%d inner loops): %v",
			task.ID, task.Title, elapsed, used, uc.subAgentLoopMax, execErr))
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
		h.Trace(ctx, fmt.Sprintf("⚠️ [%s] summary truncated to %d bytes", task.ID, subAgentSummaryMaxBytes))
	}
	h.Trace(ctx, fmt.Sprintf("✅ [%s] %s — done (%s, %d/%d inner loops)",
		task.ID, task.Title, elapsed, used, uc.subAgentLoopMax))
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
