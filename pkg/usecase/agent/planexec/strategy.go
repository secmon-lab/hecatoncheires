package planexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Phases. A transition reads the phase, does one thing, and writes the phase it
// leaves behind.
const (
	// phasePlan asks the planner what to do first.
	phasePlan = "plan"
	// phaseCollect folds the finished children into observations. No LLM call.
	phaseCollect = "collect"
	// phaseReplan asks the planner what to do next, given the observations.
	phaseReplan = "replan"
	// phaseFinal produces the terminal output.
	phaseFinal = "final"
)

// Model roles. A host binds them to a specific model through the Kernel; an
// unbound role falls back to the Kernel's default model.
var (
	// RolePlanner is the model that plans and replans.
	RolePlanner = agentkit.DefineModelRole("hecatoncheires.planner")
	// RoleFinalizer is the model that writes the terminal output.
	RoleFinalizer = agentkit.DefineModelRole("hecatoncheires.finalizer")
)

// Input is the launch input: what the host decided before the run existed.
// Everything the runtime supplies itself — history, traces, tools, the question
// channel — is deliberately absent.
type Input struct {
	// SystemPrompt is the host's base persona prompt. The planner prompt is
	// rendered around it.
	SystemPrompt string `json:"system_prompt"`
	// UserInput is the first user message.
	UserInput string `json:"user_input"`
	// LanguageLabel ("Japanese", "English", …) drives the user-facing-language
	// directive. Empty omits it.
	LanguageLabel string `json:"language_label,omitempty"`
	// KnownToolIDs is the toolset-id vocabulary the planner may assign to a task.
	// It is both the prompt's enumeration and the JSON schema's enum.
	KnownToolIDs []string `json:"known_tool_ids"`

	AllowQuestion       bool `json:"allow_question,omitempty"`
	AllowDirect         bool `json:"allow_direct,omitempty"`
	AllowSubAgentWrites bool `json:"allow_sub_agent_writes,omitempty"`

	// Progress locates the thread milestone lines are drawn into. A zero value
	// draws nothing.
	Progress ProgressTarget `json:"progress,omitempty"`
}

// Validate enforces what the planner cannot run without.
func (in Input) Validate() error {
	if in.SystemPrompt == "" {
		return goerr.New("system prompt is required")
	}
	if in.UserInput == "" {
		return goerr.New("user input is required")
	}
	if len(in.KnownToolIDs) == 0 {
		return goerr.New("known tool ids must not be empty")
	}
	return nil
}

// ProgressTarget is the thread a run draws its milestones into.
type ProgressTarget struct {
	ChannelID string `json:"channel_id,omitempty"`
	ThreadTS  string `json:"thread_ts,omitempty"`
}

// isZero reports whether there is nowhere to draw.
func (t ProgressTarget) isZero() bool { return t.ChannelID == "" || t.ThreadTS == "" }

// Progress draws a run's milestone lines. The host implements it; planexec holds
// no Slack dependency.
//
// It is stateless on purpose: the message id and the lines so far live in the
// run's checkpointed state, so another instance picking the run up keeps drawing
// into the same message instead of starting a second one.
type Progress interface {
	// Render draws lines as one message and returns its id. An empty messageTS
	// means "post a new one"; anything else means "update that one".
	Render(ctx context.Context, target ProgressTarget, messageTS string, lines []string) (string, error)
}

// OutputKind discriminates how a turn ended.
type OutputKind string

const (
	// OutputFinal is a planner-declared finalize that produced a terminal output.
	OutputFinal OutputKind = "final"
	// OutputDirect is the round-1 answer-without-investigation path.
	OutputDirect OutputKind = "direct"
	// OutputQuestion is the planner asking the user and ending the turn.
	OutputQuestion OutputKind = "question"
	// OutputFallback is a turn that reached no conclusion.
	OutputFallback OutputKind = "fallback"
)

