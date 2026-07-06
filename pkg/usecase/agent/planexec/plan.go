// Package planexec hosts the reusable plan-and-execute loop shared by
// the proposal (case-draft) host and the planexec-strategy Job host.
//
// The loop is `plan → executePhase → replan → ... → final` and follows the
// vocabulary established by secmon-lab/warren's `pkg/usecase/chat/bluebell`:
// the planner emits a `Plan` (a list of `TaskPlan` to run in parallel), the
// sub-agents fan out via `executePhase`, and the planner is asked again
// (`replan`) with the observations. The loop exits when replan returns no
// further tasks and no question, after which the runtime invokes
// `generateFinalResponse` for the host-visible terminal output. No
// "terminal action" discriminator is involved.
package planexec

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
)

// Validation limits applied during parsePlanResult / parseReplanResult.
// Adjusting these requires updating the planner system prompt as well so
// the LLM does not propose plans that always reject.
const (
	minTasksPerPhase   = 1
	maxTasksPerPhase   = 5
	minQuestionItems   = 1
	maxQuestionItems   = 5
	minQuestionOptions = 2
	maxToolsPerTask    = 4
)

// PlanResult is the parsed shape of the first planner-round JSON output.
// The planner must choose exactly one of two shapes:
//   - Tasks: the parallel investigation phase (the default path). At least
//     one task is required; if the host wants the planner to terminate after
//     a phase, that is a replan-round concern.
//   - Direct: skip investigation entirely and answer the user directly
//     (round-1 fast path). Only valid when the host set
//     RunRequest.AllowDirect.
type PlanResult struct {
	// Message is a 1-2 sentence rationale shown to the user via
	// Sink.PlanProposed.
	Message string `json:"message,omitempty"`
	// Tasks is the parallel investigation phase emitted by the planner.
	// Empty / omitted when Direct is set — the two are mutually exclusive.
	Tasks []TaskPlan `json:"tasks,omitempty"`
	// Direct, when non-nil, signals the planner judged the request trivial
	// enough to answer without any investigation phase. The nil Direct (the
	// common case) means "investigate via Tasks". Mutually exclusive with
	// Tasks; rejected unless RunRequest.AllowDirect is true.
	Direct *DirectPlan `json:"direct,omitempty"`
}

// DirectPlan is the round-1 "answer directly" payload. It carries only the
// tools the single direct ReAct agent is permitted to call; everything else
// about the direct path (system prompt, history, loop limit) is supplied by
// the runtime, and the response is always plain text — structured-final
// generation (Run[T]) is not consulted on this path.
type DirectPlan struct {
	// Tools is the subset of RunRequest.KnownToolIDs the direct agent may
	// call. May be empty for a pure conversational reply that needs no tool.
	// Bounded by maxToolsPerTask.
	Tools []string `json:"tools,omitempty"`
}

// ReplanResult is the parsed shape of every subsequent planner round. The
// planner must choose EXACTLY ONE terminal-or-continuation action per round:
//   - Tasks: run another investigation phase.
//   - Question: ask the user (only when the host allows it).
//   - Finalize: declare completion and produce the final output.
//
// An output that sets none of the three is rejected (parseReplanResult) and
// folded back into another replan round. This is deliberate: the previous
// design treated "empty tasks + no question" as an implicit completion signal,
// so a planner that merely forgot to emit tasks would silently terminate (and,
// in structured hosts, commit) a half-finished turn. Completion is now an
// explicit act.
type ReplanResult struct {
	Message  string        `json:"message,omitempty"`
	Tasks    []TaskPlan    `json:"tasks,omitempty"`
	Question *Question     `json:"question,omitempty"`
	Finalize *FinalizePlan `json:"finalize,omitempty"`
}

// FinalizePlan is the planner's explicit "I'm done" declaration. It carries an
// optional short rationale; the actual user-visible output is produced by the
// entry point (final text, or the validated structured object) after the loop
// exits.
type FinalizePlan struct {
	// Reason is a 1-sentence rationale for terminating now (optional).
	Reason string `json:"reason,omitempty"`
}

// Question is the host-facing payload when the planner needs human input.
// proposal forwards it to the Slack question UI; the job host has
// AllowQuestion=false and therefore never sees one.
type Question struct {
	// Reason is the rationale shared across every item ("why am I
	// asking?").
	Reason string `json:"reason"`
	// Items is the ordered list of questions to ask (1..5 items,
	// enforced by Validate).
	Items []QuestionItem `json:"items"`
}

