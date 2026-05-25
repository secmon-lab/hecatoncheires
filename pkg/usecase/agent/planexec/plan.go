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

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
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
// At least one task is required for the first round; if the host wants
// the planner to be able to terminate immediately, it should issue a
// replan round instead.
type PlanResult struct {
	// Message is a 1-2 sentence rationale shown to the user via
	// Sink.PlanProposed.
	Message string `json:"message,omitempty"`
	// Tasks is the parallel investigation phase emitted by the planner.
	Tasks []TaskPlan `json:"tasks"`
}

// ReplanResult is the parsed shape of every subsequent planner round.
// If Question is non-nil it takes priority over Tasks (the latter are
// ignored). Both being empty / nil signals "we're done — run the final
// response".
type ReplanResult struct {
	Message  string     `json:"message,omitempty"`
	Tasks    []TaskPlan `json:"tasks"`
	Question *Question  `json:"question,omitempty"`
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

// parsePlanResult decodes and validates a first-round planner JSON.
// Tasks must be non-empty within bounds; every TaskPlan is validated
// against knownToolIDs.
func parsePlanResult(raw []byte, knownToolIDs []string) (*PlanResult, error) {
	body := extractJSONObject(raw)
	var p PlanResult
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, goerr.Wrap(err, "decode plan json")
	}
	if err := validateTaskList(p.Tasks, knownToolIDs); err != nil {
		return nil, err
	}
	return &p, nil
}

// parseReplanResult decodes and validates a subsequent-round planner
// JSON. The replan round allows three shapes:
//   - Question (priority): host gets asked
//   - Tasks (continuation): another phase runs
//   - Both nil/empty: loop terminates and final-response phase runs
//
// allowQuestion=false (job host) rejects a question payload outright; the
// system prompt should have suppressed the option but the parser
// double-checks.
func parseReplanResult(raw []byte, knownToolIDs []string, allowQuestion bool) (*ReplanResult, error) {
	body := extractJSONObject(raw)
	var r ReplanResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, goerr.Wrap(err, "decode replan json")
	}
	if r.Question != nil {
		if !allowQuestion {
			return nil, goerr.New("replan produced a question but AllowQuestion is false")
		}
		if err := r.Question.Validate(); err != nil {
			return nil, goerr.Wrap(err, "replan question invalid")
		}
		// Question takes priority; ignore any Tasks set alongside it.
		r.Tasks = nil
		return &r, nil
	}
	if len(r.Tasks) > 0 {
		if err := validateTaskList(r.Tasks, knownToolIDs); err != nil {
			return nil, err
		}
	}
	// Tasks empty + Question nil is the legitimate termination signal.
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
}

// planSchema returns the gollem.Parameter applied to the planner LLM's
// JSON output for the FIRST round. Only `tasks` is allowed.
func planSchema(opts schemaOptions) *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Initial planner output: parallel investigation tasks for round 1.",
		Properties: map[string]*gollem.Parameter{
			"message": {
				Type:        gollem.TypeString,
				Description: "1-2 sentence rationale shown to the user.",
			},
			"tasks": tasksSchema(opts.knownToolIDs),
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
		"tasks": tasksSchema(opts.knownToolIDs),
	}
	if opts.allowQuestion {
		props["question"] = questionSchema()
	}
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Replan output: continue with more tasks, ask the user, or signal completion (both empty).",
		Properties:  props,
	}
}

func tasksSchema(knownToolIDs []string) *gollem.Parameter {
	taskProps := map[string]*gollem.Parameter{
		"id":                  {Type: gollem.TypeString, Description: "Phase-unique identifier (e.g. inv-1)."},
		"title":               {Type: gollem.TypeString, Description: "Short label for the trace UI (~40 chars)."},
		"description":         {Type: gollem.TypeString, Description: "Detailed instruction for the sub-agent."},
		"acceptance_criteria": {Type: gollem.TypeString, Description: "Measurable completion condition."},
		"tools": {
			Type:        gollem.TypeArray,
			Description: "Allowed tool set IDs for this task.",
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
