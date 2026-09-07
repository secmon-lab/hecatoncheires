package threadcase_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

// scriptedLLM pops canned responses in order; shared between planner and
// sub-agent calls (the order is deterministic).
func newThreadSession() *model.Session {
	return &model.Session{
		ID:          "s-thread-" + time.Now().Format("150405.000000"),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		WorkspaceID: "support",
		CaseID:      42,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func newThreadWorkspace() *model.WorkspaceEntry {
	set, _ := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		CaseStatusSet:         set,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

func newThreadCase() *model.Case {
	return &model.Case{
		ID:             42,
		Title:          "Initial title",
		Status:         types.CaseStatusOpen,
		ReporterID:     "U-REPORTER",
		SlackChannelID: "C-MONITOR",
		SlackThreadTS:  "1700000000.000100",
		BoardStatus:    "TRIAGE",
	}
}

// investigatePlan is the round-1 plan that runs one read-only sub-agent.
// Thread-mode manages no Actions, so the planner is offered no core (action)
// toolset; the read-only Slack toolset stands in.
const investigatePlan = `{"message":"investigate the thread","tasks":[{"id":"t-1","title":"Review thread","description":"Review the message","acceptance_criteria":"reviewed","tools":["slack_ro"],"budget_usd":0.01}]}`

// replanDone terminates the loop. Under the explicit-finalize design an empty
// tasks list no longer signals completion; the planner must emit `finalize`.
const replanDone = `{"message":"enough context","finalize":{"reason":"goal met"}}`

// recordingCaseMutator is a casewriter.CaseMutator that records the mutations a
// sub-agent's tool calls perform, so the mention tests can prove the call
// actually reached the case usecase (the whole point of the responsibility
// split: a case change is a sub-agent tool side effect, not a host-applied
// decision).
func TestBuildSystemPrompt_ThreadContext(t *testing.T) {
	ws := newThreadWorkspace()
	c := newThreadCase()
	c.Title = "Payment outage"

	prompt := threadcase.BuildSystemPromptForTest(c, ws, threadcase.ModeMention, "")
	gt.String(t, prompt).Contains("Payment outage")
	gt.String(t, prompt).Contains("severity")
	gt.String(t, prompt).Contains("DONE")
	gt.String(t, prompt).Contains("CANNOT create or manage Actions")
}

// A mention turn advertises the full case writer set, so the prompt must name
// every tool the sub-agents actually get — a prompt that omits one drives the
// model to tell the user it lacks the capability (the reported failure was a
// request to set assignees).
func TestBuildSystemPrompt_MentionModeAdvertisesEveryWriteTool(t *testing.T) {
	prompt := threadcase.BuildSystemPromptForTest(newThreadCase(), newThreadWorkspace(), threadcase.ModeMention, "")
	gt.String(t, prompt).Contains("case__update_case_status")
	gt.String(t, prompt).Contains("case__assign")
	gt.String(t, prompt).Contains("case__unassign")
	gt.String(t, prompt).Contains("case__update_case")
	// materialize replaces title/description wholesale, so the two content paths
	// must not be mixed within one turn.
	gt.String(t, prompt).Contains("pick one content path per turn")
}

// The agent has no tool to read the case back, so the current assignees must be
// in the snapshot — including when the list is empty, which is exactly the state
// a "set the assignees" request starts from.
func TestBuildSystemPrompt_RendersAssignees(t *testing.T) {
	ws := newThreadWorkspace()

	assigned := newThreadCase()
	assigned.AssigneeIDs = []string{"U-ONE", "U-TWO"}
	gt.String(t, threadcase.BuildSystemPromptForTest(assigned, ws, threadcase.ModeMention, "")).
		Contains("- Assignees (Slack user IDs): U-ONE, U-TWO")

	empty := newThreadCase()
	empty.AssigneeIDs = nil
	gt.String(t, threadcase.BuildSystemPromptForTest(empty, ws, threadcase.ModeMention, "")).
		Contains("- Assignees (Slack user IDs): (empty)")
}

func TestBuildSystemPrompt_CreateMode_WorkspacePrompt(t *testing.T) {
	ws := createTestWorkspace()
	ws.CaseCreatePrompt = "Always fill the severity field for security cases."

	// ModeCreate (no case yet) renders the schema and appends the workspace
	// instructions.
	prompt := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate, "")
	gt.String(t, prompt).Contains("NO case exists yet")
	gt.String(t, prompt).Contains("severity")
	gt.String(t, prompt).Contains("Workspace-specific instructions")
	gt.String(t, prompt).Contains("Always fill the severity field")

	// Empty CaseCreatePrompt → no workspace-specific section.
	ws.CaseCreatePrompt = ""
	bare := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate, "")
	gt.Bool(t, strings.Contains(bare, "Workspace-specific instructions")).False()
}

func TestBuildSystemPrompt_CreateInstruction(t *testing.T) {
	ws := createTestWorkspace()
	instruction := "Read the messages before and after the anchored message."

	// ModeCreate with an instruction renders the trigger-context section.
	prompt := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate, instruction)
	gt.String(t, prompt).Contains("# Trigger context")
	gt.String(t, prompt).Contains(instruction)

	// Empty instruction → no trigger-context section.
	bare := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate, "")
	gt.Bool(t, strings.Contains(bare, "# Trigger context")).False()

	// The instruction is create-only: a mention turn must not carry it.
	c := newThreadCase()
	mention := threadcase.BuildSystemPromptForTest(c, ws, threadcase.ModeMention, instruction)
	gt.Bool(t, strings.Contains(mention, "# Trigger context")).False()
	gt.Bool(t, strings.Contains(mention, instruction)).False()
}