// QuestionItemType discriminates how the host should render the answer
// control. Closed-list types (select / multi_select) require non-empty
// Options; free_text is the last-resort prose-input shape and Options is
// ignored. The planner is told to prefer the closed-list types — see
// prompts/planner.md for the policy.
type QuestionItemType string

const (
	QuestionItemSelect      QuestionItemType = "select"
	QuestionItemMultiSelect QuestionItemType = "multi_select"
	QuestionItemFreeText    QuestionItemType = "free_text"
)

// QuestionItem is one question within Question.Items.
type QuestionItem struct {
	ID      string           `json:"id"`
	Text    string           `json:"text"`
	Type    QuestionItemType `json:"type"`
	Options []string         `json:"options,omitempty"`
}

// QuestionAnswer is the host's reply payload for one QuestionItem.
type QuestionAnswer struct {
	ID       string   `json:"id"`
	Choice   string   `json:"choice,omitempty"`    // select
	Choices  []string `json:"choices,omitempty"`   // multi_select
	FreeText string   `json:"free_text,omitempty"` // free_text
}

// QuestionResult is what the host returns from OnQuestion. The
// hecatoncheires proposal host uses the {Terminate=true} shape (the
// session ends after the planner asks) but the type is set up to also
// support warren-style in-loop continuation in the future.
type QuestionResult struct {
	// Terminate, when true, signals planexec.Runner to stop the loop
	// immediately and return RunStatus=Completed without invoking the
	// final-response phase. Used by proposal to defer the conversation
	// to the next thread reply.
	Terminate bool
	// Items, when Terminate=false, supplies the user's answers and the
	// loop continues with these injected into the next planner round.
	// Empty when Terminate=true.
	Items []QuestionAnswer
}

// Validate enforces TaskPlan invariants. KnownToolIDs is the
// host-supplied allowlist (RunRequest.KnownToolIDs); every entry in
// TaskPlan.Tools must be a member.
func (t *TaskPlan) Validate(knownToolIDs []string) error {
	if t == nil {
		return goerr.New("task plan is nil")
	}
	if strings.TrimSpace(t.ID) == "" {
		return goerr.New("task plan id is required")
	}
	if strings.TrimSpace(t.Title) == "" {
		return goerr.New("task plan title is required",
			goerr.V("task_id", t.ID))
	}
	if strings.TrimSpace(t.Description) == "" {
		return goerr.New("task plan description is required",
			goerr.V("task_id", t.ID))
	}
	if strings.TrimSpace(t.AcceptanceCriteria) == "" {
		return goerr.New("task plan acceptance criteria is required",
			goerr.V("task_id", t.ID))
	}
	if len(t.Tools) == 0 {
		return goerr.New("task plan tools must not be empty",
			goerr.V("task_id", t.ID))
	}
	if len(t.Tools) > maxToolsPerTask {
		return goerr.New("task plan tools too many entries",
			goerr.V("task_id", t.ID),
			goerr.V("got", len(t.Tools)),
			goerr.V("max", maxToolsPerTask))
	}
	for _, id := range t.Tools {
		if !slices.Contains(knownToolIDs, id) {
			return goerr.New("task plan tools contains unknown id",
				goerr.V("task_id", t.ID),
				goerr.V("tool_id", id),
				goerr.V("known", knownToolIDs))
		}
	}
	return nil
}

// Validate enforces DirectPlan invariants. knownToolIDs is the host-supplied
// allowlist (RunRequest.KnownToolIDs); every entry in Tools must be a member.
// An empty Tools list is allowed — a direct reply need not call any tool.
func (d *DirectPlan) Validate(knownToolIDs []string) error {
	if d == nil {
		return goerr.New("direct plan is nil")
	}
	if len(d.Tools) > maxToolsPerTask {
		return goerr.New("direct plan tools too many entries",
			goerr.V("got", len(d.Tools)),
			goerr.V("max", maxToolsPerTask))
	}
	for _, id := range d.Tools {
		if !slices.Contains(knownToolIDs, id) {
			return goerr.New("direct plan tools contains unknown id",
				goerr.V("tool_id", id),
				goerr.V("known", knownToolIDs))
		}
	}
	return nil
}

