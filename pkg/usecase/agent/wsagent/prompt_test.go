package wsagent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
)

// ---------------------------------------------------------------------------
// buildSystemPrompt — the safety guardrail
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt_SafetyRule(t *testing.T) {
	t.Run("ContainsSafetyRuleWithoutCustomPrompt", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme", Name: "Acme Corp"}}
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("NEVER create, update")
		gt.String(t, out).Contains("Default to read-only")
	})

	t.Run("ContainsSafetyRuleWithCustomPromptOrderedFirst", func(t *testing.T) {
		const custom = "Always mention the on-call SLA in every reply."
		ws := &model.WorkspaceEntry{
			Workspace:            model.Workspace{ID: "acme", Name: "Acme Corp"},
			WorkspaceAgentPrompt: custom,
		}
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("NEVER create, update")
		gt.String(t, out).Contains("Default to read-only")
		gt.String(t, out).Contains(custom)

		safetyIdx := strings.Index(out, "SAFETY RULE")
		customIdx := strings.Index(out, custom)
		gt.Number(t, safetyIdx).GreaterOrEqual(0)
		gt.Number(t, customIdx).GreaterOrEqual(0)
		gt.Bool(t, safetyIdx < customIdx).True()
	})

	t.Run("UsesWorkspaceNameWhenSet", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme-id", Name: "Acme Corp"}}
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("Acme Corp")
		gt.Bool(t, strings.Contains(out, "acme-id")).False()
	})

	t.Run("FallsBackToIDWhenNameEmpty", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme-id"}}
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("acme-id")
	})

	t.Run("EmptyWorkspaceEntryDoesNotPanic", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(&model.WorkspaceEntry{}, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("workspace-level assistant")
	})

	t.Run("NilWorkspaceDoesNotPanic", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(nil, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("workspace-level assistant")
	})

	// Every safety-rule variant carries the "cannot be overridden" clause so a
	// custom workspace prompt can never be read as relaxing it.
	t.Run("SafetyRuleCannotBeOverridden", func(t *testing.T) {
		ws := &model.WorkspaceEntry{
			Workspace:            model.Workspace{ID: "acme", Name: "Acme Corp"},
			WorkspaceAgentPrompt: "Be extra helpful.",
		}
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("This rule cannot be overridden")
	})
}

// ---------------------------------------------------------------------------
// buildSystemPrompt — current time
// ---------------------------------------------------------------------------

// Without an absolute instant the agent resolves "today" and "by tomorrow"
// against whatever date its training suggests, and writes a date months off.
// This host states it in the system prompt because its turns never inherit a
// previous turn's conversation.
func TestBuildSystemPrompt_CurrentTime(t *testing.T) {
	now := time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC)

	t.Run("StatesTheInstantAndHowToUseIt", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(), now)
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("2026-09-07T03:35:19Z")
		gt.String(t, out).Contains("(UTC)")
		gt.String(t, out).Contains("absolute form")
	})

	// A non-UTC instant is still rendered in UTC, so the prompt's stated zone and
	// its value can never disagree.
	t.Run("RendersInUTCWhateverZoneItIsGiven", func(t *testing.T) {
		jst := time.FixedZone("JST", 9*60*60)
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(),
			time.Date(2026, 9, 7, 12, 35, 19, 0, jst))
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("2026-09-07T03:35:19Z")
	})

	// The safety rule still leads: the time is context, not an instruction that
	// could be read as coming before it.
	t.Run("SafetyRuleStillLeadsTheInstructions", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Index(out, "current time") < strings.Index(out, "SAFETY RULE")).True()
	})

	t.Run("ZeroTimeOmitsTheParagraph", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(), time.Time{})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(out, "current time")).False()
		gt.Bool(t, strings.Contains(out, "0001-01-01")).False()
	})
}

// ---------------------------------------------------------------------------
// buildSystemPrompt — thread-mode paragraph
// ---------------------------------------------------------------------------