// Output is what a finished run produces. Exactly one of Data / Text / Question
// carries the payload, selected by Kind.
type Output[T any] struct {
	Kind OutputKind `json:"kind"`
	// Data is the validated structured output. Set only for OutputFinal on a
	// structured host.
	Data *T `json:"data,omitempty"`
	// Text is the reply for OutputDirect, and for OutputFinal on a text-only host.
	Text string `json:"text,omitempty"`
	// Question is what to ask the user. Set only for OutputQuestion.
	Question *Question `json:"question,omitempty"`
	// FallbackReason says why no conclusion was reached.
	FallbackReason string `json:"fallback_reason,omitempty"`
	// Observations is the per-round trail, carried on every kind so even a
	// fallback can report what was learnt.
	Observations []PhaseSummary `json:"observations,omitempty"`
}

// TextResult is the T for hosts whose terminal output is prose rather than a
// structured object. Validate always accepts: there is no shape to check.
type TextResult struct {
	Text string `json:"text"`
}

// Validate satisfies Validatable.
func (TextResult) Validate() error { return nil }

// Config is the host's terminal-output contract.
type Config[T Validatable] struct {
	// Decode turns the terminal JSON into T. nil uses encoding/json.
	Decode func([]byte) (*T, error)
	// Finalizers validate the decoded output against host context Validate()
	// cannot see (a workspace field schema, say). A returned error is fed back to
	// the model and the output regenerated, so a finalizer MUST be
	// side-effect-free: a later attempt re-runs every one.
	Finalizers []func(*T) error
	// TextOnly generates the terminal output as prose instead of JSON and puts it
	// in Output.Text.
	TextOnly bool
}

// observationsMaxBytes bounds the checkpointed observation trail.
//
// The trail is by far the largest part of the state, and the state is written to
// one Firestore document on every transition. With the default budget a run can
// reach ~21 rounds x 5 tasks x 8 KiB of summary, which would cross Firestore's
// 1 MiB document limit and strand the run with no way to commit. Dropping the
// oldest rounds keeps the recent, relevant observations and leaves a note saying
// what went.
const observationsMaxBytes = 256 * 1024

// state is the checkpointed state. Everything the next transition needs must be
// here: Step runs from the top every time, including after a crash on another
// instance.
type state struct {
	Phase string `json:"phase"`
	Round int    `json:"round"`
	Input Input  `json:"input"`
	// NextInput is the user message the next planner round receives.
	NextInput    string         `json:"next_input"`
	Observations []PhaseSummary `json:"observations,omitempty"`
	// Current are the children this round launched, in plan order.
	Current  []taskRef         `json:"current,omitempty"`
	RoundKey agentkit.AwaitKey `json:"round_key,omitempty"`
	// Direct marks the current round as the direct fast path, whose single
	// child's answer IS the turn's reply.
	Direct   bool          `json:"direct,omitempty"`
	Progress progressState `json:"progress,omitempty"`
	// FinalRetries counts terminal outputs rejected by decode / Validate /
	// finalizers.
	FinalRetries int `json:"final_retries,omitempty"`
	// Wrap records that the budget notice was seen, so the run stops planning and
	// goes to produce an answer with what it has.
	Wrap bool `json:"wrap,omitempty"`
}

// taskRef ties a planned task to the child Process running it. The plan fields
// are copied rather than looked up because the child's result must be
// attributable even if the round that produced it has been dropped from the
// trail.
type taskRef struct {
	TaskID             string             `json:"task_id"`
	Title              string             `json:"title"`
	AcceptanceCriteria string             `json:"acceptance_criteria"`
	ProcessID          agentkit.ProcessID `json:"process_id"`
}

// progressState is the minimum needed to keep drawing into the same message
// after the run moves to another instance.
type progressState struct {
	MessageTS string   `json:"message_ts,omitempty"`
	Lines     []string `json:"lines,omitempty"`
}

// Register registers the strategy under name and returns the typed handle.
//
// taskAgent is the agent each planned task runs as — a ReAct agent, spawned as a
// child. limiter is the budget this run answers Limit with; it is required,
// because a run with no ceiling is a run that can spend without bound. progress
// may be nil, which draws nothing.
func Register[T Validatable](
	reg *agentkit.Registry, name agentkit.AgentName, version int,
	taskAgent agentkit.Agent[react.Input], progress Progress,
	limiter agentkit.Limiter, cfg Config[T],
	opts ...agentkit.RegisterOption[Output[T]],
) (agentkit.Agent[Input], error) {
	if limiter == nil {
		return agentkit.Agent[Input]{}, goerr.Wrap(agentkit.ErrInvalidAgentDef,
			"planexec: a limiter is required", goerr.V("agent", name))
	}
	if taskAgent.Name() == "" {
		return agentkit.Agent[Input]{}, goerr.Wrap(agentkit.ErrInvalidAgentDef,
			"planexec: a task agent is required", goerr.V("agent", name))
	}
	s := &strategy[T]{
		limiter:   limiter,
		version:   version,
		taskAgent: taskAgent,
		progress:  progress,
		cfg:       cfg,
	}
	return agentkit.Register(reg, name, version, s, opts...)
}