// The ModeCreate field-schema block must give the planner the hints it needs to
// fill fields correctly on the first attempt: which fields are required, and the
// exact RFC3339 format for date fields (whose bare-date value was the reported
// failure). The instruction text now promises feedback-and-retry rather than the
// old "NO retry" wording.
func TestBuildSystemPrompt_CreateMode_FieldSchemaHints(t *testing.T) {
	ws := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "support", Name: "Support"},
		CaseMode:  model.CaseModeThread,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{{ID: "high", Name: "High"}}},
				{ID: "due_date", Name: "Due date", Type: types.FieldTypeDate},
			},
		},
	}
	prompt := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate, "")
	// Required fields are marked so the planner knows which it must fill.
	gt.String(t, prompt).Contains("(required)")
	// Date fields spell out the exact RFC3339 format the validator enforces.
	gt.String(t, prompt).Contains("format=RFC3339")
	gt.String(t, prompt).Contains("2026-07-14T00:00:00Z")
	// The instruction promises feedback-and-retry, not the removed "NO retry".
	gt.String(t, prompt).Contains("fed back to you")
	gt.Bool(t, strings.Contains(prompt, "NO retry")).False()
}

func TestDecision_Validate(t *testing.T) {
	// Unknown kind is rejected.
	gt.Error(t, threadcase.Decision{Kind: "explode"}.Validate())
	// respond requires a non-empty message.
	gt.Error(t, threadcase.Decision{Kind: threadcase.DecisionRespond}.Validate())
	gt.NoError(t, threadcase.Decision{Kind: threadcase.DecisionRespond, Message: "hi"}.Validate())
	// materialize requires both title and description.
	gt.Error(t, threadcase.Decision{Kind: threadcase.DecisionMaterialize, Title: "t"}.Validate())
	gt.NoError(t, threadcase.Decision{Kind: threadcase.DecisionMaterialize, Title: "t", Description: "d"}.Validate())
}

func TestBuildUserInput_FallsBackWhenEmpty(t *testing.T) {
	got, err := threadcase.BuildUserInputForTest(time.Time{}, nil, nil, threadcase.ConversationMessage{})
	gt.NoError(t, err).Required()
	gt.String(t, got).NotEqual("")
}