// Validate enforces Question invariants. Called from Validate on the
// containing ReplanResult.
func (q *Question) Validate() error {
	if q == nil {
		return goerr.New("question is nil")
	}
	if strings.TrimSpace(q.Reason) == "" {
		return goerr.New("question reason is required")
	}
	if n := len(q.Items); n < minQuestionItems || n > maxQuestionItems {
		return goerr.New("question items count out of range",
			goerr.V("got", n),
			goerr.V("min", minQuestionItems),
			goerr.V("max", maxQuestionItems))
	}
	seenID := make(map[string]struct{}, len(q.Items))
	for i := range q.Items {
		item := &q.Items[i]
		if err := item.Validate(); err != nil {
			return goerr.Wrap(err, "question item invalid",
				goerr.V("index", i))
		}
		if _, dup := seenID[item.ID]; dup {
			return goerr.New("question item id duplicated",
				goerr.V("id", item.ID))
		}
		seenID[item.ID] = struct{}{}
	}
	return nil
}

// Validate enforces QuestionItem invariants. The free_text exemption
// applies: free_text items skip the Options ≥2 rule and ignore any
// supplied options as a discardable hint.
func (i *QuestionItem) Validate() error {
	if i == nil {
		return goerr.New("question item is nil")
	}
	if strings.TrimSpace(i.ID) == "" {
		return goerr.New("question item id is required")
	}
	if strings.TrimSpace(i.Text) == "" {
		return goerr.New("question item text is required",
			goerr.V("id", i.ID))
	}
	switch i.Type {
	case QuestionItemSelect, QuestionItemMultiSelect:
		if len(i.Options) < minQuestionOptions {
			return goerr.New("question item options must contain at least the minimum",
				goerr.V("id", i.ID),
				goerr.V("got", len(i.Options)),
				goerr.V("min", minQuestionOptions))
		}
		seenOpt := make(map[string]struct{}, len(i.Options))
		for j, opt := range i.Options {
			if strings.TrimSpace(opt) == "" {
				return goerr.New("question item option must not be empty",
					goerr.V("id", i.ID), goerr.V("index", j))
			}
			if _, dup := seenOpt[opt]; dup {
				return goerr.New("question item option duplicated",
					goerr.V("id", i.ID), goerr.V("option", opt))
			}
			seenOpt[opt] = struct{}{}
		}
	case QuestionItemFreeText:
		// Options ignored.
	default:
		return goerr.New("question item type must be select, multi_select, or free_text",
			goerr.V("id", i.ID),
			goerr.V("got", string(i.Type)))
	}
	return nil
}

// parsePlanResult decodes and validates a first-round planner JSON. The
// planner returns one of two shapes:
//   - Direct (fast path): only honoured when allowDirect is true and Tasks
//     is empty. The direct.tools list is validated against knownToolIDs.
//   - Tasks (default): non-empty within bounds; every TaskPlan is validated
//     against knownToolIDs.
//
// allowDirect=false (e.g. a host that has not opted in) rejects a direct
// payload outright; the system prompt / schema should have suppressed the
// option but the parser double-checks.
func parsePlanResult(raw []byte, knownToolIDs []string, allowDirect bool) (*PlanResult, error) {
	body := extractJSONObject(raw)
	var p PlanResult
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, goerr.Wrap(err, "decode plan json")
	}
	if p.Direct != nil {
		if !allowDirect {
			return nil, goerr.New("plan produced a direct payload but AllowDirect is false")
		}
		if len(p.Tasks) > 0 {
			return nil, goerr.New("plan sets both tasks and direct; they are mutually exclusive")
		}
		if err := p.Direct.Validate(knownToolIDs); err != nil {
			return nil, goerr.Wrap(err, "direct plan invalid")
		}
		return &p, nil
	}
	if err := validateTaskList(p.Tasks, knownToolIDs); err != nil {
		return nil, err
	}
	return &p, nil
}