type strategy[T Validatable] struct {
	limiter   agentkit.Limiter
	version   int
	taskAgent agentkit.Agent[react.Input]
	progress  Progress
	cfg       Config[T]
}

func (s *strategy[T]) Version() int { return s.version }

func (s *strategy[T]) Limit(ctx context.Context, proc *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
	return s.limiter(ctx, proc, m)
}

func (s *strategy[T]) Init(in Input) (state, error) {
	if err := in.Validate(); err != nil {
		return state{}, goerr.Wrap(err, "planexec: invalid input")
	}
	return state{Phase: phasePlan, Round: 1, Input: in, NextInput: in.UserInput}, nil
}

func (s *strategy[T]) Step(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	// A budget notice has to be TOLD to the run, not merely enforced against it:
	// enforcement alone cuts it off mid-investigation with no answer. It is
	// recorded on the state rather than acted on here, because the phase that can
	// act may be several transitions away — a round waiting for its children
	// cannot abandon them.
	if sys.LimitStatus().Kind() == agentkit.LimitKindNotice {
		st.Wrap = true
	}

	switch st.Phase {
	case phasePlan:
		return s.stepPlan(ctx, sys, st)
	case phaseCollect:
		return s.stepCollect(ctx, sys, st)
	case phaseReplan:
		return s.stepReplan(ctx, sys, st)
	case phaseFinal:
		return s.stepFinal(ctx, sys, st)
	default:
		return st, agentkit.Decision[Output[T]]{}, goerr.New("planexec: unknown phase",
			goerr.V("phase", st.Phase))
	}
}

// stepPlan makes the opening planner call and launches the first round.
func (s *strategy[T]) stepPlan(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	if st.Wrap {
		// Nothing has been investigated, but the budget is already nearly gone.
		// Answering from an empty trail beats spending the remainder on a plan
		// that could never be executed.
		st.Phase = phaseFinal
		return st, agentkit.Continue[Output[T]](), nil
	}

	st = s.note(ctx, st, "🧭 Planning")

	prompt, err := s.plannerPrompt(st)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, err
	}
	// The opening round may not ask a question: it has nothing to ask about yet.
	schema := planSchema(schemaOptions{
		knownToolIDs: st.Input.KnownToolIDs,
		allowDirect:  st.Input.AllowDirect,
	})

	res, err := sys.Session().Generate(ctx, []gollem.Input{gollem.Text(st.NextInput)},
		agentkit.WithSystemPrompt(prompt),
		agentkit.WithSchema(schema),
		agentkit.WithRole(RolePlanner),
	)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: planner generate")
	}

	plan, perr := parsePlanResult([]byte(strings.Join(res.Texts, "\n")),
		st.Input.KnownToolIDs, st.Input.AllowDirect)
	if perr != nil {
		st = s.carryCorrection(ctx, st, perr, "planexec: planner output rejected")
		return st, agentkit.Continue[Output[T]](), nil
	}

	if plan.Direct != nil {
		return s.launchDirect(ctx, sys, st, plan.Direct)
	}
	return s.launchRound(ctx, sys, st, plan.Tasks)
}

