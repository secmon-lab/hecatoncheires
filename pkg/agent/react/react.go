// Package react is the tool-calling agent loop, written as an agentkit
// Strategy: generate, run the tool calls the model asked for, feed the results
// back, repeat until it answers without asking for a tool.
//
// It splits that loop finer than a conventional implementation would: one
// transition is either ONE LLM call or ONE tool call, never both and never
// several. Every transition is checkpointed before the next starts, so a crash
// costs at most one call, and a run can move between instances at any point.
//
// The bundled agentkit strategy/simple is deliberately not used. It fixes the
// system prompt at registration time (this application's prompts are built per
// case), and it runs every pending tool call inside one transition (which is
// the opposite of the split above).
//
// It lives under pkg/agent rather than pkg/usecase because it is runtime
// machinery the kernel drives: it holds no business rule, reaches no
// repository, and its method set is fixed by agentkit.Strategy — Init, Step,
// EncodeState and the rest take what the library passes them, not a request
// context.
package react

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/toolcall"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Phases. A transition reads the phase, does one thing, and writes the phase it
// leaves behind.
const (
	// phaseGenerate asks the model what to do next.
	phaseGenerate = "generate"
	// phaseTool runs one tool call the model asked for.
	phaseTool = "tool"
)

// Input is the launch input. Both fields are required: this strategy knows
// nothing about cases or workspaces, so the host supplies the finished prompts.
type Input struct {
	// SystemPrompt is the agent's persistent context.
	SystemPrompt string `json:"system_prompt"`
	// Prompt is the first user message.
	Prompt string `json:"prompt"`
}

// Output is what a finished run produces.
type Output struct {
	// Texts is every text block the model emitted across the run, in order.
	Texts []string `json:"texts"`
}

// Text joins the emitted blocks into the single reply a host posts.
func (o Output) Text() string { return strings.Join(o.Texts, "\n") }

// state is the checkpointed state. Everything the next transition needs must be
// here: Step runs from the top every time, including after a crash.
type state struct {
	Phase        string `json:"phase"`
	SystemPrompt string `json:"system_prompt"`
	// Prompt is cleared once it has been sent. A later Generate continues from
	// the conversation instead, which is what follows a tool result.
	Prompt string `json:"prompt,omitempty"`
	// Pending are the tool calls the model asked for and this run has not made
	// yet. They are consumed one per transition.
	Pending []*gollem.FunctionCall `json:"pending,omitempty"`
	// ToolResponses are the results of the drained Pending calls, held until the
	// next Generate reports them as ONE turn.
	ToolResponses []toolcall.Response `json:"tool_responses,omitempty"`
	Texts         []string            `json:"texts,omitempty"`
}

// Option configures the strategy.
type Option func(*config)

type config struct {
	role agentkit.ModelRole
}

// WithRole selects the model role every Generate runs under. Omitting it uses
// the Kernel's default model.
func WithRole(r agentkit.ModelRole) Option {
	return func(c *config) { c.role = r }
}

// Register registers the strategy under name and returns the typed handle.
//
// limiter is the budget this agent answers Limit with; it is required, because
// a run with no ceiling is a run that can spend without bound.
func Register(reg *agentkit.Registry, name agentkit.AgentName, version int,
	limiter agentkit.Limiter, opts ...agentkit.RegisterOption[Output], // nolint:revive // options mirror agentkit.Register
) (agentkit.Agent[Input], error) {
	return RegisterWithOptions(reg, name, version, limiter, nil, opts...)
}

// RegisterWithOptions is Register plus strategy options.
func RegisterWithOptions(reg *agentkit.Registry, name agentkit.AgentName, version int,
	limiter agentkit.Limiter, strategyOpts []Option, opts ...agentkit.RegisterOption[Output],
) (agentkit.Agent[Input], error) {
	if limiter == nil {
		return agentkit.Agent[Input]{}, goerr.Wrap(agentkit.ErrInvalidAgentDef,
			"react: a limiter is required", goerr.V("agent", name))
	}
	var cfg config
	for _, o := range strategyOpts {
		o(&cfg)
	}
	return agentkit.Register(reg, name, version, &strategy{limiter: limiter, role: cfg.role, version: version}, opts...)
}

type strategy struct {
	limiter agentkit.Limiter
	role    agentkit.ModelRole
	version int
}

func (s *strategy) Version() int { return s.version }

func (s *strategy) Limit(ctx context.Context, proc *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
	return s.limiter(ctx, proc, m)
}

func (s *strategy) Init(in Input) (state, error) {
	if in.SystemPrompt == "" {
		return state{}, goerr.New("react: system prompt is required")
	}
	if in.Prompt == "" {
		return state{}, goerr.New("react: prompt is required")
	}
	return state{Phase: phaseGenerate, SystemPrompt: in.SystemPrompt, Prompt: in.Prompt}, nil
}

func (s *strategy) Step(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output], error) {
	switch st.Phase {
	case phaseGenerate:
		return s.stepGenerate(ctx, sys, st)
	case phaseTool:
		return s.stepTool(ctx, sys, st)
	default:
		return st, agentkit.Decision[Output]{}, goerr.New("react: unknown phase", goerr.V("phase", st.Phase))
	}
}