// parseReplanResult decodes and validates a subsequent-round planner JSON. The
// replan round must set EXACTLY ONE of three actions:
//   - Tasks (continuation): another phase runs
//   - Question: the host asks the user
//   - Finalize: the planner declares completion → the final output is produced
//
// Setting none is rejected (the caller folds it back into another replan round)
// so a planner that merely omitted tasks cannot silently terminate the turn.
// Setting more than one is rejected as ambiguous — the schema cannot express a
// oneOf, so the parser is the enforcement point.
//
// allowQuestion=false (job host) rejects a question payload outright; the
// system prompt should have suppressed the option but the parser double-checks.
func parseReplanResult(raw []byte, knownToolIDs []string, allowQuestion bool) (*ReplanResult, error) {
	body := extractJSONObject(raw)
	var r ReplanResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, goerr.Wrap(err, "decode replan json")
	}

	hasTasks := len(r.Tasks) > 0
	hasQuestion := r.Question != nil
	hasFinalize := r.Finalize != nil

	set := 0
	for _, on := range []bool{hasTasks, hasQuestion, hasFinalize} {
		if on {
			set++
		}
	}
	switch set {
	case 0:
		return nil, goerr.New("replan set no action: provide exactly one of tasks, question, or finalize")
	case 1:
		// ok
	default:
		return nil, goerr.New("replan set multiple actions: provide exactly one of tasks, question, or finalize",
			goerr.V("has_tasks", hasTasks),
			goerr.V("has_question", hasQuestion),
			goerr.V("has_finalize", hasFinalize))
	}

	if hasQuestion {
		if !allowQuestion {
			return nil, goerr.New("replan produced a question but AllowQuestion is false")
		}
		if err := r.Question.Validate(); err != nil {
			return nil, goerr.Wrap(err, "replan question invalid")
		}
		return &r, nil
	}
	if hasTasks {
		if err := validateTaskList(r.Tasks, knownToolIDs); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

func validateTaskList(tasks []TaskPlan, knownToolIDs []string) error {
	if n := len(tasks); n < minTasksPerPhase || n > maxTasksPerPhase {
		return goerr.New("tasks count out of range",
			goerr.V("got", n),
			goerr.V("min", minTasksPerPhase),
			goerr.V("max", maxTasksPerPhase))
	}
	seenID := make(map[string]struct{}, len(tasks))
	for i := range tasks {
		task := &tasks[i]
		if err := task.Validate(knownToolIDs); err != nil {
			return goerr.Wrap(err, "task invalid",
				goerr.V("index", i))
		}
		if _, dup := seenID[task.ID]; dup {
			return goerr.New("task id duplicated within plan",
				goerr.V("id", task.ID))
		}
		seenID[task.ID] = struct{}{}
	}
	return nil
}

// extractJSONObject returns the slice of raw containing a single
// top-level JSON object. We strip a ```json fence (or bare ``` fence)
// if present and then scan for the first balanced `{ … }` region. The
// scanner is the single source of truth for "which substring is the
// object" — an earlier first-and-last-char fast path silently returned
// the whole input for pathological cases like `{"a":1} {"b":2}` and
// pushed the json.Unmarshal failure downstream instead of letting the
// scanner extract just the first object. (Inherited verbatim from the
// proposal-side implementation; see draft/plan.go's prior version.)
func extractJSONObject(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}
	if bytes.HasPrefix(trimmed, []byte("```")) {
		body := trimmed[3:]
		if rest, ok := bytes.CutPrefix(body, []byte("json")); ok {
			body = rest
		}
		body = bytes.TrimLeft(body, "\r\n")
		if end := bytes.LastIndex(body, []byte("```")); end >= 0 {
			body = bytes.TrimSpace(body[:end])
		}
		trimmed = body
	}
	start := bytes.IndexByte(trimmed, '{')
	if start < 0 {
		return raw
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(trimmed); i++ {
		c := trimmed[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1]
			}
		}
	}
	return raw
}

// schemaOptions controls which sections appear in the planner schema for
// a given round. The first round uses planSchema (no question), every
// subsequent round uses replanSchema (question optional via
// AllowQuestion).
type schemaOptions struct {
	knownToolIDs  []string
	allowQuestion bool
	allowDirect   bool
}

// planSchema returns the gollem.Parameter applied to the planner LLM's
// JSON output for the FIRST round. `tasks` is always offered; `direct` is
// added only when the host opted in (allowDirect). The schema cannot express
// the tasks/direct mutual exclusion (gollem has no oneOf), so that, plus the
// task-count bounds, is enforced Go-side in parsePlanResult.
func planSchema(opts schemaOptions) *gollem.Parameter {
	props := map[string]*gollem.Parameter{
		"message": {
			Type:        gollem.TypeString,
			Description: "1-2 sentence rationale shown to the user.",
		},
		"tasks": tasksSchema(opts.knownToolIDs),
	}
	desc := "Initial planner output: parallel investigation tasks for round 1."
	if opts.allowDirect {
		props["direct"] = directSchema(opts.knownToolIDs)
		desc = "Initial planner output: either parallel investigation `tasks`, or a `direct` answer (round 1 only). Set exactly one."
	}
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: desc,
		Properties:  props,
	}
}

