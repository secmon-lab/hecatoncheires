package planexec

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/m-mizutani/goerr/v2"
)

// TaskPlan is one parallel investigation task within a PlanResult or
// ReplanResult:
//   - ID: stable identifier the host uses to correlate progress lines
//   - Title: short label rendered to the user
//   - Description: full instruction handed to the sub-agent
//   - AcceptanceCriteria: the measurable bar against which the next
//     replan judges whether the goal has been met
//   - Tools: the toolset ids this task's sub-agent may use (a subset of
//     Input.KnownToolIDs)
type TaskPlan struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Tools              []string `json:"tools"`
	// BudgetUSD is what this task's sub-agent may spend, in USD, carved by the
	// planner out of what the run has left. It is the planner's job because the
	// planner is the only party that knows which of the tasks it just wrote is the
	// heavy one.
	//
	// It is omitempty and validated only when the host wired Config.Remaining: a
	// host that did not is not asking for per-task budgets, and its children keep
	// inheriting the run's own figure.
	BudgetUSD float64 `json:"budget_usd,omitempty"`
}

// TaskStatus marks the outcome of a single sub-agent task.
type TaskStatus string

const (
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskResult is the per-task summary folded into the next planner round's
// observations.
type TaskResult struct {
	TaskID             string
	Title              string
	AcceptanceCriteria string
	Status             TaskStatus
	Summary            string
	Error              string
}

// PhaseSummary aggregates one round's task results so the planner has
// structured observations on the next round.
type PhaseSummary struct {
	Phase   int
	Tasks   []TaskPlan
	Results []TaskResult
}

// subAgentSummaryMaxBytes bounds the sub-agent text fed back into the
// planner. Long summaries are truncated at a UTF-8 rune boundary to keep
// the planner-input token budget bounded.
const subAgentSummaryMaxBytes = 8 * 1024

//go:embed prompts/subagent.md
var subAgentPromptTmpl string

var subAgentPromptTemplate = template.Must(template.New("planexec_subagent").Parse(subAgentPromptTmpl))

// subAgentPromptInput is the data fed into prompts/subagent.md. It embeds
// the per-task fields and adds AllowWrites so the template can toggle the
// observation-only vs may-write instruction. AllowWrites mirrors the run's
// Input.AllowSubAgentWrites, and Context mirrors Input.TaskContext.
type subAgentPromptInput struct {
	TaskPlan
	AllowWrites bool
	Context     string
}

// buildSubAgentSystemPrompt renders prompts/subagent.md with the per-task
// fields and the run's host-supplied context block. Returns an error only when
// template execution fails — should never happen with valid struct data, but
// the guard prevents a malformed task from silently producing an empty prompt.
func buildSubAgentSystemPrompt(task TaskPlan, allowWrites bool, taskContext string) (string, error) {
	var buf bytes.Buffer
	input := subAgentPromptInput{TaskPlan: task, AllowWrites: allowWrites, Context: taskContext}
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

// formatObservationsAsUserTurn renders the user-input string fed into the
// next planner round: a markdown document with one section per completed
// task.
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
