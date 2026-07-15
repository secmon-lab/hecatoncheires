package agent

import (
	"slices"

	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
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
	ToolSetJira     = "jira"
	// ToolSetCaseStatusWrite is the writer toolset carrying ONLY the case
	// status-change tool (case__update_case_status / case__close_case). It is
	// the one write capability a thread-mode sub-agent is granted: closing /
	// transitioning the case it investigates. Content materialization
	// (title / description / fields) stays with the host, so case__update_case
	// is deliberately NOT part of this set.
	ToolSetCaseStatusWrite = "case_status_write"
	// ToolSetCaseMulti is the cross-case ("workspace-scoped") toolset used by the
	// workspace-channel agent. Unlike core/case_status_write (pinned to one case),
	// its tools take case_id as a call-time argument so a single turn can operate
	// across every case the requesting user can access. Advertised only via
	// KnownToolSetIDsWorkspaceChannel (never the default lists) so it is not
	// offered to the per-case mention / proposal planners.
	ToolSetCaseMulti = "case_multi"
)

// KnownToolSetIDs is the canonical list of identifiers a planner is allowed
// to request. Anything outside this list is rejected at plan validation.
var KnownToolSetIDs = []string{
	ToolSetCoreRO,
	ToolSetSlackRO,
	ToolSetNotion,
	ToolSetGitHub,
	ToolSetWebFetch,
	ToolSetJira,
}

// KnownToolSetIDsNoCore is KnownToolSetIDs without the core (action) toolset.
// Thread-mode agents advertise this list to the planner: a thread-mode
// workspace manages no Actions, so the planner must never be offered the
// core read tools (list/get action). Paired with ToolSetDeps.OmitCore so the
// resolver also withholds the underlying tools.
var KnownToolSetIDsNoCore = []string{
	ToolSetSlackRO,
	ToolSetNotion,
	ToolSetGitHub,
	ToolSetWebFetch,
	ToolSetJira,
}

// KnownToolSetIDsThreadWrite is KnownToolSetIDsNoCore plus the case
// status-change writer toolset. Thread-mode agents advertise this to the
// planner ONLY when a concrete case exists to act on (mention / materialize
// turns): the sub-agent may then close / transition that case via
// case__update_case_status. Creation turns (no case yet) advertise the plain
// KnownToolSetIDsNoCore instead, so the planner is never offered a writer tool
// the resolver cannot wire — the prompt-vs-capability mismatch the architecture
// rule forbids.
var KnownToolSetIDsThreadWrite = append(append([]string{}, KnownToolSetIDsNoCore...), ToolSetCaseStatusWrite)

// KnownToolSetIDsWorkspaceChannel is the planner-advertised list for the
// workspace-channel agent: the cross-case toolset plus the read-only auxiliary
// toolsets. It deliberately omits core_ro (the case-pinned action read tools) —
// case_multi carries the cross-case action tools instead.
var KnownToolSetIDsWorkspaceChannel = []string{
	ToolSetCaseMulti,
	ToolSetSlackRO,
	ToolSetNotion,
	ToolSetGitHub,
	ToolSetWebFetch,
	ToolSetJira,
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
	// jira is the already-expanded Jira read tool set (see
	// pkg/agent/tool/jira). Unlike notion/github/webfetch this is not built
	// from a client here: it is handed in pre-expanded via ToolSetDeps.Jira
	// because gollem has no exported ToolSet-to-[]Tool helper.
	jira []gollem.Tool
	// caseStatus is the case status-change writer tool set (case_status_write).
	// Unlike knowledge it is NOT always included: a sub-agent gets it only when
	// the planner requested ToolSetCaseStatusWrite for that task. Empty unless
	// ToolSetDeps.CaseStatus.StatusSet is set (a thread-mode case with a board
	// status set).
	caseStatus []gollem.Tool
	// knowledge is the read-only workspace knowledge tool set. It is always
	// included in every Resolve result (not gated by a planner-requested ID):
	// investigation sub-agents may always consult shared knowledge, but never
	// mutate it (write tools are wired only in the case-bound / job paths).
	knowledge []gollem.Tool
	// caseMulti is the cross-case ("workspace-scoped") tool set (case_multi).
	// Unlike caseStatus it carries full cross-case read+write tools taking
	// case_id at call time. Empty unless ToolSetDeps.CaseMulti.CaseUC is set;
	// gated on the planner requesting ToolSetCaseMulti. Used by the
	// workspace-channel agent, never the per-case mention / proposal planners.
	caseMulti []gollem.Tool
}