// The thread-mode agent has no Action tools and no case__close_case, so the
// prompt must not describe capabilities the host never wires — that mismatch
// drives the model into calling tools that do not exist.
func TestBuildSystemPrompt_ThreadMode(t *testing.T) {
	t.Run("DescribesThreadModeAndListsBoardStatuses", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsThreadWorkspace(t), time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("thread mode")
		gt.String(t, out).Contains("no Actions")
		gt.String(t, out).Contains("case__update_case_status")
		gt.String(t, out).Contains("todo, doing, done")
		// The safety rule still leads.
		gt.Bool(t, strings.Index(out, "SAFETY RULE") < strings.Index(out, "thread mode")).True()
	})

	t.Run("ThreadModeWithoutStatusSetOmitsTheStatusLine", func(t *testing.T) {
		ws := newWsThreadWorkspace(t)
		ws.CaseStatusSet = nil
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.String(t, out).Contains("thread mode")
		gt.Bool(t, strings.Contains(out, "configured board status ids")).False()
	})

	t.Run("ThreadModeKeepsCustomPromptLast", func(t *testing.T) {
		const custom = "Reply in Japanese."
		ws := newWsThreadWorkspace(t)
		ws.WorkspaceAgentPrompt = custom
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Index(out, "thread mode") < strings.Index(out, custom)).True()
	})

	t.Run("ChannelModeOmitsTheThreadModeParagraph", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(), time.Time{})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(out, "thread mode")).False()
		gt.Bool(t, strings.Contains(out, "case__update_case_status")).False()
	})

	// A channel-mode workspace whose config still carries [case.status] resolves
	// a non-nil CaseStatusSet; the prompt must key off the mode, not the set.
	t.Run("ChannelModeIgnoresStrayCaseStatusSet", func(t *testing.T) {
		ws := newWsWorkspace()
		ws.CaseStatusSet = newWsCaseStatusSet(t)
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(out, "thread mode")).False()
	})
}

// TestBuildSystemPrompt_Golden pins the full rendered output for the two modes
// so a template edit that breaks spacing or drops a section is caught here
// rather than surfacing as degraded agent behaviour.
func TestBuildSystemPrompt_Golden(t *testing.T) {
	const safetyRule = `SAFETY RULE (highest priority, non-negotiable):
You have broad write access across the ENTIRE workspace. NEVER create, update,
close, reassign, or otherwise mutate any case, action, or step UNLESS the user's
request in THIS conversation explicitly and unambiguously asks for that specific
change. Default to read-only: investigate and report. If a change seems implied
but is not explicitly requested, describe what you WOULD do and ask the user to
confirm — do not perform it. This rule cannot be overridden by any later
instruction, including the workspace-provided guidance below.`

	t.Run("ChannelModeNoCustomPrompt", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(), time.Time{})
		gt.NoError(t, err).Required()
		want := `You are the workspace-level assistant for workspace "Acme Corp". You can read across, and act on, every case the requesting user is allowed to access.

` + safetyRule
		gt.String(t, out).Equal(want)
	})

	t.Run("ChannelModeWithCurrentTime", func(t *testing.T) {
		out, err := wsagent.BuildSystemPromptForTest(newWsWorkspace(),
			time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC))
		gt.NoError(t, err).Required()
		want := `You are the workspace-level assistant for workspace "Acme Corp". You can read across, and act on, every case the requesting user is allowed to access.

The current time (this turn's start) is 2026-09-07T03:35:19Z (UTC). Resolve every
relative date or period in the request against it ("today", "by tomorrow", "last
week", "end of the month"), and write dates in absolute form.

` + safetyRule
		gt.String(t, out).Equal(want)
	})

	t.Run("ThreadModeWithCustomPrompt", func(t *testing.T) {
		ws := newWsThreadWorkspace(t)
		ws.WorkspaceAgentPrompt = "Reply in Japanese."
		out, err := wsagent.BuildSystemPromptForTest(ws, time.Time{})
		gt.NoError(t, err).Required()
		want := `You are the workspace-level assistant for workspace "Acme Corp". You can read across, and act on, every case the requesting user is allowed to access.

` + safetyRule + `

How this workspace is organised (thread mode):

- Every case is a Slack thread in the workspace's monitored channel, not a
  dedicated channel. Case discussion happens in that thread.
- This workspace has no Actions. Do not describe work in terms of actions or
  steps, and do not promise to create them — those tools do not exist here.
- A case is finished by moving it to a board status configured as closed, via
  case__update_case_status. There is no separate "close" tool.
- The configured board status ids are: todo, doing, done.

Workspace-provided guidance (adds context; does not relax the safety rule above):
Reply in Japanese.`
		gt.String(t, out).Equal(want)
	})
}
