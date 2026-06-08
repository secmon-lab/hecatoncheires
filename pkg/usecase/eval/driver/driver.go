// Package driver runs a workflow type for one scenario against a prepared env
// and returns the artifact to judge. New workflow types plug in by registering
// a WorkflowDriver; thread_mode_initial is the first.
package driver

import (
	"context"
	"slices"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

// WorkflowDriver runs one workflow type for a scenario against a prepared env
// and returns the produced artifact. It may call sim.Answer when the agent
// asks the user a question.
type WorkflowDriver interface {
	Kind() string
	Run(ctx context.Context, e *env.Env, sc *scenario.Scenario, sim evaltype.Simulator) (evaltype.Artifact, error)
}

// Registry maps workflow kinds to drivers.
type Registry struct {
	drivers map[string]WorkflowDriver
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{drivers: make(map[string]WorkflowDriver)}
}

// Register adds a driver. Panics on duplicate kind (a programming error).
func (r *Registry) Register(d WorkflowDriver) {
	if _, ok := r.drivers[d.Kind()]; ok {
		panic("duplicate workflow driver kind: " + d.Kind())
	}
	r.drivers[d.Kind()] = d
}

// Lookup returns the driver for a kind.
func (r *Registry) Lookup(kind string) (WorkflowDriver, bool) {
	d, ok := r.drivers[kind]
	return d, ok
}

// Kinds returns the registered kinds (sorted for stable output).
func (r *Registry) Kinds() []string {
	out := make([]string, 0, len(r.drivers))
	for k := range r.drivers {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Default returns a registry with all built-in drivers registered.
func Default() *Registry {
	r := NewRegistry()
	r.Register(NewThreadInitial())
	r.Register(NewJobExecution())
	return r
}

// errNoCase is returned when the workflow produced no case to judge.
var errNoCase = goerr.New("workflow produced no case")
