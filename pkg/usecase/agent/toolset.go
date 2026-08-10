package agent

import (
	"slices"

	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
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
	// ToolSetCore is the FULL action toolset (create / update / archive), as
	// opposed to the read-only core_ro handed to investigation sub-agents. Only
	// the channel-mode case agent asks for it: a thread-mode workspace manages
	// no Actions at all.
	ToolSetCore = "core"
	// ToolSetMemo is the Case-scoped memo toolset (memo__*). Built only when a
	// memo mutator and a memo schema are configured for the workspace.
	ToolSetMemo = "memo"
	// ToolSetKnowledge is the workspace knowledge toolset INCLUDING the write
	// tools. The read tools are always available to every agent regardless of
	// what it requested (see Resolve); this ID is what additionally grants
	// create/update, and a host withholds it while processing a private case so
	// that case's contents cannot leak into workspace-wide knowledge.
	ToolSetKnowledge = "knowledge"
	// ToolSetCaseWrite is the writer toolset for the single case the turn is
	// pinned to: the full casewriter set (case__update_case, case__assign,
	// case__unassign, and the mode-appropriate case__update_case_status /
	// case__close_case). Every mention-driven host grants it, so a sub-agent can
	// carry out any case edit the user asked for. Note that a mention turn's
	// terminal `materialize` decision replaces title / description wholesale, so
	// a host offering both must tell the planner to pick one path per turn (see
	// threadcase's system prompt).
	ToolSetCaseWrite = "case_write"
	// ToolSetCaseMulti is the cross-case ("workspace-scoped") toolset used by the
	// workspace-channel agent. Unlike core/case_write (pinned to one case),
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

// KnownToolSetIDsThreadWrite is KnownToolSetIDsNoCore plus the case writer
// toolset. Thread-mode agents advertise this to the planner on mention turns,
// where a concrete case exists to act on and a human asked for the change: the
// sub-agent may then edit, assign, and transition that case. Materialize and
// creation turns advertise the plain KnownToolSetIDsNoCore instead, so the
// planner is never offered a writer tool the resolver cannot wire — the
// prompt-vs-capability mismatch the architecture rule forbids.
var KnownToolSetIDsThreadWrite = append(append([]string{}, KnownToolSetIDsNoCore...), ToolSetCaseWrite)

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

// KnownToolSetIDsCaseChannel is the full palette of the channel-mode case
// agent: the mutating action tools, the single-case writer tools, memos,
// knowledge writes, and every read-only auxiliary set. Unlike the planner-facing
// lists above it is not a menu an LLM chooses from — the channel-mode agent runs
// a single ReAct loop and is handed the whole set at once.
var KnownToolSetIDsCaseChannel = []string{
	ToolSetCore,
	ToolSetSlackRO,
	ToolSetNotion,
	ToolSetGitHub,
	ToolSetWebFetch,
	ToolSetJira,
	ToolSetCaseWrite,
	ToolSetMemo,
	ToolSetKnowledge,
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
	core []gollem.Tool
	// coreFull is the mutating action tool set (ToolSetCore). It is separate
	// from core (read-only) rather than a superset flag because one resolver
	// can be asked for either by different tasks in the same run.
	coreFull []gollem.Tool
	// memo is the Case-scoped memo tool set (ToolSetMemo).
	memo []gollem.Tool
	// knowledgeWrite is the knowledge tool set including create/update
	// (ToolSetKnowledge). Empty unless a mutator is wired.
	knowledgeWrite []gollem.Tool
	slack          []gollem.Tool
	notion         []gollem.Tool
	github         []gollem.Tool
	webfetch       []gollem.Tool
	// jira is the already-expanded Jira read tool set (see
	// pkg/agent/tool/jira). Unlike notion/github/webfetch this is not built
	// from a client here: it is handed in pre-expanded via ToolSetDeps.Jira
	// because gollem has no exported ToolSet-to-[]Tool helper.
	jira []gollem.Tool
	// caseWrite is the single-case writer tool set (case_write). Unlike
	// knowledge it is NOT always included: a sub-agent gets it only when the
	// planner requested ToolSetCaseWrite for that task. Empty unless
	// ToolSetDeps.CaseWrite identifies a concrete case to write to.
	caseWrite []gollem.Tool
	// knowledge is the read-only workspace knowledge tool set. It is always
	// included in every Resolve result (not gated by a planner-requested ID):
	// investigation sub-agents may always consult shared knowledge, but never
	// mutate it (write tools are wired only in the case-bound / job paths).
	knowledge []gollem.Tool
	// caseMulti is the cross-case ("workspace-scoped") tool set (case_multi).
	// Unlike caseWrite it carries full cross-case read+write tools taking
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

	// CaseWrite backs the case_write toolset (the full single-case writer set).
	// The tools are built when CaseUC and CaseID identify a concrete case; a zero
	// value (no case yet, or no mutator wired) leaves the toolset empty so
	// requesting the ID resolves to nothing. StatusSet selects the mode-specific
	// "mark done" tool (case__update_case_status when set, case__close_case when
	// not) and Schema drives case__update_case's custom-field coercion.
	CaseWrite casewriter.Deps

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

	// Memo backs the memo toolset. Built only when Memo.MemoUC and a memo
	// schema are present; a zero value leaves the toolset empty so requesting
	// the ID resolves to nothing.
	Memo memotool.Deps
}

// NewToolSetResolver builds the per-toolset slices once so each sub-agent
// just picks the union of its requested IDs. The "core" pool is the read-only
// subset (list / get only) — investigation sub-agents must not mutate the
// case while a turn is forming.
func NewToolSetResolver(d ToolSetDeps) *ToolSetResolver {
	var coreTools []gollem.Tool
	var coreFullTools []gollem.Tool
	if !d.OmitCore {
		coreTools = core.NewReadOnly(d.Core)
		coreFullTools = core.New(d.Core)
	}
	var knowledge []gollem.Tool
	var knowledgeWrite []gollem.Tool
	if d.Knowledge.Accessor != nil {
		knowledge = knowledgetool.NewReadOnly(d.Knowledge)
		// The write tools need a mutator. A host that wires none (or withholds
		// it for a private case) leaves this empty, so requesting the knowledge
		// id resolves to the read tools it already had.
		if d.Knowledge.Mutator != nil {
			knowledgeWrite = knowledgetool.New(d.Knowledge)
		}
	}
	// Memo tools need both a mutator and the workspace's memo schema; the schema
	// drives field coercion in create/update.
	var memoTools []gollem.Tool
	if d.Memo.MemoUC != nil && d.Memo.Schema != nil {
		memoTools = memotool.New(d.Memo)
	}
	// The writer tools need a mutator and a concrete case to be pinned to. A
	// create turn (no case yet) or a host that wires no CaseUC leaves the set
	// empty, so a stray case_write request resolves to nothing rather than
	// producing tools pinned to case 0.
	var caseWrite []gollem.Tool
	if d.CaseWrite.CaseUC != nil && d.CaseWrite.CaseID != 0 {
		caseWrite = casewriter.New(d.CaseWrite)
	}
	// The cross-case toolset is built only when a CaseUsecase is wired (the
	// workspace-channel host); casemulti.New returns nil otherwise.
	var caseMulti []gollem.Tool
	if d.CaseMulti.CaseUC != nil {
		caseMulti = casemulti.New(d.CaseMulti)
	}
	return &ToolSetResolver{
		core:           coreTools,
		coreFull:       coreFullTools,
		memo:           memoTools,
		knowledgeWrite: knowledgeWrite,
		slack:          slacktool.NewReadOnly(d.Slack),
		notion:         notiontool.New(d.Notion),
		github:         githubtool.New(d.GitHub),
		webfetch:       webfetch.New(d.WebFetch),
		jira:           d.Jira,
		caseWrite:      caseWrite,
		knowledge:      knowledge,
		caseMulti:      caseMulti,
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
	// The knowledge base set is read-only, and requesting ToolSetKnowledge
	// REPLACES it with the read+write set rather than adding to it — the write
	// set already contains the read tools, and offering a tool twice makes the
	// model's tool list ambiguous.
	base := r.knowledge
	if slices.Contains(ids, ToolSetKnowledge) && len(r.knowledgeWrite) > 0 {
		base = r.knowledgeWrite
	}

	// Pre-compute capacity to avoid repeated growth.
	total := len(base)
	for _, id := range ids {
		total += len(r.setFor(id))
	}
	out := make([]gollem.Tool, 0, total)
	out = append(out, base...)
	for _, id := range ids {
		out = append(out, r.setFor(id)...)
	}
	return out
}

// setFor maps one toolset id to its built slice. Unknown ids resolve to nothing
// so a stray id cannot crash a turn.
//
// ToolSetKnowledge resolves to nothing here on purpose: it selects which
// knowledge set Resolve uses as its always-included base, so returning it again
// would duplicate every knowledge tool.
func (r *ToolSetResolver) setFor(id string) []gollem.Tool {
	switch id {
	case ToolSetCoreRO:
		return r.core
	case ToolSetCore:
		return r.coreFull
	case ToolSetSlackRO:
		return r.slack
	case ToolSetNotion:
		return r.notion
	case ToolSetGitHub:
		return r.github
	case ToolSetWebFetch:
		return r.webfetch
	case ToolSetJira:
		return r.jira
	case ToolSetCaseWrite:
		return r.caseWrite
	case ToolSetCaseMulti:
		return r.caseMulti
	case ToolSetMemo:
		return r.memo
	default:
		return nil
	}
}