// directSchema returns the gollem.Parameter for the round-1 `direct` payload.
// Only `tools` (a subset of the known tool ids) is carried; the rest of the
// direct path is runtime-supplied.
func directSchema(knownToolIDs []string) *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Answer the user directly without any investigation phase. Mutually exclusive with `tasks`; set only one.",
		Properties: map[string]*gollem.Parameter{
			"tools": {
				Type:        gollem.TypeArray,
				Description: "Tool ids the direct agent may call (0-4). Omit or leave empty for a pure conversational reply.",
				Items: &gollem.Parameter{
					Type: gollem.TypeString,
					Enum: knownToolIDs,
				},
			},
		},
	}
}

// replanSchema returns the gollem.Parameter applied to the planner LLM's
// JSON output for every round AFTER the first. May include `question`
// when the host enabled it.
func replanSchema(opts schemaOptions) *gollem.Parameter {
	props := map[string]*gollem.Parameter{
		"message": {
			Type:        gollem.TypeString,
			Description: "1-2 sentence rationale shown to the user.",
		},
		"tasks":    tasksSchema(opts.knownToolIDs),
		"finalize": finalizeSchema(),
	}
	if opts.allowQuestion {
		props["question"] = questionSchema()
	}
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Replan output: set EXACTLY ONE of `tasks` (run another phase), `question` (ask the user), or `finalize` (declare completion). Leaving all unset is rejected — completion must be explicit via `finalize`.",
		Properties:  props,
	}
}

// finalizeSchema is the `finalize` action shape: an explicit completion
// declaration carrying only an optional rationale.
func finalizeSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Declare the turn complete and produce the final output. Set this (instead of `tasks`) when the goal is met — an empty `tasks` list alone does NOT signal completion.",
		Properties: map[string]*gollem.Parameter{
			"reason": {
				Type:        gollem.TypeString,
				Description: "Optional 1-sentence rationale for finishing now.",
			},
		},
	}
}

func tasksSchema(knownToolIDs []string) *gollem.Parameter {
	// Every field below is enforced by TaskPlan.Validate; mark them Required so
	// the model is compelled to emit them. Without this, models that omit (most
	// commonly) `description` send the planner into a retry loop that burns the
	// whole budget before the turn falls back.
	taskProps := map[string]*gollem.Parameter{
		"id":                  {Type: gollem.TypeString, Description: "Phase-unique identifier (e.g. inv-1).", Required: true},
		"title":               {Type: gollem.TypeString, Description: "Short label for the trace UI (~40 chars).", Required: true},
		"description":         {Type: gollem.TypeString, Description: "Detailed instruction for the sub-agent.", Required: true},
		"acceptance_criteria": {Type: gollem.TypeString, Description: "Measurable completion condition.", Required: true},
		"tools": {
			Type:        gollem.TypeArray,
			Description: "Allowed tool set IDs for this task.",
			Required:    true,
			Items: &gollem.Parameter{
				Type: gollem.TypeString,
				Enum: knownToolIDs,
			},
		},
	}
	return &gollem.Parameter{
		Type:        gollem.TypeArray,
		Description: "Parallel investigation tasks for this phase.",
		Items: &gollem.Parameter{
			Type:       gollem.TypeObject,
			Properties: taskProps,
		},
	}
}

func questionSchema() *gollem.Parameter {
	itemProps := map[string]*gollem.Parameter{
		"id":   {Type: gollem.TypeString, Description: "Item-unique identifier (e.g. q-1)."},
		"text": {Type: gollem.TypeString, Description: "Question text shown to the user."},
		"type": {
			Type:        gollem.TypeString,
			Description: "Answer control type. Use free_text only as a last resort.",
			Enum: []string{
				string(QuestionItemSelect),
				string(QuestionItemMultiSelect),
				string(QuestionItemFreeText),
			},
		},
		"options": {
			Type:        gollem.TypeArray,
			Description: "Allowed answer values for select / multi_select (≥2 entries). Ignored for free_text.",
			Items:       &gollem.Parameter{Type: gollem.TypeString},
		},
	}
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Ask the user for clarification. Use sparingly — prefer continuing investigation when possible.",
		Properties: map[string]*gollem.Parameter{
			"reason": {Type: gollem.TypeString, Description: "Why these questions are necessary."},
			"items": {
				Type:        gollem.TypeArray,
				Description: "Ordered list of questions (1-5).",
				Items: &gollem.Parameter{
					Type:       gollem.TypeObject,
					Properties: itemProps,
				},
			},
		},
	}
}
