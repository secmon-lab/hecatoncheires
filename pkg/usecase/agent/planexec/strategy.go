package planexec

import (
	"context"
	"encoding/json"
	"errors"
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
	// phasePlannerTool runs ONE tool call the planner asked for before it decides.
	// It is a phase of its own, rather than a loop inside the planning transition,
	// for the same reason every other step is: one transition is one LLM call or
	// one tool call, so a crash costs at most one of them.
	phasePlannerTool = "planner_tool"
	// phaseAnswer folds a human's answer back into the run. It is reached only by
	// a host that asked to wait in-band (Input.SuspendOnQuestion); every other host
	// ends the turn on a question instead.
	phaseAnswer = "answer"
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
	// SuspendOnQuestion keeps the run open across the human's reply instead of
	// ending the turn on it: the run parks on a question await and continues when
	// the host calls Kernel.Respond with the answer.
	//
	// A host should set it ONLY when its own record spans the wait — an
	// interactive Job, whose run id covers the whole exchange. For a Slack thread
	// it is the wrong trade: the run would hold the thread's subject for as long as
	// the person takes to answer, blocking every later turn on that thread.
	SuspendOnQuestion bool `json:"suspend_on_question,omitempty"`

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

// Asker delivers a run's question to whoever can answer it, for a host that waits
// in-band (Input.SuspendOnQuestion). The host implements it; planexec holds no
// Slack dependency.
//
// pid and key are what the answer must be Responded to — the host records the
// pair alongside whatever it posts, because the reply arrives later, out of band,
// on an instance that never saw this transition. meta is the run's Process
// metadata, which is what lets one registered Asker serve every run.
//
// It posts before the run suspends, so a transition that is replayed would post
// twice. That is why every Serve in this application bounds unclean reclaims to 0
// (agentkernel.Serve): a replay fails the run instead of re-posting.
type Asker interface {
	Ask(ctx context.Context, pid agentkit.ProcessID, meta map[string]string,
		key agentkit.AwaitKey, q Question) error
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

// Finalizer validates a decoded terminal output against context T.Validate()
// cannot see — a workspace field schema, say. A returned error is fed back to the
// model and the output regenerated.
//
// It MUST be side-effect-free: a later attempt re-runs every finalizer, so
// committing anything here would commit it several times. Committing the output
// happens after the turn, never in a finalizer.
//
// meta is the run's Process metadata, which is what lets one registered finalizer
// serve every run: the host reads its own scope back out of it (the workspace,
// the case) instead of closing over a single run's values, since a strategy is
// registered once at startup and then serves every run of that agent.
type Finalizer[T any] func(ctx context.Context, meta map[string]string, out *T) error

// Config is the host's terminal-output contract.
type Config[T Validatable] struct {
	// Decode turns the terminal JSON into T. nil uses encoding/json.
	Decode func([]byte) (*T, error)
	// Finalizers run in order after T.Validate(); the first rejection wins.
	Finalizers []Finalizer[T]
	// Asker delivers a question for a host that waits in-band. Required when a run
	// sets Input.SuspendOnQuestion — without one there would be nobody to show the
	// question to, and the run would park on an await nothing can answer.
	Asker Asker
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
	// PendingCalls are the tool calls the planner asked for before deciding, and
	// this run has not made yet. They are consumed one per transition.
	PendingCalls []*gollem.FunctionCall `json:"pending_calls,omitempty"`
	// AfterTool is the phase to return to once PendingCalls is drained — the
	// planning phase that asked for them.
	AfterTool string `json:"after_tool,omitempty"`
	// PlannerToolRounds counts how many times the planner has asked for tools
	// within one planning phase, so a model that only ever calls tools cannot spin
	// forever inside a single round.
	PlannerToolRounds int `json:"planner_tool_rounds,omitempty"`
	// AnswerKey is the await a suspended run is waiting on. It is per-round so a
	// follow-up question opens a fresh await rather than reusing a closed one.
	AnswerKey agentkit.AwaitKey `json:"answer_key,omitempty"`
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
	case phasePlannerTool:
		return s.stepPlannerTool(ctx, sys, st)
	case phaseAnswer:
		return s.stepAnswer(ctx, sys, st)
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

	res, err := sys.Session().Generate(ctx, plannerInput(st),
		agentkit.WithSystemPrompt(prompt),
		agentkit.WithSchema(schema),
		agentkit.WithRole(RolePlanner),
	)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: planner generate")
	}

	if next, diverted := s.divertToTools(st, res, phasePlan); diverted {
		return next, agentkit.Continue[Output[T]](), nil
	}

	plan, perr := parsePlanResult([]byte(strings.Join(res.Texts, "\n")),
		st.Input.KnownToolIDs, st.Input.AllowDirect)
	if perr != nil {
		st = s.carryCorrection(ctx, st, perr, "planexec: planner output rejected")
		return st, agentkit.Continue[Output[T]](), nil
	}

	st.PlannerToolRounds = 0
	if plan.Direct != nil {
		return s.launchDirect(ctx, sys, st, plan.Direct)
	}
	return s.launchRound(ctx, sys, st, plan.Tasks)
}

// plannerInput is the user turn a planning call sends.
//
// An empty NextInput sends nothing and continues from the conversation as it
// stands, which is what follows the tool results a previous transition appended:
// re-sending the original request there would ask the same question again as
// though nothing had been learnt.
func plannerInput(st state) []gollem.Input {
	if st.NextInput == "" {
		return nil
	}
	return []gollem.Input{gollem.Text(st.NextInput)}
}

// plannerToolRoundsMax bounds how many times the planner may ask for tools before
// committing to a decision, within one planning phase.
//
// It exists because the planner's tool calls are free of the round budget — they
// are not sub-agent work — so a model that answers every prompt with another
// lookup would otherwise only be stopped by the token ceiling, having produced
// nothing. Past the bound the tools are withheld and the planner must decide from
// what it has.
const plannerToolRoundsMax = 4

// divertToTools sends the run to the tool phase when the planner asked for tools
// instead of deciding, and reports whether it did.
//
// Beyond plannerToolRoundsMax it does not divert: the calls are dropped and the
// planner's own text is parsed as usual, which fails validation and comes back as
// a correction telling it to decide.
func (s *strategy[T]) divertToTools(st state, res *agentkit.GenerateResult, from string) (state, bool) {
	if len(res.FunctionCalls) == 0 || st.PlannerToolRounds >= plannerToolRoundsMax {
		return st, false
	}
	st.PendingCalls = res.FunctionCalls
	st.AfterTool = from
	st.PlannerToolRounds++
	st.Phase = phasePlannerTool
	// The next planning call continues from the conversation, which by then carries
	// the tool results; re-sending the original input would ask the same question
	// again as though nothing had been learnt.
	st.NextInput = ""
	return st, true
}

// stepPlannerTool runs exactly one tool call the planner asked for.
//
// A tool that fails is not a transition failure: CallTool appends a tool_response
// carrying the error either way, so the planner gets to react to the failure on
// its next call. A refusal from the budget is the one error that must not be
// swallowed — continuing past it would spend beyond a ceiling its owner declared
// closed.
func (s *strategy[T]) stepPlannerTool(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	if len(st.PendingCalls) == 0 {
		st.Phase = st.AfterTool
		return st, agentkit.Continue[Output[T]](), nil
	}

	call := st.PendingCalls[0]
	st.PendingCalls = st.PendingCalls[1:]
	if call != nil {
		if _, err := sys.Session().CallTool(ctx, *call); err != nil {
			if errors.Is(err, agentkit.ErrLimitExceeded) {
				return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err,
					"planexec: planner tool call refused by the budget",
					goerr.V("tool", call.Name))
			}
		}
	}

	if len(st.PendingCalls) == 0 {
		st.Phase = st.AfterTool
	}
	return st, agentkit.Continue[Output[T]](), nil
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

	res, err := sys.Session().Generate(ctx, plannerInput(st),
		agentkit.WithSystemPrompt(prompt),
		agentkit.WithSchema(schema),
		agentkit.WithRole(RolePlanner),
	)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: replanner generate")
	}

	if next, diverted := s.divertToTools(st, res, phaseReplan); diverted {
		return next, agentkit.Continue[Output[T]](), nil
	}

	rr, perr := parseReplanResult([]byte(strings.Join(res.Texts, "\n")),
		st.Input.KnownToolIDs, st.Input.AllowQuestion)
	if perr != nil {
		st = s.carryCorrection(ctx, st, perr, "planexec: replanner output rejected")
		return st, agentkit.Continue[Output[T]](), nil
	}

	st.PlannerToolRounds = 0
	switch {
	case rr.Question != nil:
		st = s.note(ctx, st, "❓ Asked a question")
		if st.Input.SuspendOnQuestion {
			return s.askAndWait(ctx, sys, st, rr.Question)
		}
		// The turn ENDS on a question rather than waiting on an await. Holding the
		// run open while a person takes minutes or hours would pin its subject and
		// block every later turn on the thread; the answer arrives as a fresh run
		// that inherits this one's history.
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

// RenderAnswers turns a human's answers into the user turn a suspended run
// continues from. The host calls it and passes the result to Kernel.Respond, so
// the encoding of an answer stays the host's business and planexec only sees a
// string.
//
// Each answer is labelled with the question it belongs to, because the planner
// sees them one round after it asked and an unlabelled list of values is
// ambiguous when several items were asked at once.
func RenderAnswers(q Question, answers []QuestionAnswer) string {
	var sb strings.Builder
	sb.WriteString("# User answers\n\n")
	byID := make(map[string]QuestionItem, len(q.Items))
	for _, it := range q.Items {
		byID[it.ID] = it
	}
	for _, ans := range answers {
		if item, ok := byID[ans.ID]; ok {
			fmt.Fprintf(&sb, "## %s — %s\n", ans.ID, item.Text)
		} else {
			fmt.Fprintf(&sb, "## %s\n(unknown question id; answer kept verbatim)\n", ans.ID)
		}
		switch {
		case ans.FreeText != "":
			fmt.Fprintf(&sb, "Answer (free_text): %s\n\n", ans.FreeText)
		case len(ans.Choices) > 0:
			fmt.Fprintf(&sb, "Answer (multi_select): %s\n\n", strings.Join(ans.Choices, ", "))
		case ans.Choice != "":
			fmt.Fprintf(&sb, "Answer (select): %s\n\n", ans.Choice)
		default:
			sb.WriteString("Answer: (none provided)\n\n")
		}
	}
	sb.WriteString("Use these answers to decide the next action.\n")
	return sb.String()
}

// askAndWait delivers the question and parks the run until it is answered.
//
// The question is handed to the host BEFORE the suspend so the host can record
// the await key next to whatever it posts — the answer arrives out of band, and
// without the key there is nothing to Respond to.
func (s *strategy[T]) askAndWait(ctx context.Context, sys agentkit.Syscalls, st state,
	q *Question,
) (state, agentkit.Decision[Output[T]], error) {
	if s.cfg.Asker == nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.New(
			"planexec: a run that waits on its question needs an asker")
	}
	// Per-round, because agentkit closes an await once it is answered: a follow-up
	// question reusing the key would be responding to a closed one.
	key := agentkit.AwaitKey(fmt.Sprintf("question:%d", st.Round))
	if err := s.cfg.Asker.Ask(ctx, sys.ProcessID(), sys.Metadata(), key, *q); err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: deliver the question")
	}

	payload, err := json.Marshal(q)
	if err != nil {
		return st, agentkit.Decision[Output[T]]{}, goerr.Wrap(err, "planexec: encode the question")
	}
	st.AnswerKey = key
	st.Phase = phaseAnswer
	return st, agentkit.Suspend[Output[T]](agentkit.Question(key, payload)), nil
}