// stepCollect folds the finished children into observations. It makes no LLM
// call, which is why it is its own transition: the children's results are
// committed before anything is asked of the model.
func (s *strategy[T]) stepCollect(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	aw, ok := sys.Await(st.RoundKey)
	if !ok {
		return st, agentkit.Decision[Output[T]]{}, goerr.New("planexec: round await is missing",
			goerr.V("key", string(st.RoundKey)), goerr.V("round", st.Round))
	}
	if aw.Status != agentkit.AwaitResponded {
		return st, agentkit.Decision[Output[T]]{}, goerr.New("planexec: round await is not resolved",
			goerr.V("key", string(st.RoundKey)), goerr.V("status", string(aw.Status)))
	}

	results := collectResults(st.Current, aw.Results)

	if st.Direct {
		// The direct path's single child IS the answer.
		text := ""
		if len(results) > 0 {
			text = results[0].Summary
		}
		if text == "" {
			// A direct child that produced nothing is a dead end, not an answer:
			// replying with an empty message would look like the agent ignored the
			// user.
			return st, agentkit.Done(Output[T]{
				Kind:           OutputFallback,
				FallbackReason: "the direct reply produced no text",
				Observations:   st.Observations,
			}), nil
		}
		st = s.note(ctx, st, "✅ Answered directly")
		return st, agentkit.Done(Output[T]{
			Kind: OutputDirect, Text: text, Observations: st.Observations,
		}), nil
	}

	tasks := make([]TaskPlan, 0, len(st.Current))
	for _, ref := range st.Current {
		tasks = append(tasks, TaskPlan{
			ID: ref.TaskID, Title: ref.Title, AcceptanceCriteria: ref.AcceptanceCriteria,
		})
	}
	st.Observations = appendObservations(st.Observations,
		PhaseSummary{Phase: st.Round, Tasks: tasks, Results: results})
	st = s.note(ctx, st, roundSummaryLine(st.Round, results))

	st.NextInput = formatObservationsAsUserTurn(tasks, results)
	st.Current = nil
	st.RoundKey = ""
	st.Phase = phaseReplan
	return st, agentkit.Continue[Output[T]](), nil
}

// stepReplan asks the planner what to do with the observations so far.
func (s *strategy[T]) stepReplan(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	if st.Wrap {
		st.Phase = phaseFinal
		return st, agentkit.Continue[Output[T]](), nil
	}

	st = s.note(ctx, st, "🧭 Re-planning")

	prompt, err := s.plannerPrompt(st)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, err
	}
	schema := replanSchema(schemaOptions{
		knownToolIDs:  st.Input.KnownToolIDs,
		allowQuestion: st.Input.AllowQuestion,
	})

	res, err := sys.Session().Generate(ctx, []gollem.Input{gollem.Text(st.NextInput)},
		agentkit.WithSystemPrompt(prompt),
		agentkit.WithSchema(schema),
		agentkit.WithRole(RolePlanner),
	)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: replanner generate")
	}

	rr, perr := parseReplanResult([]byte(strings.Join(res.Texts, "\n")),
		st.Input.KnownToolIDs, st.Input.AllowQuestion)
	if perr != nil {
		st = s.carryCorrection(ctx, st, perr, "planexec: replanner output rejected")
		return st, agentkit.Continue[Output[T]](), nil
	}

	switch {
	case rr.Question != nil:
		// The turn ENDS on a question rather than waiting on an await. Holding the
		// run open while a person takes minutes or hours would pin its subject and
		// block every later turn on the thread; the answer arrives as a fresh run
		// that inherits this one's history.
		st = s.note(ctx, st, "❓ Asked a question")
		return st, agentkit.Done(Output[T]{
			Kind: OutputQuestion, Question: rr.Question, Observations: st.Observations,
		}), nil
	case rr.Finalize != nil:
		st.Phase = phaseFinal
		return st, agentkit.Continue[Output[T]](), nil
	default:
		st.Round++
		return s.launchRound(ctx, sys, st, rr.Tasks)
	}
}

