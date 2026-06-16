package agent

import (
	"slices"

	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
)

// ToolSet IDs known to the planner. Sub-agents request a subset of these
// per investigation task and the resolver below maps each ID to a concrete
// []gollem.Tool slice.
const (
	ToolSetCoreRO   = "core_ro"
	ToolSetSlackRO  = "slack_ro"
	ToolSetNotion   = "notion"
	ToolSetGitHub   = "github"
	ToolSetWebFetch = "webfetch"
)

// KnownToolSetIDs is the canonical list of identifiers a planner is allowed
// to request. Anything outside this list is rejected at plan validation.
var KnownToolSetIDs = []string{
	ToolSetCoreRO,
	ToolSetSlackRO,
	ToolSetNotion,
	ToolSetGitHub,
	ToolSetWebFetch,
}

// IsKnownToolSetID reports whether id is a member of KnownToolSetIDs.
func IsKnownToolSetID(id string) bool {
	return slices.Contains(KnownToolSetIDs, id)
}

// ToolSetResolver builds gollem.Tool slices for sub-agents based on a list
// of ToolSet IDs. The resolver is created once per turn (with the deps that
// vary per turn — workspace, case, slack/notion/github clients) and called
// per sub-agent.
type ToolSetResolver struct {
	core     []gollem.Tool
	slack    []gollem.Tool
	notion   []gollem.Tool
	github   []gollem.Tool
	webfetch []gollem.Tool
}

// ToolSetDeps carries the per-turn deps that flavor each toolset's binding.
// Optional fields (SlackSearch / NotionClient / GitHubClient) may be nil; the
// corresponding toolset is empty in that case.
type ToolSetDeps struct {
	Core     core.Deps
	Slack    slacktool.Deps
	Notion   notiontool.Deps
	GitHub   *githubtool.Client
	WebFetch *webfetch.Client
}

// NewToolSetResolver builds the per-toolset slices once so each sub-agent
// just picks the union of its requested IDs. The "core" pool is the read-only
// subset (list / get only) — investigation sub-agents must not mutate the
// case while a turn is forming.
func NewToolSetResolver(d ToolSetDeps) *ToolSetResolver {
	return &ToolSetResolver{
		core:     core.NewReadOnly(d.Core),
		slack:    slacktool.NewReadOnly(d.Slack),
		notion:   notiontool.New(d.Notion),
		github:   githubtool.New(d.GitHub),
		webfetch: webfetch.New(d.WebFetch),
	}
}

// Resolve returns the concatenated tool list for the requested IDs. Unknown
// IDs are skipped (they should already have been rejected by plan validation,
// but Resolve never panics so a stray ID does not crash a turn).
func (r *ToolSetResolver) Resolve(ids []string) []gollem.Tool {
	if r == nil || len(ids) == 0 {
		return nil
	}
	// Pre-compute capacity to avoid repeated growth.
	total := 0
	for _, id := range ids {
		switch id {
		case ToolSetCoreRO:
			total += len(r.core)
		case ToolSetSlackRO:
			total += len(r.slack)
		case ToolSetNotion:
			total += len(r.notion)
		case ToolSetGitHub:
			total += len(r.github)
		case ToolSetWebFetch:
			total += len(r.webfetch)
		}
	}
	out := make([]gollem.Tool, 0, total)
	for _, id := range ids {
		switch id {
		case ToolSetCoreRO:
			out = append(out, r.core...)
		case ToolSetSlackRO:
			out = append(out, r.slack...)
		case ToolSetNotion:
			out = append(out, r.notion...)
		case ToolSetGitHub:
			out = append(out, r.github...)
		case ToolSetWebFetch:
			out = append(out, r.webfetch...)
		}
	}
	return out
}
