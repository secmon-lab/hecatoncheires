// Package jira adapts a gollem.ToolSet (e.g. github.com/gollem-dev/tools/jira)
// into individual gollem.Tool values so it can be appended to hecatoncheires's
// internal []gollem.Tool aggregates ([agent.ToolSetResolver], buildJobTools,
// casebound/assist allTools). gollem itself has no exported helper for this:
// the expansion logic exists only as the unexported buildToolMap/toolWrapper
// pair inside gollem.New, reachable only through gollem.WithToolSets when
// constructing a full gollem.Agent.
package jira

import (
	"context"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
)

// New expands ts into individual gollem.Tool wrappers by querying its specs
// once. Returns nil, nil when ts is nil (nil-safe, mirrors github.New).
func New(ctx context.Context, ts gollem.ToolSet) ([]gollem.Tool, error) {
	if ts == nil {
		return nil, nil
	}
	specs, err := ts.Specs(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "list jira toolset specs")
	}
	tools := make([]gollem.Tool, 0, len(specs))
	for _, spec := range specs {
		tools = append(tools, &toolSetTool{spec: spec, ts: ts})
	}
	return tools, nil
}

// toolSetTool wraps a single ToolSpec from a gollem.ToolSet, delegating Run
// back to the ToolSet by name.
type toolSetTool struct {
	spec gollem.ToolSpec
	ts   gollem.ToolSet
}

func (t *toolSetTool) Spec() gollem.ToolSpec {
	return t.spec
}

func (t *toolSetTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	out, err := t.ts.Run(ctx, t.spec.Name, args)
	if err != nil {
		return nil, goerr.Wrap(err, "jira tool run failed", goerr.V("tool", t.spec.Name))
	}
	return out, nil
}