// stepFinal produces the terminal output.
func (s *strategy[T]) stepFinal(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	st = s.note(ctx, st, "📝 Writing the answer")

	// A retry carries the rejection reason instead of the original request: the
	// model already has the request in the conversation.
	userPrompt := st.NextInput
	if st.FinalRetries == 0 || userPrompt == "" {
		rendered, err := renderFinalUserPrompt(finalPromptInput{
			Observations:    renderObservationsForFinal(st.Observations),
			StructuredFinal: !s.cfg.TextOnly,
			Language:        st.Input.LanguageLabel,
		})
		if err != nil {
			return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: render final prompt")
		}
		userPrompt = rendered
	}

	prompt, err := s.plannerPrompt(st)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, err
	}

	opts := []agentkit.GenerateOption{
		agentkit.WithSystemPrompt(prompt),
		agentkit.WithRole(RoleFinalizer),
	}
	if !s.cfg.TextOnly {
		var zero T
		schema, serr := gollem.ToSchema(zero)
		if serr != nil {
			return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(serr,
				"planexec: derive the final output schema from its type")
		}
		opts = append(opts, agentkit.WithSchema(schema))
	}

	res, err := sys.Session().Generate(ctx, []gollem.Input{gollem.Text(userPrompt)}, opts...)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: final generate")
	}
	body := strings.Join(res.Texts, "\n")
	if strings.TrimSpace(body) == "" {
		return st, agentkit.Done(Output[T]{
			Kind:           OutputFallback,
			FallbackReason: "the final response was empty",
			Observations:   st.Observations,
		}), nil
	}

	if s.cfg.TextOnly {
		return st, agentkit.Done(Output[T]{
			Kind: OutputFinal, Text: body, Observations: st.Observations,
		}), nil
	}

	out, derr := s.decodeFinal([]byte(body))
	if derr == nil {
		derr = (*out).Validate()
	}
	if derr == nil {
		derr = runFinalizers(out, s.cfg.Finalizers)
	}
	if derr != nil {
		st.FinalRetries++
		if st.FinalRetries > finalOutputMaxRetry {
			return st, agentkit.Done(Output[T]{
				Kind:           OutputFallback,
				FallbackReason: derr.Error(),
				Observations:   st.Observations,
			}), nil
		}
		// Same discipline as a rejected plan: the correction goes to the next
		// transition, not to a loop inside this one.
		errutil.Handle(ctx, goerr.Wrap(derr, "planexec: final output rejected",
			goerr.T(errutil.TagBenign)), "planexec: final output rejected")
		st.NextInput = finalRetryInput(derr)
		return st, agentkit.Continue[Output[T]](), nil
	}

	return st, agentkit.Done(Output[T]{
		Kind: OutputFinal, Data: out, Observations: st.Observations,
	}), nil
}

// carryCorrection records a rejected planner output and puts the reason in front
// of the next planner round.
//
// The retry is deliberately a separate transition rather than a loop inside this
// one: one transition is one LLM call, so a retry loop here would spend the
// budget without checkpointing between attempts, and a crash would repeat the
// whole thing.
func (s *strategy[T]) carryCorrection(ctx context.Context, st state, cause error, msg string) state {
	errutil.Handle(ctx, goerr.Wrap(cause, msg, goerr.T(errutil.TagBenign)), msg)
	st.NextInput = planRetryInput(cause)
	return st
}

// decodeFinal turns the terminal JSON into T, through the host's decoder when it
// supplied one.
func (s *strategy[T]) decodeFinal(raw []byte) (*T, error) {
	body := extractJSONObject(raw)
	if s.cfg.Decode != nil {
		out, err := s.cfg.Decode(body)
		if err != nil {
			return nil, goerr.Wrap(err, "decode the final output")
		}
		if out == nil {
			return nil, goerr.New("the final output decoder returned nothing")
		}
		return out, nil
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, goerr.Wrap(err, "decode the final output json")
	}
	return &out, nil
}

// launchRound spawns one child per task and suspends until they all finish.
func (s *strategy[T]) launchRound(ctx context.Context, sys agentkit.Syscalls, st state, tasks []TaskPlan) (state, agentkit.Decision[Output[T]], error) {
	if len(tasks) == 0 {
		// Plan validation rejects an empty task list, so reaching here means the
		// planner emitted a shape no branch handles. Finishing from what we have
		// beats another round that would ask the same question again.
		st.Phase = phaseFinal
		return st, agentkit.Continue[Output[T]](), nil
	}
	if len(tasks) > maxTasksPerPhase {
		tasks = tasks[:maxTasksPerPhase]
	}

	refs := make([]taskRef, 0, len(tasks))
	ids := make([]agentkit.ProcessID, 0, len(tasks))
	for _, task := range tasks {
		pid, err := s.spawnTask(ctx, sys, st, task)
		if err != nil {
			return st, agentkit.Decision[Output[T]]{}, err
		}
		refs = append(refs, taskRef{
			TaskID: task.ID, Title: task.Title,
			AcceptanceCriteria: task.AcceptanceCriteria, ProcessID: pid,
		})
		ids = append(ids, pid)
	}

	st = s.note(ctx, st, fmt.Sprintf("🔎 Investigating (%d task(s))", len(refs)))
	st.Current = refs
	st.Direct = false
	st.RoundKey = agentkit.AwaitKey(fmt.Sprintf("tasks:%d", st.Round))
	st.Phase = phaseCollect
	return st, agentkit.Suspend[Output[T]](agentkit.WaitChildren(st.RoundKey, ids...)), nil
}

