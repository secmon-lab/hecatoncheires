package kernel

import (
	"maps"
	"slices"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// The providers a model definition may name. They are the providers
// config.LLM knows how to build a client for; anything else is a typo in the
// configuration document rather than a model this build can reach.
const (
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
)

// ModelDef is one model an operator declared usable, with what it costs.
//
// It is the resolved form of a global config [[llm_model]] entry: the reference
// name has already been decided (the alias, or the model name when the entry
// declares no alias) and the prices have already been converted out of the
// dollars-per-million-tokens the operator writes.
type ModelDef struct {
	// Ref is the name a Job or the CLI names this model by. Unique across every
	// definition the deployment loaded.
	Ref string
	// Provider is which client builds it: openai, claude or gemini.
	Provider string
	// Model is the model name handed to that provider.
	Model string
	// Rate prices one token of each kind.
	Rate pricing.Rate
}

// Validate enforces what a definition cannot be used without.
func (d ModelDef) Validate() error {
	if d.Ref == "" {
		return goerr.New("model reference name is required")
	}
	if d.Model == "" {
		return goerr.New("model name is required", goerr.V("ref", d.Ref))
	}
	switch d.Provider {
	case ProviderOpenAI, ProviderClaude, ProviderGemini:
	default:
		return goerr.New("model provider must be openai, claude or gemini",
			goerr.V("ref", d.Ref), goerr.V("provider", d.Provider))
	}
	if !d.Rate.IsPriced() {
		return goerr.New("a model priced at nothing would make its budget unbounded",
			goerr.V("ref", d.Ref),
			goerr.V("input", int64(d.Rate.Input)), goerr.V("output", int64(d.Rate.Output)))
	}
	if d.Rate.CacheRead < 0 || d.Rate.CacheWrite < 0 {
		return goerr.New("a model has a negative cache price",
			goerr.V("ref", d.Ref),
			goerr.V("cache_read", int64(d.Rate.CacheRead)),
			goerr.V("cache_write", int64(d.Rate.CacheWrite)))
	}
	return nil
}

// ModelPolicyInput is everything a ModelPolicy is built from.
type ModelPolicyInput struct {
	// Defs are every model the deployment declared. Reference names must be
	// unique, and every definition must be priced.
	Defs []ModelDef
	// DefaultRef is the reference name of the model a run that names none
	// generates through. Required, and must be one of Defs.
	DefaultRef string
	// Clients maps a reference name to the client that serves it. The default
	// reference needs no entry: it is the Kernel's positional client, which an
	// unbound role already resolves to.
	Clients map[string]gollem.LLMClient
	// DefaultBudget is what a run that names no budget of its own may spend.
	// Required and positive: a zero ceiling stops every run (see
	// budget.Root.Limiter), so it must be caught here rather than at the first
	// transition.
	DefaultBudget pricing.NanoUSD
}

// Validate enforces the required-field contract so a configuration mistake fails
// at startup rather than at the first run.
func (in ModelPolicyInput) Validate() error {
	if len(in.Defs) == 0 {
		return goerr.New("at least one model definition is required")
	}
	seen := make(map[string]struct{}, len(in.Defs))
	for _, d := range in.Defs {
		if err := d.Validate(); err != nil {
			return goerr.Wrap(err, "invalid model definition")
		}
		if _, dup := seen[d.Ref]; dup {
			return goerr.New("duplicate model reference name", goerr.V("ref", d.Ref))
		}
		seen[d.Ref] = struct{}{}
	}
	if in.DefaultRef == "" {
		return goerr.New("a default model reference name is required")
	}
	if _, ok := seen[in.DefaultRef]; !ok {
		return goerr.New("the default model is not defined",
			goerr.V("ref", in.DefaultRef), goerr.V("known", slices.Sorted(maps.Keys(seen))))
	}
	for ref, client := range in.Clients {
		if _, ok := seen[ref]; !ok {
			return goerr.New("a client was supplied for an undefined model",
				goerr.V("ref", ref), goerr.V("known", slices.Sorted(maps.Keys(seen))))
		}
		if client == nil {
			return goerr.New("a model has a nil client", goerr.V("ref", ref))
		}
	}
	if in.DefaultBudget <= 0 {
		return goerr.New("the default budget must be positive",
			goerr.V("default_budget", int64(in.DefaultBudget)))
	}
	return nil
}

// ModelPolicy answers, for one run, which model it generates through and what it
// is judged against.
//
// Both halves live in one value because they must agree: resolving the model
// from one place and the price from another is how a run ends up generating with
// a cheap model while being metered at an expensive one's rate, or the reverse.
// Everything here is decided at startup and never changes afterwards, which is
// also what makes it safe to read on the transition hot path.
type ModelPolicy struct {
	defs          map[string]ModelDef
	roles         map[string]agentkit.ModelRole
	clients       map[string]gollem.LLMClient
	defaultRef    string
	defaultBudget pricing.NanoUSD
}

// NewModelPolicy builds the policy. One model role is defined per supplied
// client; the default model gets none, because an unbound role already resolves
// to the Kernel's positional client and defining one would only add a second
// name for it.
func NewModelPolicy(in ModelPolicyInput) (ModelPolicy, error) {
	if err := in.Validate(); err != nil {
		return ModelPolicy{}, goerr.Wrap(err, "invalid model policy")
	}
	p := ModelPolicy{
		defs:          make(map[string]ModelDef, len(in.Defs)),
		roles:         make(map[string]agentkit.ModelRole, len(in.Clients)),
		clients:       make(map[string]gollem.LLMClient, len(in.Clients)),
		defaultRef:    in.DefaultRef,
		defaultBudget: in.DefaultBudget,
	}
	for _, d := range in.Defs {
		p.defs[d.Ref] = d
	}
	for _, ref := range slices.Sorted(maps.Keys(in.Clients)) {
		if ref == in.DefaultRef {
			continue
		}
		p.roles[ref] = agentkit.DefineModelRole("hecatoncheires.model." + ref)
		p.clients[ref] = in.Clients[ref]
	}
	return p, nil
}

// IsZero reports whether this is the empty policy, which a deployment with no
// LLM configured carries.
func (p ModelPolicy) IsZero() bool { return len(p.defs) == 0 }

// Refs lists every defined reference name, sorted. For error messages and tests.
func (p ModelPolicy) Refs() []string { return slices.Sorted(maps.Keys(p.defs)) }

// Resolve returns what this run is judged against: its budget and the price of
// the model it generates through.
func (p ModelPolicy) Resolve(proc *agentkit.Process) budget.RunLimit {
	def, sc := p.resolve(proc)
	limit := budget.RunLimit{Budget: p.defaultBudget, Rate: def.Rate}
	if sc.Budget > 0 {
		limit.Budget = sc.Budget
	}
	return limit
}

// CostOf prices what a finished run actually spent, at the rate of the model it
// generated through. It is what the run record stores, so a later edit to the
// configured price cannot rewrite history.
func (p ModelPolicy) CostOf(proc *agentkit.Process) pricing.NanoUSD {
	if proc == nil {
		return 0
	}
	def, _ := p.resolve(proc)
	m := proc.Metrics
	return def.Rate.Cost(m.InputTokens, m.OutputTokens,
		m.CacheReadInputTokens, m.CacheCreationInputTokens)
}

// ModelOf names the model a run generated through — the provider's own model
// name, not the reference name, because that is the value an operator can match
// against a provider's billing.
func (p ModelPolicy) ModelOf(proc *agentkit.Process) string {
	def, _ := p.resolve(proc)
	return def.Model
}

// resolve reads the run's model definition off its metadata.
//
// An empty or UNKNOWN reference name falls back to the default definition, and
// that is deliberate: the role lookup falls back the same way, so a run whose
// model was removed from the configuration generates with the default client and
// is priced at the default rate. Pricing it at the removed model's rate would
// meter it for a model it is not using.
func (p ModelPolicy) resolve(proc *agentkit.Process) (ModelDef, Scope) {
	var sc Scope
	if proc != nil {
		sc = ScopeFrom(proc.Metadata)
	}
	if def, ok := p.defs[sc.LLMModel]; ok {
		return def, sc
	}
	return p.defs[p.defaultRef], sc
}

// kernelOptions binds each non-default model to its role. Build passes them to
// agentkit.New; nothing else may, because a role bound after the Kernel exists
// would never be resolved.
func (p ModelPolicy) kernelOptions() []agentkit.KernelOption {
	opts := make([]agentkit.KernelOption, 0, len(p.roles))
	for _, ref := range slices.Sorted(maps.Keys(p.roles)) {
		opts = append(opts, agentkit.WithModelRole(p.roles[ref], p.clients[ref]))
	}
	return opts
}

// roleFor returns the model role a run's metadata selects, or nil when the run
// generates through the default model.
func (p ModelPolicy) roleFor(meta map[string]string) agentkit.ModelRole {
	return p.roles[ScopeFrom(meta).LLMModel]
}
