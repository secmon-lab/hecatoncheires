package job

import (
	"context"
	_ "embed"
	"strings"
	"text/template"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
)

// reflectionSystemPrompt is the system prompt that drives the reflection agent.
// It is embedded so the prompt lives as readable Markdown next to the code. It
// is static (no dynamic injection), so it is used verbatim.
//
//go:embed prompts/reflection.md
var reflectionSystemPrompt string

// reflectionInstructionText is the user message appended after the carried-over
// history. It is a text/template because it injects the Job name / description.
//
//go:embed prompts/reflection_instruction.md
var reflectionInstructionText string

// reflectionInstructionTmpl is parsed once at init per the prompt convention
// (no per-request re-parse).
var reflectionInstructionTmpl = template.Must(
	template.New("reflection_instruction").Parse(reflectionInstructionText))

// reflectInstructionInput is the typed data passed to reflectionInstructionTmpl.
type reflectInstructionInput struct {
	JobName        string
	JobDescription string
}

// ReflectRequest is the input to a Reflector. The reflection agent continues the
// just-finished run's conversation (History) with only the knowledge / tag tools
// available, and curates the workspace's shared Knowledge.
type ReflectRequest struct {
	WorkspaceID    string
	CaseID         int64
	JobID          string
	JobName        string
	JobDescription string
	// History is the carried-over conversation of the main run. The reflection
	// agent is constructed with gollem.WithHistory(History) so it sees exactly
	// what the run did before being asked to reflect.
	History *gollem.History
	// TraceHandler, when non-nil, records the reflection agent's LLM / tool
	// events (the caller relabels them as the "reflection" phase).
	TraceHandler trace.Handler
}

// Validate enforces the inputs the reflection pass cannot run without.
func (r ReflectRequest) Validate() error {
	if r.WorkspaceID == "" {
		return goerr.New("reflect: workspace id is required")
	}
	if r.JobID == "" {
		return goerr.New("reflect: job id is required")
	}
	if r.History == nil {
		return goerr.New("reflect: history is required", goerr.V("job_id", r.JobID))
	}
	return nil
}

// Reflector reviews a finished Job run and curates workspace Knowledge.
type Reflector interface {
	Reflect(ctx context.Context, req ReflectRequest) error
}

// ReflectorDeps groups the dependencies the LLMReflector needs.
type ReflectorDeps struct {
	LLMClient         gollem.LLMClient
	KnowledgeAccessor knowledgetool.KnowledgeAccessor
	KnowledgeMutator  knowledgetool.KnowledgeMutator
	// LoopMax bounds the reflection agent's internal tool-calling loop. The
	// caller supplies it (no hidden default); a value <= 0 leaves gollem's own
	// default in effect.
	LoopMax int
}

// LLMReflector is the production Reflector: a single-loop gollem agent equipped
// only with the workspace knowledge / tag tools.
type LLMReflector struct {
	llm     gollem.LLMClient
	deps    ReflectorDeps
	loopMax int
}

// NewLLMReflector constructs a Reflector. It requires an LLM client and both
// knowledge surfaces (read + write) — the reflection pass exists to write
// knowledge, so a missing mutator is a wiring error.
func NewLLMReflector(deps ReflectorDeps) (*LLMReflector, error) {
	if deps.LLMClient == nil {
		return nil, goerr.New("reflector: llm client is required")
	}
	if deps.KnowledgeAccessor == nil {
		return nil, goerr.New("reflector: knowledge accessor is required")
	}
	if deps.KnowledgeMutator == nil {
		return nil, goerr.New("reflector: knowledge mutator is required")
	}
	return &LLMReflector{llm: deps.LLMClient, deps: deps, loopMax: deps.LoopMax}, nil
}

// Reflect runs the reflection agent over req.History. Tool calls mutate the
// workspace Knowledge / Tags through the configured surfaces. Errors are
// returned for the caller to report non-fatally.
func (r *LLMReflector) Reflect(ctx context.Context, req ReflectRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	tools := knowledgetool.New(knowledgetool.Deps{
		WorkspaceID: req.WorkspaceID,
		Accessor:    r.deps.KnowledgeAccessor,
		Mutator:     r.deps.KnowledgeMutator,
	})

	opts := []gollem.Option{
		gollem.WithSystemPrompt(reflectionSystemPrompt),
		gollem.WithHistory(req.History),
		gollem.WithTools(tools...),
		gollem.WithPromptCache(true),
	}
	if r.loopMax > 0 {
		opts = append(opts, gollem.WithLoopLimit(r.loopMax))
	}
	if req.TraceHandler != nil {
		opts = append(opts, gollem.WithTrace(req.TraceHandler))
	}

	instruction, err := renderReflectInstruction(req)
	if err != nil {
		return goerr.Wrap(err, "render reflection instruction", goerr.V("job_id", req.JobID))
	}

	agent := gollem.New(r.llm, opts...)
	if _, err := agent.Execute(ctx, gollem.Text(instruction)); err != nil {
		return goerr.Wrap(err, "reflection agent execute",
			goerr.V("workspace_id", req.WorkspaceID), goerr.V("job_id", req.JobID))
	}
	return nil
}

// renderReflectInstruction renders the user message appended after the
// carried-over history to kick off the reflection.
func renderReflectInstruction(req ReflectRequest) (string, error) {
	var sb strings.Builder
	if err := reflectionInstructionTmpl.Execute(&sb, reflectInstructionInput{
		JobName:        req.JobName,
		JobDescription: req.JobDescription,
	}); err != nil {
		return "", goerr.Wrap(err, "execute reflection instruction template")
	}
	return strings.TrimSpace(sb.String()), nil
}
