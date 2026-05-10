// Package draft contains the open-mode (case-draft) agent runtime: a
// plan/execute loop that coordinates a planner LLM and parallel sub-agent
// investigations to produce a CaseDraft for the host (Slack) to render.
// The plan schema and parsing live in this file.
package draft

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// planAction enumerates the three terminal / continuation choices the planner
// LLM can return in a single round. `post_message` was retired in favour of
// always asking via `question` when user input is required.
type planAction string

const (
	actionInvestigate planAction = "investigate"
	actionQuestion    planAction = "question"
	actionMaterialize planAction = "materialize"
)

// plan is the structured response shape the planner LLM emits each round.
// Exactly one of the per-action payload fields must be non-nil for a given
// Action value; mismatches are rejected at validation.
type plan struct {
	// Reasoning is a 1-2 sentence rationale for the chosen Action. Surfaced
	// in the Slack trace UI so users can follow the agent's path.
	Reasoning string     `json:"reasoning"`
	Action    planAction `json:"action"`

	Investigate *planInvestigate `json:"investigate,omitempty"`
	Question    *planQuestion    `json:"question,omitempty"`
	Materialize *planMaterialize `json:"materialize,omitempty"`
}

// planInvestigate is the payload for action=investigate. Tasks are run in
// parallel; their summaries are folded into the next planner round's user
// input as observations.
type planInvestigate struct {
	// Message is an optional phase-opening declaration shown above the
	// per-task progress lines (e.g. "Looking into A, B, C...").
	Message string                `json:"message,omitempty"`
	Tasks   []planInvestigateTask `json:"tasks"`
}

// planInvestigateTask follows the bluebell TaskPlan shape: ID for trace
// correlation, Title for short trace labels, Description for the sub-agent
// instruction, AcceptanceCriteria as the measurable bar, Tools as the
// allowed ToolSet ID list.
type planInvestigateTask struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Tools              []string `json:"tools"`
}

// planQuestion is the payload for action=question. The planner can pose
// multiple questions in one turn — useful when several pieces of information
// are missing so the user can answer them all in one round trip rather than
// being asked one at a time.
type planQuestion struct {
	// Reason explains the information gap (single rationale shared across
	// the items). Surfaced in the host's question UI.
	Reason string `json:"reason"`
	// Items is the ordered list of questions to ask in this turn. Must be
	// non-empty.
	Items []planQuestionItem `json:"items"`
}

// planQuestionType discriminates how the host should render the answer
// control. `select` and `multi_select` require a non-empty Options list
// (≥2 entries); `free_text` is the last-resort prose-input shape and
// the `Options` field is ignored. The planner is told to prefer the
// closed-list types and to use `free_text` only after exhausting other
// avenues — see prompts/planner.md for the policy.
type planQuestionType string

const (
	questionTypeSelect      planQuestionType = "select"
	questionTypeMultiSelect planQuestionType = "multi_select"
	questionTypeFreeText    planQuestionType = "free_text"
)

// planQuestionItem is a single question within planQuestion.Items.
type planQuestionItem struct {
	// ID is unique within the items list. The host uses it to correlate
	// answers back to the question on submission.
	ID string `json:"id"`
	// Text is the prompt presented to the user.
	Text string `json:"text"`
	// Type is the answer control type (`select`, `multi_select`, or
	// `free_text`).
	Type planQuestionType `json:"type"`
	// Options lists the allowed answer values for `select` /
	// `multi_select` (≥2 entries). Ignored for `free_text`.
	Options []string `json:"options,omitempty"`
}