// launchDirect spawns the single child that answers a trivial request outright.
func (s *strategy[T]) launchDirect(ctx context.Context, sys agentkit.Syscalls, st state, plan *DirectPlan) (state, agentkit.Decision[Output[T]], error) {
	task := TaskPlan{
		ID:                 "direct",
		Title:              "Direct reply",
		Description:        st.Input.UserInput,
		AcceptanceCriteria: "The user's request is answered directly.",
		Tools:              plan.Tools,
	}
	pid, err := s.spawnTask(ctx, sys, st, task)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, err
	}
	st.Current = []taskRef{{TaskID: task.ID, Title: task.Title, ProcessID: pid}}
	st.Direct = true
	st.RoundKey = agentkit.AwaitKey("direct")
	st.Phase = phaseCollect
	return st, agentkit.Suspend[Output[T]](agentkit.WaitChildren(st.RoundKey, pid)), nil
}

// spawnTask launches one task as a child Process.
func (s *strategy[T]) spawnTask(ctx context.Context, sys agentkit.Syscalls, st state, task TaskPlan) (agentkit.ProcessID, error) {
	prompt, err := buildSubAgentSystemPrompt(task, st.Input.AllowSubAgentWrites)
	if err != nil {
		return "", goerr.Wrap(err, "planexec: build the task prompt", goerr.V("task_id", task.ID))
	}
	description := task.Description
	if description == "" {
		description = task.Title
	}
	// WithToolSets rebuilds the parent's metadata map with only the toolsets
	// replaced. SpawnChild's WithMetadata REPLACES the map rather than merging
	// into it, so building a fresh one here would drop the workspace and case the
	// child needs to have any tools at all.
	pid, err := s.taskAgent.SpawnChild(ctx, sys,
		react.Input{SystemPrompt: prompt, Prompt: description},
		agentkit.WithMetadata(agentkernel.WithToolSets(sys.Metadata(), task.Tools)),
	)
	if err != nil {
		return "", goerr.Wrap(err, "planexec: spawn the task", goerr.V("task_id", task.ID))
	}
	return pid, nil
}

// collectResults turns the children's outcomes into TaskResults, in plan order.
func collectResults(refs []taskRef, children []agentkit.ChildResult) []TaskResult {
	byID := make(map[agentkit.ProcessID]agentkit.ChildResult, len(children))
	for _, c := range children {
		byID[c.ProcessID] = c
	}

	out := make([]TaskResult, 0, len(refs))
	for _, ref := range refs {
		res := TaskResult{
			TaskID: ref.TaskID, Title: ref.Title,
			AcceptanceCriteria: ref.AcceptanceCriteria,
		}
		child, ok := byID[ref.ProcessID]
		switch {
		case !ok:
			// The await resolved without this child. Reporting it as failed keeps
			// the planner's picture complete rather than silently short.
			res.Status = TaskStatusFailed
			res.Error = "the task produced no result"
		case child.Status == agentkit.ProcessSucceeded:
			text, err := react.DecodeOutput(child.Output)
			if err != nil {
				res.Status = TaskStatusFailed
				res.Error = "the task output could not be read: " + err.Error()
				break
			}
			res.Status = TaskStatusCompleted
			res.Summary = truncateSummary(text.Text())
		default:
			res.Status = TaskStatusFailed
			res.Error = childFailureMessage(child)
		}
		out = append(out, res)
	}
	return out
}

// childFailureMessage describes why a child did not succeed.
func childFailureMessage(child agentkit.ChildResult) string {
	if child.Failure != nil && child.Failure.Message != "" {
		return child.Failure.Message
	}
	return "the task ended as " + string(child.Status)
}