// stepAnswer folds the human's answer into the next planning round.
//
// The answer is opaque: the host encodes whatever the person said as the user
// turn the planner should read next. planexec knows nothing about forms, options
// or Slack — only that a string came back.
func (s *strategy[T]) stepAnswer(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output[T]], error) {
	aw, ok := sys.Await(st.AnswerKey)
	if !ok {
		return st, agentkit.Decision[Output[T]]{}, goerr.New("planexec: the question await is missing",
			goerr.V("key", string(st.AnswerKey)))
	}
	if aw.Status != agentkit.AwaitResponded {
		return st, agentkit.Decision[Output[T]]{}, goerr.New("planexec: the question await is not answered",
			goerr.V("key", string(st.AnswerKey)), goerr.V("status", string(aw.Status)))
	}

	answer := strings.TrimSpace(string(aw.Response))
	if answer == "" {
		// An empty answer is not a reason to fail: the person may have submitted the
		// form with nothing filled in, and the planner can ask again or proceed.
		answer = "The user submitted the form without providing an answer."
	}
	st = s.note(ctx, st, "💬 Received the answer")
	st.AnswerKey = ""
	st.NextInput = answer
	st.Round++
	st.Phase = phaseReplan
	return st, agentkit.Continue[Output[T]](), nil
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
		derr = s.runFinalizers(ctx, sys.Metadata(), out)
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

// runFinalizers applies the host's validators to a decoded terminal output. The
// first rejection wins: a later finalizer would be judging an output that is
// already going to be regenerated.
func (s *strategy[T]) runFinalizers(ctx context.Context, meta map[string]string, out *T) error {
	for _, fin := range s.cfg.Finalizers {
		if fin == nil {
			continue
		}
		if err := fin(ctx, meta, out); err != nil {
			return goerr.Wrap(err, "final output rejected by finalizer")
		}
	}
	return nil
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