// planMaterialize is the payload for materialize. Field values are passed as
// untyped JSON to keep the planner's surface narrow; the host validates
// against the workspace's FieldSchema before persisting.
type planMaterialize struct {
	WorkspaceID       string         `json:"workspace_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	CustomFieldValues map[string]any `json:"custom_field_values"`
}

// Limits applied at validation time. Adjusting these requires updating the
// planner system prompt as well so the LLM does not propose plans that
// always reject.
const (
	investigateMinTasks  = 1
	investigateMaxTasks  = 5
	questionMinItems     = 1
	questionMaxItems     = 5
	questionMinOptions   = 2
	investigateMaxToolID = 4 // upper bound on Tools list length per task
)

// parseAndValidate decodes the planner LLM's JSON response and runs the full
// validation pass (action ↔ payload coherence, sub-task counts, tool ID
// allowlist, etc.). Returns a typed error wrapping the underlying cause for
// the retry path; the planner driver feeds err.Error() back as user input
// when retrying.
func parseAndValidate(raw []byte) (*plan, error) {
	var p plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, goerr.Wrap(err, "decode plan json")
	}
	if err := validate(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// validate enforces the plan invariants. Surfaced to callers (and looped
// back to the LLM as retry input) on failure.
func validate(p *plan) error {
	if p == nil {
		return goerr.New("plan is nil")
	}
	if strings.TrimSpace(p.Reasoning) == "" {
		return goerr.New("reasoning is required")
	}

	// Exactly one payload non-nil; matches Action.
	switch p.Action {
	case actionInvestigate:
		if p.Investigate == nil {
			return goerr.New("investigate payload is required for action=investigate")
		}
		if p.Question != nil || p.Materialize != nil {
			return goerr.New("only the investigate payload may be set for action=investigate")
		}
		return validateInvestigate(p.Investigate)
	case actionQuestion:
		if p.Question == nil {
			return goerr.New("question payload is required for action=question")
		}
		if p.Investigate != nil || p.Materialize != nil {
			return goerr.New("only the question payload may be set for action=question")
		}
		return validateQuestion(p.Question)
	case actionMaterialize:
		if p.Materialize == nil {
			return goerr.New("materialize payload is required for action=materialize")
		}
		if p.Investigate != nil || p.Question != nil {
			return goerr.New("only the materialize payload may be set for action=materialize")
		}
		return validateMaterialize(p.Materialize)
	default:
		return goerr.New("unknown action", goerr.V("action", string(p.Action)))
	}
}

func validateInvestigate(inv *planInvestigate) error {
	if n := len(inv.Tasks); n < investigateMinTasks || n > investigateMaxTasks {
		return goerr.New("investigate.tasks count out of range",
			goerr.V("got", n),
			goerr.V("min", investigateMinTasks),
			goerr.V("max", investigateMaxTasks),
		)
	}
	seenID := make(map[string]struct{}, len(inv.Tasks))
	for i, task := range inv.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return goerr.New("investigate.tasks[i].id is required", goerr.V("i", i))
		}
		if _, dup := seenID[task.ID]; dup {
			return goerr.New("investigate.tasks[i].id is duplicated within plan", goerr.V("id", task.ID))
		}
		seenID[task.ID] = struct{}{}
		if strings.TrimSpace(task.Title) == "" {
			return goerr.New("investigate.tasks[i].title is required", goerr.V("id", task.ID))
		}
		if strings.TrimSpace(task.Description) == "" {
			return goerr.New("investigate.tasks[i].description is required", goerr.V("id", task.ID))
		}
		if strings.TrimSpace(task.AcceptanceCriteria) == "" {
			return goerr.New("investigate.tasks[i].acceptance_criteria is required", goerr.V("id", task.ID))
		}
		if len(task.Tools) == 0 {
			return goerr.New("investigate.tasks[i].tools must not be empty", goerr.V("id", task.ID))
		}
		if len(task.Tools) > investigateMaxToolID {
			return goerr.New("investigate.tasks[i].tools too many entries",
				goerr.V("id", task.ID),
				goerr.V("got", len(task.Tools)),
				goerr.V("max", investigateMaxToolID),
			)
		}
		for _, id := range task.Tools {
			if !agent.IsKnownToolSetID(id) {
				return goerr.New("investigate.tasks[i].tools contains unknown id",
					goerr.V("id", task.ID),
					goerr.V("tool_id", id),
					goerr.V("known", agent.KnownToolSetIDs),
				)
			}
		}
	}
	return nil
}

func validateQuestion(q *planQuestion) error {
	if strings.TrimSpace(q.Reason) == "" {
		return goerr.New("question.reason is required")
	}
	if n := len(q.Items); n < questionMinItems || n > questionMaxItems {
		return goerr.New("question.items count out of range",
			goerr.V("got", n),
			goerr.V("min", questionMinItems),
			goerr.V("max", questionMaxItems),
		)
	}
	seenID := make(map[string]struct{}, len(q.Items))
	for i, it := range q.Items {
		if strings.TrimSpace(it.ID) == "" {
			return goerr.New("question.items[i].id is required", goerr.V("i", i))
		}
		if _, dup := seenID[it.ID]; dup {
			return goerr.New("question.items[i].id is duplicated within plan", goerr.V("id", it.ID))
		}
		seenID[it.ID] = struct{}{}
		if strings.TrimSpace(it.Text) == "" {
			return goerr.New("question.items[i].text is required", goerr.V("id", it.ID))
		}
		switch it.Type {
		case questionTypeSelect, questionTypeMultiSelect:
			if len(it.Options) < questionMinOptions {
				return goerr.New("question.items[i].options must contain at least 2 entries",
					goerr.V("id", it.ID), goerr.V("got", len(it.Options)))
			}
			seenOpt := make(map[string]struct{}, len(it.Options))
			for j, opt := range it.Options {
				if strings.TrimSpace(opt) == "" {
					return goerr.New("question.items[i].options[j] must not be empty",
						goerr.V("id", it.ID), goerr.V("j", j))
				}
				if _, dup := seenOpt[opt]; dup {
					return goerr.New("question.items[i].options contains duplicate",
						goerr.V("id", it.ID), goerr.V("opt", opt))
				}
				seenOpt[opt] = struct{}{}
			}
		case questionTypeFreeText:
			// Options are ignored for free_text — the host renders a
			// plain text input, not a chooser. We do not require the
			// field to be empty (the planner is allowed to leave it
			// out or include it as a hint that we discard).
		default:
			return goerr.New("question.items[i].type must be select, multi_select, or free_text",
				goerr.V("id", it.ID), goerr.V("got", string(it.Type)))
		}
	}
	return nil
}

func validateMaterialize(m *planMaterialize) error {
	if strings.TrimSpace(m.WorkspaceID) == "" {
		return goerr.New("materialize.workspace_id is required")
	}
	if strings.TrimSpace(m.Title) == "" {
		return goerr.New("materialize.title is required")
	}
	// Description and CustomFieldValues are workspace-schema-dependent, so the
	// host (which holds the FieldSchema) is responsible for the field-level
	// shape. We only enforce the presence of the action-routing fields here.
	return nil
}

// planSchema returns the *gollem.Parameter that constrains the planner LLM's
// JSON output. The schema is intentionally permissive at the JSON level
// (most non-trivial constraints are enforced in Go via validate) so the
// planner has fewer hard rejection paths from the model side.
func planSchema() *gollem.Parameter {
	str := func(desc string) *gollem.Parameter {
		return &gollem.Parameter{Type: gollem.TypeString, Description: desc}
	}
	investigateTaskProps := map[string]*gollem.Parameter{
		"id":                  {Type: gollem.TypeString, Description: "Phase-unique identifier (e.g. inv-1)."},
		"title":               {Type: gollem.TypeString, Description: "Short label for the trace UI (~40 chars)."},
		"description":         {Type: gollem.TypeString, Description: "Detailed instruction for the sub-agent."},
		"acceptance_criteria": {Type: gollem.TypeString, Description: "Measurable completion condition."},
		"tools": {
			Type:        gollem.TypeArray,
			Description: "Allowed ToolSet IDs for this task.",
			Items: &gollem.Parameter{
				Type: gollem.TypeString,
				Enum: agent.KnownToolSetIDs,
			},
		},
	}
	investigateProps := map[string]*gollem.Parameter{
		"message": {Type: gollem.TypeString, Description: "Optional phase-opening declaration."},
		"tasks": {
			Type:        gollem.TypeArray,
			Description: "Parallel investigation tasks for this phase.",
			Items: &gollem.Parameter{
				Type:       gollem.TypeObject,
				Properties: investigateTaskProps,
			},
		},
	}
	questionItemProps := map[string]*gollem.Parameter{
		"id":   str("Item-unique identifier (e.g. q-1)."),
		"text": str("Question text shown to the user."),
		"type": {
			Type:        gollem.TypeString,
			Description: "Answer control type. Use free_text only as a last resort.",
			Enum: []string{
				string(questionTypeSelect),
				string(questionTypeMultiSelect),
				string(questionTypeFreeText),
			},
		},
		"options": {
			Type:        gollem.TypeArray,
			Description: "Allowed answer values for select / multi_select (≥2 entries). Ignored for free_text.",
			Items:       &gollem.Parameter{Type: gollem.TypeString},
		},
	}
	questionProps := map[string]*gollem.Parameter{
		"reason": str("Why these questions are necessary."),
		"items": {
			Type:        gollem.TypeArray,
			Description: "Ordered list of questions for this turn (1-5).",
			Items: &gollem.Parameter{
				Type:       gollem.TypeObject,
				Properties: questionItemProps,
			},
		},
	}
	materializeProps := map[string]*gollem.Parameter{
		"workspace_id": str("Target workspace ID for the draft."),
		"title":        str("Case title."),
		"description":  str("Case description."),
		"custom_field_values": {
			Type:        gollem.TypeObject,
			Description: "Workspace-schema-dependent field values keyed by field ID.",
			// gollem requires Properties for object types; supply an empty
			// map so the open-shape object is accepted.
			Properties: map[string]*gollem.Parameter{},
		},
	}

	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Single planner round output.",
		Properties: map[string]*gollem.Parameter{
			"reasoning": str("1-2 sentence rationale for the chosen action."),
			"action": {
				Type: gollem.TypeString,
				Enum: []string{
					string(actionInvestigate),
					string(actionQuestion),
					string(actionMaterialize),
				},
			},
			"investigate": {Type: gollem.TypeObject, Properties: investigateProps},
			"question":    {Type: gollem.TypeObject, Properties: questionProps},
			"materialize": {Type: gollem.TypeObject, Properties: materializeProps},
		},
	}
}

// investigationStatus marks the outcome of a single sub-agent task.
type investigationStatus string

const (
	investigationCompleted investigationStatus = "completed"
	investigationFailed    investigationStatus = "failed"
)

// investigationResult is the per-task summary the planner sees on the next
// round as part of formatObservationsAsUserTurn output.
type investigationResult struct {
	TaskID             string
	Title              string
	AcceptanceCriteria string
	Status             investigationStatus
	Summary            string
	Error              string

	// InnerLoopsUsed / InnerLoopsMax are surfaced in the trace UI but not
	// folded into the planner's user input (the planner doesn't need them
	// for decisions).
	InnerLoopsUsed int64
	InnerLoopsMax  int
}

// formatObservationsAsUserTurn renders the next planner-round user input
// from the just-finished investigation phase. The format is a markdown
// document the LLM can scan to update its mental state for the next plan
// step.
func formatObservationsAsUserTurn(inv *planInvestigate, results []investigationResult) string {
	var b strings.Builder
	b.WriteString("# Observations from prior investigations\n\n")

	taskByID := make(map[string]planInvestigateTask, len(inv.Tasks))
	for _, t := range inv.Tasks {
		taskByID[t.ID] = t
	}

	for _, res := range results {
		title := res.Title
		ac := res.AcceptanceCriteria
		// Fall back to the original task spec if the result row was somehow
		// stripped of metadata.
		if title == "" {
			if t, ok := taskByID[res.TaskID]; ok {
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
		case investigationCompleted:
			fmt.Fprintf(&b, "**Result**:\n<task-output>\n%s\n</task-output>\n\n", res.Summary)
		case investigationFailed:
			fmt.Fprintf(&b, "**Error**: %s\n\n", res.Error)
		default:
			fmt.Fprintf(&b, "**Note**: status=%s\n\n", res.Status)
		}
	}

	b.WriteString("Use these observations to decide the next action. Each task's `acceptance_criteria` is the bar against which you should evaluate whether the goal has been met or whether further investigation is needed.\n")
	return b.String()
}