// appendObservations adds a round to the trail and drops the oldest rounds once
// the trail outgrows what a checkpoint can hold.
func appendObservations(trail []PhaseSummary, next PhaseSummary) []PhaseSummary {
	trail = append(trail, next)
	for len(trail) > 1 && observationsSize(trail) > observationsMaxBytes {
		dropped := trail[0]
		trail = trail[1:]
		// The note replaces the dropped round rather than deleting it silently, so
		// the planner can tell "nothing was found" from "that round is no longer in
		// front of me".
		trail[0].Tasks = append([]TaskPlan{{
			ID:    fmt.Sprintf("dropped-%d", dropped.Phase),
			Title: fmt.Sprintf("round %d was dropped to stay within the state size limit", dropped.Phase),
		}}, trail[0].Tasks...)
	}
	return trail
}

// observationsSize measures the trail the way the checkpoint will store it.
func observationsSize(trail []PhaseSummary) int {
	raw, err := json.Marshal(trail)
	if err != nil {
		// Unmeasurable means "assume it is too big": under-reporting would let the
		// state grow past what the store accepts, which strands the run.
		return observationsMaxBytes + 1
	}
	return len(raw)
}

// roundSummaryLine renders one round's outcome as a milestone.
func roundSummaryLine(round int, results []TaskResult) string {
	done, failed := 0, 0
	for _, r := range results {
		if r.Status == TaskStatusCompleted {
			done++
			continue
		}
		failed++
	}
	if failed == 0 {
		return fmt.Sprintf("✅ Round %d: %d task(s) done", round, done)
	}
	return fmt.Sprintf("⚠️ Round %d: %d done, %d failed", round, done, failed)
}

// plannerPrompt renders the planner system prompt for the run's input.
func (s *strategy[T]) plannerPrompt(st state) (string, error) {
	prompt, err := renderPlannerSystemPrompt(plannerPromptInput{
		HostPrompt:          st.Input.SystemPrompt,
		Language:            st.Input.LanguageLabel,
		KnownToolIDs:        st.Input.KnownToolIDs,
		AllowQuestion:       st.Input.AllowQuestion,
		AllowDirect:         st.Input.AllowDirect,
		StructuredFinal:     !s.cfg.TextOnly,
		AllowSubAgentWrites: st.Input.AllowSubAgentWrites,
	})
	if err != nil {
		return "", goerr.Wrap(err, "planexec: render the planner prompt")
	}
	return prompt, nil
}

// note appends a milestone and draws it. Drawing is observability: a failure
// leaves the thread without a progress line but must never fail the turn.
func (s *strategy[T]) note(ctx context.Context, st state, line string) state {
	if line == "" {
		return st
	}
	st.Progress.Lines = append(st.Progress.Lines, line)
	if s.progress == nil || st.Input.Progress.isZero() {
		return st
	}
	ts, err := s.progress.Render(ctx, st.Input.Progress, st.Progress.MessageTS, st.Progress.Lines)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "planexec: draw the progress message"),
			"planexec: draw the progress message")
		return st
	}
	if ts != "" {
		st.Progress.MessageTS = ts
	}
	return st
}

// planRetryInput is the correction handed to the planner after its output was
// rejected.
func planRetryInput(cause error) string {
	return "Your previous response was rejected: " + cause.Error() +
		". Emit a single JSON object that matches the schema exactly."
}

func (s *strategy[T]) EncodeState(st state) ([]byte, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, goerr.Wrap(err, "planexec: encode state")
	}
	return raw, nil
}

func (s *strategy[T]) DecodeState(_ int, raw []byte) (state, error) {
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return state{}, goerr.Wrap(err, "planexec: decode state")
	}
	return st, nil
}

func (s *strategy[T]) EncodeOutput(out Output[T]) ([]byte, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, goerr.Wrap(err, "planexec: encode output")
	}
	return raw, nil
}

// DecodeOutput reads back the bytes a finished Process stored. A host reading a
// completed run's output needs this; the kernel itself never calls it.
func DecodeOutput[T Validatable](raw []byte) (Output[T], error) {
	var out Output[T]
	if err := json.Unmarshal(raw, &out); err != nil {
		return Output[T]{}, goerr.Wrap(err, "planexec: decode output")
	}
	return out, nil
}