// ToolSetDeps carries the per-turn deps that flavor each toolset's binding.
// Optional fields (SlackSearch / NotionClient / GitHubClient) may be nil; the
// corresponding toolset is empty in that case.
type ToolSetDeps struct {
	Core      core.Deps
	Slack     slacktool.Deps
	Notion    notiontool.Deps
	GitHub    *githubtool.Client
	WebFetch  *webfetch.Client
	Knowledge knowledgetool.Deps

	// Jira carries the already-expanded Jira read tools (see
	// pkg/agent/tool/jira). nil/empty means Jira is not configured, so the
	// "jira" ToolSet ID resolves to nothing.
	Jira []gollem.Tool

	// CaseStatus backs the case_status_write toolset (the status-change tool
	// only). The status tool is built when CaseStatus.StatusSet is non-nil
	// (a thread-mode case carrying a board status set); a nil StatusSet leaves
	// the toolset empty so requesting the ID resolves to nothing. CaseUC /
	// WorkspaceID / CaseID identify the case the sub-agent may transition.
	CaseStatus casewriter.Deps

	// OmitCore omits the core (action) toolset entirely. Set by thread-mode
	// agents: a thread-mode workspace manages no Actions, so even the
	// read-only list/get-action tools must not exist. Without this the
	// resolver would always build them (they only need Repo), since the
	// core read tools do not depend on ActionUC being wired.
	OmitCore bool

	// CaseMulti backs the case_multi (cross-case) toolset. Built only when
	// CaseMulti.CaseUC is non-nil (the workspace-channel host wires it); a nil
	// CaseUC leaves the toolset empty so requesting the ID resolves to nothing.
	CaseMulti casemulti.Deps
}

// NewToolSetResolver builds the per-toolset slices once so each sub-agent
// just picks the union of its requested IDs. The "core" pool is the read-only
// subset (list / get only) — investigation sub-agents must not mutate the
// case while a turn is forming.
func NewToolSetResolver(d ToolSetDeps) *ToolSetResolver {
	var coreTools []gollem.Tool
	if !d.OmitCore {
		coreTools = core.NewReadOnly(d.Core)
	}
	var knowledge []gollem.Tool
	if d.Knowledge.Accessor != nil {
		knowledge = knowledgetool.NewReadOnly(d.Knowledge)
	}
	// The status-change tool needs both a mutator and a board status set; a nil
	// StatusSet (non-thread-mode, or a create turn with no case yet) leaves the
	// set empty so a stray case_status_write request resolves to nothing.
	var caseStatus []gollem.Tool
	if d.CaseStatus.StatusSet != nil && d.CaseStatus.CaseUC != nil {
		caseStatus = casewriter.NewStatusTool(d.CaseStatus)
	}
	// The cross-case toolset is built only when a CaseUsecase is wired (the
	// workspace-channel host); casemulti.New returns nil otherwise.
	var caseMulti []gollem.Tool
	if d.CaseMulti.CaseUC != nil {
		caseMulti = casemulti.New(d.CaseMulti)
	}
	return &ToolSetResolver{
		core:       coreTools,
		slack:      slacktool.NewReadOnly(d.Slack),
		notion:     notiontool.New(d.Notion),
		github:     githubtool.New(d.GitHub),
		webfetch:   webfetch.New(d.WebFetch),
		jira:       d.Jira,
		caseStatus: caseStatus,
		knowledge:  knowledge,
		caseMulti:  caseMulti,
	}
}

// Resolve returns the concatenated tool list for the requested IDs. Unknown
// IDs are skipped (they should already have been rejected by plan validation,
// but Resolve never panics so a stray ID does not crash a turn).
func (r *ToolSetResolver) Resolve(ids []string) []gollem.Tool {
	if r == nil {
		return nil
	}
	// Knowledge read tools are always available to every sub-agent, regardless
	// of which toolset IDs the planner requested.
	if len(ids) == 0 {
		if len(r.knowledge) == 0 {
			return nil
		}
		out := make([]gollem.Tool, len(r.knowledge))
		copy(out, r.knowledge)
		return out
	}
	// Pre-compute capacity to avoid repeated growth.
	total := len(r.knowledge)
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
		case ToolSetJira:
			total += len(r.jira)
		case ToolSetCaseStatusWrite:
			total += len(r.caseStatus)
		case ToolSetCaseMulti:
			total += len(r.caseMulti)
		}
	}
	out := make([]gollem.Tool, 0, total)
	out = append(out, r.knowledge...)
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
		case ToolSetJira:
			out = append(out, r.jira...)
		case ToolSetCaseStatusWrite:
			out = append(out, r.caseStatus...)
		case ToolSetCaseMulti:
			out = append(out, r.caseMulti...)
		}
	}
	return out
}