// The turn's absolute instant. Without it the planner resolves "today" against
// whatever date its training suggests — the create turn that produced this test
// was told "by today" and wrote a due date a month in the past.
//
// It goes in the USER message, not the system prompt, because a thread-mode turn
// continues the previous turn's conversation and the system prompt is not part of
// that history: a resumed turn would otherwise carry the earlier messages with
// nothing saying when they were written.
func TestBuildUserInput_StatesTheCurrentTime(t *testing.T) {
	now := time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC)

	t.Run("LeadsTheMessageWithTheInstant", func(t *testing.T) {
		got, err := threadcase.BuildUserInputForTest(now, nil, nil,
			threadcase.ConversationMessage{Text: "<@bot> due today"})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("2026-09-07T03:35:19Z")
		gt.String(t, got).Contains("(UTC)")
		// Before the conversation it dates.
		gt.Bool(t, strings.Index(got, "# Current time") < strings.Index(got, "# Current mention")).True()
	})

	// A resumed turn inherits the previous turn's messages, each carrying its own
	// current time. The section must say which one is now.
	t.Run("SaysTheLatestSectionIsNow", func(t *testing.T) {
		got, err := threadcase.BuildUserInputForTest(now, nil, nil,
			threadcase.ConversationMessage{Text: "<@bot> due today"})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("the latest one is now")
	})

	// A non-UTC instant is still rendered in UTC, so the stated zone and the value
	// can never disagree.
	t.Run("RendersInUTCWhateverZoneItIsGiven", func(t *testing.T) {
		jst := time.FixedZone("JST", 9*60*60)
		got, err := threadcase.BuildUserInputForTest(
			time.Date(2026, 9, 7, 12, 35, 19, 0, jst), nil, nil,
			threadcase.ConversationMessage{Text: "<@bot> due today"})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("2026-09-07T03:35:19Z")
	})

	// The fallback instruction is what a materialize turn with no messages is
	// given; the time section must not stand in for it.
	t.Run("KeepsTheFallbackInstructionWhenThereIsNoConversation", func(t *testing.T) {
		got, err := threadcase.BuildUserInputForTest(now, nil, nil, threadcase.ConversationMessage{})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("2026-09-07T03:35:19Z")
		gt.String(t, got).Contains("Investigate this case and decide the next action.")
	})

	t.Run("ZeroTimeOmitsTheSection", func(t *testing.T) {
		got, err := threadcase.BuildUserInputForTest(time.Time{}, nil, nil,
			threadcase.ConversationMessage{Text: "<@bot> due today"})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(got, "# Current time")).False()
		gt.Bool(t, strings.Contains(got, "0001-01-01")).False()
	})
}

// The mention-mode system prompt tells the agent to resolve a named person to a
// Slack user ID before calling case__assign, and no tool can look one up. Every
// conversation line must therefore carry the author's ID alongside the display
// name, or that instruction is unsatisfiable.
func TestBuildUserInput_RendersSpeakerIDs(t *testing.T) {
	msgs := []threadcase.ConversationMessage{
		{Timestamp: "1700000000.000100", UserID: "U-ALICE", UserName: "Alice", Text: "the DB is down"},
		{Timestamp: "1700000000.000200", UserID: "U-BOB", Text: "looking into it"},
		{Timestamp: "1700000000.000300", UserName: "Webhook", Text: "alert fired"},
	}

	got, err := threadcase.BuildUserInputForTest(time.Time{}, msgs, nil, threadcase.ConversationMessage{})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("[1700000000.000100] Alice (U-ALICE): the DB is down")
	// Name unknown: the ID alone still reaches the model.
	gt.String(t, got).Contains("[1700000000.000200] U-BOB: looking into it")
	// ID unknown (e.g. a bot post): degrade to the name rather than printing "()".
	gt.String(t, got).Contains("[1700000000.000300] Webhook: alert fired")
}

// "assign me" is only actionable when the mention's own author is identified,
// so the current mention block names its speaker.
func TestBuildUserInput_RendersMentionAuthor(t *testing.T) {
	mention := threadcase.ConversationMessage{
		Timestamp: "1700000009.000001",
		UserID:    "U-CALLER",
		UserName:  "Caller",
		Text:      "<@bot> assign me",
	}

	got, err := threadcase.BuildUserInputForTest(time.Time{}, nil, nil, mention)
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("# Current mention")
	gt.String(t, got).Contains("From: Caller (U-CALLER)")
	gt.String(t, got).Contains("<@bot> assign me")

	// An unattributed mention still renders its text, with no dangling "From:".
	anon, err := threadcase.BuildUserInputForTest(time.Time{}, nil, nil,
		threadcase.ConversationMessage{Text: "<@bot> hello"})
	gt.NoError(t, err).Required()
	gt.String(t, anon).Contains("<@bot> hello")
	gt.Bool(t, strings.Contains(anon, "From:")).False()
}

// The mention itself is already rendered under "# Current mention", so it must
// not also appear in the thread transcript above it.
func TestBuildUserInput_SkipsTheMentionInTheTranscript(t *testing.T) {
	mention := threadcase.ConversationMessage{
		Timestamp: "1700000009.000001",
		UserID:    "U-CALLER",
		Text:      "<@bot> assign me",
	}
	msgs := []threadcase.ConversationMessage{
		{Timestamp: "1700000000.000100", UserID: "U-ALICE", Text: "earlier note"},
		mention,
	}

	got, err := threadcase.BuildUserInputForTest(time.Time{}, msgs, nil, mention)
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("earlier note")
	gt.Number(t, strings.Count(got, "<@bot> assign me")).Equal(1)
}

// createWorkspaceEntry is the workspace used by the ModeCreate tests: it has a
// required select (severity) and a required text (summary).
func createTestWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
				{ID: "summary", Name: "Summary", Type: types.FieldTypeText, Required: true},
			},
		},
	}
}