// stepGenerate makes the one LLM call this transition is allowed.
func (s *strategy) stepGenerate(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output], error) {
	// Pending tool results go in this one call, and go ALONE — see package toolcall
	// for why they cannot be reported one at a time, and why a turn holding them
	// may hold nothing else.
	//
	// With no results and an empty prompt it still sends a turn: gollem appends no
	// user content for an empty input, so the request would END on the previous
	// model turn and the provider rejects that outright.
	input := toolcall.Inputs(st.ToolResponses)
	if len(input) == 0 {
		text := st.Prompt
		if text == "" {
			text = continueInstruction
		}
		input = []gollem.Input{gollem.Text(text)}
	}

	// A budget that is close to its ceiling has to be TOLD to the model, not
	// merely enforced against it. Enforcement alone produces the worst outcome:
	// the run is cut off mid-investigation with no answer at all. Given the
	// notice, the model can spend its last call on a conclusion instead of
	// another tool.
	//
	// It rides in the system prompt for this call, not as an input: the crossing
	// happens mid-run, exactly when a turn is likely to be reporting tool results,
	// and such a turn may carry nothing else.
	systemPrompt := st.SystemPrompt
	if notice := sys.LimitStatus(); notice.Kind() == agentkit.LimitKindNotice {
		systemPrompt += "\n\n" + noticeInstruction(notice.Message())
	}

	opts := []agentkit.GenerateOption{agentkit.WithSystemPrompt(systemPrompt)}
	if s.role != nil {
		opts = append(opts, agentkit.WithRole(s.role))
	}

	res, err := sys.Session().Generate(ctx, input, opts...)
	if err != nil {
		return st, agentkit.Decision[Output]{}, goerr.Wrap(err, "react: generate")
	}
	// The call carried them, so they are now in the conversation. Cleared only
	// after it succeeded: a failed transition is retried from the checkpoint, and
	// dropping them earlier would leave its function calls unanswered forever.
	st.ToolResponses = nil

	st.Prompt = ""
	st.Texts = append(st.Texts, res.Texts...)

	if len(res.FunctionCalls) == 0 {
		return st, agentkit.Done(Output{Texts: st.Texts}), nil
	}

	st.Pending = res.FunctionCalls
	st.Phase = phaseTool
	return st, agentkit.Continue[Output](), nil
}

// continueInstruction is the user turn a call sends when it has nothing new to
// say. It must not restate the request: everything the model needs is already in
// the conversation, and repeating it invites the same answer again.
const continueInstruction = "Continue from what you have so far."

// noticeInstruction turns a budget notice into an instruction the model can act
// on. The limiter's message says what is nearly spent; this says what to do
// about it.
func noticeInstruction(msg string) string {
	if msg == "" {
		return "This run is close to its budget. Answer now from what you already have, and do not call any more tools."
	}
	return msg + "\nThis run is close to its budget. Answer now from what you already have, and do not call any more tools."
}

// stepTool runs exactly one pending tool call and holds the result on the state.
// The results are reported together by the next Generate — see package toolcall
// for why they cannot be reported one at a time, and why this uses the primitive
// CallTool rather than the session's.
//
// A tool that fails is not a transition failure: the failure is recorded as this
// call's response, so the call is still answered and the model gets to react to
// it on its next turn — which is how the previous runtime behaved too. The one
// error that must not be swallowed is a refusal from the budget: continuing past
// it would spend beyond the ceiling.
//
// That refusal check is DEFENSIVE rather than a path budget.Config can take.
// agentkit evaluates Limit against "committed metrics plus whatever this
// transition has already spent", and this transition spends nothing before the
// call — so a metrics-based limiter that let the transition begin also lets the
// call through, and the run is stopped at the next transition boundary instead.
// The check is kept because Limit is an arbitrary function: a stateful or
// time-based one can refuse here, and swallowing that would spend past a ceiling
// its owner had already declared closed. Do not remove it on the grounds that no
// test drives it; no test can, with the limiter this application ships.
func (s *strategy) stepTool(ctx context.Context, sys agentkit.Syscalls, st state) (state, agentkit.Decision[Output], error) {
	if len(st.Pending) == 0 {
		st.Phase = phaseGenerate
		return st, agentkit.Continue[Output](), nil
	}

	call := st.Pending[0]
	st.Pending = st.Pending[1:]
	if call == nil {
		return st, agentkit.Continue[Output](), nil
	}

	out, err := sys.CallTool(ctx, *call)
	if err != nil {
		if errors.Is(err, agentkit.ErrLimitExceeded) {
			return st, agentkit.Decision[Output]{}, goerr.Wrap(err, "react: tool call refused by the budget",
				goerr.V("tool", call.Name))
		}
		// Reported as well as fed back. The model needs the failure to react to;
		// an operator needs it to tell a broken tool from a model that chose not
		// to use its result, and the run timeline that also records it is only
		// written for a run that keeps a run record.
		errutil.Handle(ctx, goerr.Wrap(err, "react: tool call",
			goerr.V("tool", call.Name), goerr.V("call_id", call.ID)), "react: tool call")
	}
	st.ToolResponses = append(st.ToolResponses, toolcall.New(*call, out, err))

	if len(st.Pending) == 0 {
		st.Phase = phaseGenerate
	}
	return st, agentkit.Continue[Output](), nil
}

func (s *strategy) EncodeState(st state) ([]byte, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, goerr.Wrap(err, "react: encode state")
	}
	return raw, nil
}

func (s *strategy) DecodeState(_ int, raw []byte) (state, error) {
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return state{}, goerr.Wrap(err, "react: decode state")
	}
	return st, nil
}

func (s *strategy) EncodeOutput(out Output) ([]byte, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, goerr.Wrap(err, "react: encode output")
	}
	return raw, nil
}

// DecodeOutput reads back the bytes a finished Process stored. agentkit hands a
// parent only its child's raw output, so a caller collecting sub-agent results
// needs this; the kernel itself never calls it.
func DecodeOutput(raw []byte) (Output, error) {
	var out Output
	if err := json.Unmarshal(raw, &out); err != nil {
		return Output{}, goerr.Wrap(err, "react: decode output")
	}
	return out, nil
}
