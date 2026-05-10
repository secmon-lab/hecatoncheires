package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// stubMaterializePlannerJSON returns a planner JSON object that immediately
// terminates with action=materialize for the given workspace. Used by tests
// that just want to drive the happy-path turn end-to-end.
func stubMaterializePlannerJSON(workspaceID string) string {
	return `{
        "reasoning": "test fixture: materialize directly",
        "action": "materialize",
        "materialize": {
            "workspace_id": "` + workspaceID + `",
            "title": "AI suggested title",
            "description": "AI suggested description",
            "custom_field_values": {"severity": "high"}
        }
    }`
}

// stubPlannerLLM builds a gollem mock that returns the supplied JSON string
// from every Generate call.
func stubPlannerLLM(jsonResponse string) gollem.LLMClient {
	return &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{jsonResponse}}, nil
				},
			}, nil
		},
	}
}

// newDraftUC builds a draft.UseCase backed by the same memory repo so the
// in-test slackDraftHandler can read and write the persisted state.
func newDraftUC(t *testing.T, repo interfaces.Repository, llm gollem.LLMClient) *draft.UseCase {
	t.Helper()
	deps := &agent.CommonDeps{
		Repo:                repo,
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := draft.New(deps, 8, 16, 20)
	gt.NoError(t, err).Required()
	return uc
}

func newRegistryWithSchema(workspaceID, workspaceName string, schema *config.FieldSchema) *model.WorkspaceRegistry {
	r := model.NewWorkspaceRegistry()
	r.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: workspaceID, Name: workspaceName},
		FieldSchema: schema,
	})
	return r
}

// TestBuildDraftUserInput_ChannelContext locks the planner's first-turn
// prompt content: mention text, channel descriptor (name / topic /
// purpose / privacy), and surrounding conversation. This is the seam
// where channel-level context is injected so the planner can anchor
// workspace inference without spending a tool call on it.
func TestBuildDraftUserInput_ChannelContext(t *testing.T) {
	d := &model.CaseDraft{
		RawMessages: []model.DraftMessage{
			{TS: "1700000000.000100", UserID: "U001", Text: "first line"},
			{TS: "1700000001.000100", UserID: "U002", Text: "second\nline"},
		},
	}
	ci := &slacksvc.ChannelInfo{
		ID:         "C-RISK",
		Name:       "sec-risk-ops",
		Topic:      "Daily ops for the security risk team",
		Purpose:    "Triaging incoming risk reports\nand coordinating response",
		IsPrivate:  true,
		IsShared:   false,
		IsArchived: false,
		NumMembers: 17,
		Creator:    "U999",
	}

	got := usecase.BuildDraftUserInputForTest(d, "@bot please draft a case for Tanaka's issue", ci)

	gt.S(t, got).Contains("# User mention")
	gt.S(t, got).Contains("please draft a case for Tanaka's issue")
	gt.S(t, got).Contains("# Channel context")
	gt.S(t, got).Contains("- name: #sec-risk-ops")
	gt.S(t, got).Contains("- topic: Daily ops for the security risk team")
	// Newlines inside topic / purpose are flattened to single spaces so
	// the planner sees a single-row entry.
	gt.S(t, got).Contains("- description: Triaging incoming risk reports and coordinating response")
	gt.S(t, got).Contains("- privacy: private")
	gt.S(t, got).Contains("- members: 17")
	gt.S(t, got).Contains("- creator: U999")
	gt.S(t, got).Contains("# Surrounding conversation (chronological, oldest first)")
	gt.S(t, got).Contains("first line")
	gt.S(t, got).Contains("second line") // newline collapsed
}

// TestBuildDraftUserInput_NilChannelInfoOmitsSection confirms the host
// degrades gracefully when conversations.info fails: the planner still
// gets the mention text and the surrounding conversation, just no
// channel-level hints.
func TestBuildDraftUserInput_NilChannelInfoOmitsSection(t *testing.T) {
	d := &model.CaseDraft{}
	got := usecase.BuildDraftUserInputForTest(d, "@bot something", nil)

	gt.S(t, got).Contains("# User mention")
	gt.S(t, got).Contains("@bot something")
	gt.Bool(t, strings.Contains(got, "# Channel context")).False()
}

func TestMentionDraftUseCase_HandleAppMention_HappyPath(t *testing.T) {
	repo := memory.New()
	schema := &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "severity", Type: types.FieldTypeSelect,
			Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
	}}
	registry := newRegistryWithSchema("ws-only", "OnlyWS", schema)

	slackMock := newCollectorOnlyMockSlack()
	uc := usecase.NewMentionDraftUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-only"))))
	gt.Value(t, uc).NotNil().Required()

	ev := &slackevents.AppMentionEvent{
		Channel:   "C-USER",
		User:      "U-USER",
		Text:      "<@BOT> please open a case",
		TimeStamp: "1700000010.000000",
	}

	gt.NoError(t, uc.HandleAppMention(context.Background(), ev)).Required()

	// First a "processing…" thread message is posted, then it's
	// UpdateMessage-replaced with the full preview blocks.
	gt.Number(t, len(slackMock.threadBlockPosts)).GreaterOrEqual(1)
	gt.Number(t, len(slackMock.updateBlockPosts)).GreaterOrEqual(1)
	last := slackMock.updateBlockPosts[len(slackMock.updateBlockPosts)-1]
	gt.Value(t, last.channelID).Equal("C-USER")
	gt.Number(t, len(last.blocks)).GreaterOrEqual(3) // title+desc markdown, divider, actions at minimum
}

func TestMentionDraftUseCase_HandleAppMention_NoWorkspace_PostsError(t *testing.T) {
	repo := memory.New()
	registry := model.NewWorkspaceRegistry() // empty
	slackMock := newCollectorOnlyMockSlack()
	uc := usecase.NewMentionDraftUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-only"))))

	ev := &slackevents.AppMentionEvent{
		Channel:   "C1",
		User:      "U1",
		Text:      "<@BOT> hi",
		TimeStamp: "1700000010.000000",
	}
	gt.NoError(t, uc.HandleAppMention(context.Background(), ev)).Required()

	// PostThreadMessage (text only) called for the no-workspace error.
	gt.Array(t, slackMock.threadTexts).Length(1)
	gt.String(t, slackMock.threadTexts[0]).Contains("No workspace")
	// The processing block was posted then immediately UpdateMessage-cleared
	// by removeProcessingMessage; both calls show in the mock.
	gt.Number(t, len(slackMock.threadBlockPosts)).Equal(1)
	gt.Number(t, len(slackMock.updateBlockPosts)).GreaterOrEqual(1)
}

func TestMentionDraftUseCase_NilSlackService(t *testing.T) {
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
	uc := usecase.NewMentionDraftUseCase(repo, registry, nil, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))
	gt.Value(t, uc).Nil()
}

func TestSlackUseCases_AppMention_DispatchesToMentionDraft(t *testing.T) {
	repo := memory.New()
	schema := &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "severity", Type: types.FieldTypeSelect,
			Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
	}}
	registry := newRegistryWithSchema("ws-1", "ws", schema)

	slackMock := newCollectorOnlyMockSlack()
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))

	slackUC := usecase.NewSlackUseCases(repo, registry, nil, mentionDraft, slackMock)

	// Channel is NOT bound to any Case.
	ev := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T1",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{
				Channel:   "C-NEW",
				User:      "U1",
				Text:      "<@BOT>",
				TimeStamp: "1700000010.000000",
			},
		},
	}

	gt.NoError(t, slackUC.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(slackMock.threadBlockPosts)).GreaterOrEqual(1)
}

func TestSlackUseCases_AppMention_CaseBoundChannelDoesNotInvokeDraft(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})

	// Pre-create a Case whose Slack channel matches the mention channel.
	_, err := repo.Case().Create(ctx, "ws-1", &model.Case{
		Title:          "existing",
		Status:         types.CaseStatusOpen,
		SlackChannelID: "C-CASE",
	})
	gt.NoError(t, err).Required()

	slackMock := newCollectorOnlyMockSlack()
	llm := stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock, newDraftUC(t, repo, llm))
	agent := usecase.NewAgentUseCase(repo, registry, slackMock, nil, nil, nil, llm, llm, agentarchive.NewMemoryHistoryRepository(), agentarchive.NewMemoryTraceRepository(), nil, nil)
	slackUC := usecase.NewSlackUseCases(repo, registry, agent, mentionDraft, slackMock)

	ev := &slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{
				Channel:   "C-CASE",
				User:      "U1",
				Text:      "<@BOT>",
				TimeStamp: "1700000010.000000",
			},
		},
	}

	gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
	// MentionDraft must NOT have been invoked. Agent path posts a single
	// session-start block; the mentionDraft preview posts 4+ blocks.
	for _, post := range slackMock.threadBlockPosts {
		gt.Number(t, len(post.blocks)).LessOrEqual(1)
	}
}

// --- thread-reply dispatcher (F1-F8) tests ---

// dispatcherFixture wires a SlackUseCases for thread-reply tests with a
// pre-seeded Session in the requested state.
type dispatcherFixture struct {
	uc        *usecase.SlackUseCases
	repo      any // memory.New() — kept opaque so tests don't reach in
	slackMock *collectorOnlyMockSlack
}

func newDispatcherWithOpenSession(t *testing.T, channelID, threadTS string, lastAction model.SessionEndReason) *dispatcherFixture {
	t.Helper()
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock,
		newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))
	slackUC := usecase.NewSlackUseCases(repo, registry, nil, mentionDraft, slackMock)

	now := time.Now().UTC()
	gt.NoError(t, repo.Session().Put(context.Background(), &model.Session{
		ID:            "ssn-disp",
		ChannelID:     channelID,
		ThreadTS:      threadTS,
		CreatorUserID: "U-CREATOR",
		LastAction:    lastAction,
		CreatedAt:     now,
		UpdatedAt:     now,
	})).Required()

	return &dispatcherFixture{uc: slackUC, repo: repo, slackMock: slackMock}
}

func newMessageEvent(channel, user, text, ts, threadTS, subtype, botID string) *slackevents.EventsAPIEvent {
	return &slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Channel:         channel,
				User:            user,
				Text:            text,
				TimeStamp:       ts,
				ThreadTimeStamp: threadTS,
				SubType:         subtype,
				BotID:           botID,
			},
		},
	}
}

func TestDispatcher_ThreadReply_F1_DropOnSubType(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	ev := newMessageEvent("C-OPEN", "U1", "hello", "1700000020.000000", "1700000010.000000", "message_changed", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	// No turn fired → no thread blocks posted by handler.
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
	gt.Number(t, len(f.slackMock.updateBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F2_DropOnBotSelfPost(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	ev := newMessageEvent("C-OPEN", "BOT", "hi", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F3_DropOnBotID(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	ev := newMessageEvent("C-OPEN", "U1", "hi", "1700000020.000000", "1700000010.000000", "", "B999")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F4_DropOnTopLevel(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	// thread_ts == ts means the parent post itself; drop.
	ev := newMessageEvent("C-OPEN", "U1", "hi", "1700000020.000000", "1700000020.000000", "", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F5_DropOnMention(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	ev := newMessageEvent("C-OPEN", "U1", "<@BOT> hi", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F6_DropOnNoSession(t *testing.T) {
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock,
		newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))
	slackUC := usecase.NewSlackUseCases(repo, registry, nil, mentionDraft, slackMock)

	ev := newMessageEvent("C-NEW", "U1", "hi", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, slackUC.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F7_DropCaseBound(t *testing.T) {
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock,
		newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))
	slackUC := usecase.NewSlackUseCases(repo, registry, nil, mentionDraft, slackMock)

	now := time.Now().UTC()
	gt.NoError(t, repo.Session().Put(context.Background(), &model.Session{
		ID:         "ssn-cb",
		ChannelID:  "C-CB",
		ThreadTS:   "1700000010.000000",
		CaseID:     42, // case-bound → F7 drop
		LastAction: model.SessionEndedWithQuestion,
		CreatedAt:  now,
		UpdatedAt:  now,
	})).Required()

	ev := newMessageEvent("C-CB", "U1", "hi", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, slackUC.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_F8_DropOnNonQuestionEnd(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithMessage)
	ev := newMessageEvent("C-OPEN", "U1", "hi", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	gt.Number(t, len(f.slackMock.threadBlockPosts)).Equal(0)
}

func TestDispatcher_ThreadReply_HappyPath_ResumesTurn(t *testing.T) {
	f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
	ev := newMessageEvent("C-OPEN", "U1", "user follow-up answer", "1700000020.000000", "1700000010.000000", "", "")
	gt.NoError(t, f.uc.HandleSlackEvent(context.Background(), ev)).Required()
	// Planner stub returns materialize → handler posts blocks (preview).
	gt.Number(t, len(f.slackMock.threadBlockPosts)+len(f.slackMock.updateBlockPosts)).GreaterOrEqual(1)
}

// --- collector-only mock slack service ---

type ephemeralBlockPost struct {
	channelID string
	userID    string
	blocks    []slackBlockSnapshot
	// rawBlocks carries the actual Block Kit blocks the production code
	// passed in. Most assertions only need the count (recorded in `blocks`)
	// but tests that need to inspect rendered text/markdown can reach into
	// rawBlocks. Filled by UpdateMessage / PostThreadMessage / etc.
	rawBlocks []goslack.Block
}

// slackBlockSnapshot is intentionally opaque; we only check counts and
// presence rather than the deep Block Kit structure.
type slackBlockSnapshot struct{}

type collectorOnlyMockSlack struct {
	thread              []slacksvc.ConversationMessage
	history             []slacksvc.ConversationMessage
	ephemeralText       string
	ephemeralBlockPosts []ephemeralBlockPost
	threadTexts         []string
	threadReplies       []string // texts posted via PostThreadReply
	threadBlockPosts    []ephemeralBlockPost
	updateBlockPosts    []ephemeralBlockPost
}

func newCollectorOnlyMockSlack() *collectorOnlyMockSlack {
	return &collectorOnlyMockSlack{}
}

// --- collector-required impls ---

func (m *collectorOnlyMockSlack) GetConversationReplies(_ context.Context, _ string, _ string, _ int) ([]slacksvc.ConversationMessage, error) {
	return m.thread, nil
}
func (m *collectorOnlyMockSlack) GetConversationHistory(_ context.Context, _ string, _ time.Time, _ int) ([]slacksvc.ConversationMessage, error) {
	return m.history, nil
}
func (m *collectorOnlyMockSlack) GetPermalink(_ context.Context, channelID, ts string) (string, error) {
	return "https://slack/" + channelID + "/" + ts, nil
}
func (m *collectorOnlyMockSlack) PostEphemeral(_ context.Context, _ string, _ string, text string) error {
	m.ephemeralText = text
	return nil
}
func (m *collectorOnlyMockSlack) PostEphemeralBlocks(_ context.Context, channelID string, userID string, blocks []goslack.Block, _ string) (string, error) {
	snaps := make([]slackBlockSnapshot, len(blocks))
	m.ephemeralBlockPosts = append(m.ephemeralBlockPosts, ephemeralBlockPost{
		channelID: channelID,
		userID:    userID,
		blocks:    snaps,
	})
	return "ts-eph", nil
}

// --- unused interface stubs ---

func (m *collectorOnlyMockSlack) ListJoinedChannels(context.Context, string) ([]slacksvc.Channel, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) GetChannelNames(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) GetUserInfo(context.Context, string) (*slacksvc.User, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) ListUsers(context.Context, string) ([]*slacksvc.User, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) CreateChannel(context.Context, int64, string, string, bool, string) (string, error) {
	// Return a deterministic synthetic channel ID so post-create assertions
	// have something to recognise as a Slack channel mention. Tests that do
	// not care about the value still see a non-empty string.
	return "C-CREATED", nil
}
func (m *collectorOnlyMockSlack) GetConversationMembers(context.Context, string) ([]string, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) GetChannelInfo(_ context.Context, channelID string) (*slacksvc.ChannelInfo, error) {
	return &slacksvc.ChannelInfo{
		ID:      channelID,
		Name:    "draft-test",
		Topic:   "drafting test cases",
		Purpose: "fixture channel for the draft mention flow",
	}, nil
}
func (m *collectorOnlyMockSlack) RenameChannel(context.Context, string, int64, string, string) error {
	return nil
}
func (m *collectorOnlyMockSlack) InviteUsersToChannel(context.Context, string, []string) error {
	return nil
}
func (m *collectorOnlyMockSlack) AddBookmark(context.Context, string, string, string) error {
	return nil
}
func (m *collectorOnlyMockSlack) GetTeamURL(context.Context) (string, error) { return "", nil }
func (m *collectorOnlyMockSlack) PostMessage(context.Context, string, []goslack.Block, string) (string, error) {
	return "", nil
}
func (m *collectorOnlyMockSlack) UpdateMessage(_ context.Context, channelID string, _ string, blocks []goslack.Block, _ string) error {
	snaps := make([]slackBlockSnapshot, len(blocks))
	m.updateBlockPosts = append(m.updateBlockPosts, ephemeralBlockPost{
		channelID: channelID,
		blocks:    snaps,
		rawBlocks: append([]goslack.Block(nil), blocks...),
	})
	return nil
}
func (m *collectorOnlyMockSlack) PostThreadReply(_ context.Context, _ string, _ string, text string) (string, error) {
	m.threadReplies = append(m.threadReplies, text)
	return "ts-reply", nil
}
func (m *collectorOnlyMockSlack) PostThreadMessage(_ context.Context, channelID string, _ string, blocks []goslack.Block, text string) (string, error) {
	if len(blocks) > 0 {
		snaps := make([]slackBlockSnapshot, len(blocks))
		m.threadBlockPosts = append(m.threadBlockPosts, ephemeralBlockPost{
			channelID: channelID,
			blocks:    snaps,
		})
	} else {
		m.threadTexts = append(m.threadTexts, text)
	}
	return "ts-thread", nil
}
func (m *collectorOnlyMockSlack) GetBotUserID(context.Context) (string, error) { return "BOT", nil }
func (m *collectorOnlyMockSlack) OpenView(context.Context, string, goslack.ModalViewRequest) error {
	return nil
}
func (m *collectorOnlyMockSlack) ListUserGroups(context.Context, string) ([]slacksvc.UserGroup, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) GetUserGroupMembers(context.Context, string) ([]string, error) {
	return nil, nil
}
func (m *collectorOnlyMockSlack) ListTeams(context.Context) ([]slacksvc.Team, error) {
	return nil, nil
}

// --- Lifecycle (multi-turn integration) tests ---
//
// These tests drive the *whole* dispatcher path (HandleSlackEvent /
// HandleSelectWorkspace / HandleSubmit) across multiple turns to catch
// state-machine bugs that per-method tests cannot. They share three pieces
// of infrastructure:
//
//  1. lifecycleHarness — assembles MentionDraftUseCase + SlackUseCases against
//     a single memory repo, using a scripted LLM client.
//  2. scriptedPlannerLLM — sequences planner JSON outputs, with optional
//     keyed sub-agent canned summaries. Failing the test if more LLM calls
//     happen than scripted catches budget regressions.
//  3. mention/messageEvent helpers — produce real Slack event shapes so the
//     dispatcher walks the same code path as production.

// scriptedPlannerLLM returns a planner JSON entry per Generate call (in
// order) and routes a configured set of inputs to canned sub-agent summaries.
// Test scope: catches "planner called more times than expected" by failing
// the test on overflow.
type scriptedPlannerLLM struct {
	t              *testing.T
	plannerScript  []string
	subAgentByDesc map[string]string
	plannerCalls   atomic.Int32
	subAgentCalls  atomic.Int32
}

func newScriptedPlannerLLM(t *testing.T, plannerScript []string, subAgentByDesc map[string]string) gollem.LLMClient {
	t.Helper()
	s := &scriptedPlannerLLM{
		t:              t,
		plannerScript:  plannerScript,
		subAgentByDesc: subAgentByDesc,
	}
	return &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, input ...gollem.Input) (*gollem.Response, error) {
					if len(input) > 0 {
						if txt, ok := input[0].(gollem.Text); ok {
							if canned, ok := s.subAgentByDesc[string(txt)]; ok {
								s.subAgentCalls.Add(1)
								return &gollem.Response{Texts: []string{canned}}, nil
							}
						}
					}
					n := int(s.plannerCalls.Add(1) - 1)
					if n >= len(s.plannerScript) {
						s.t.Errorf("planner script exhausted at call %d (only %d responses configured)", n+1, len(s.plannerScript))
						return nil, errors.New("planner script exhausted")
					}
					return &gollem.Response{Texts: []string{s.plannerScript[n]}}, nil
				},
			}, nil
		},
	}
}

// lifecycleHarness wires the host-side usecases against a shared memory repo
// and the supplied scripted LLM. Returns the SlackUseCases (the dispatcher)
// and the MentionDraftUseCase (so tests can drive interaction handlers).
type lifecycleHarness struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackMock    *collectorOnlyMockSlack
	mentionDraft *usecase.MentionDraftUseCase
	slackUC      *usecase.SlackUseCases
	caseUC       *usecase.CaseUseCase
}

func newLifecycleHarness(t *testing.T, registry *model.WorkspaceRegistry, llm gollem.LLMClient) *lifecycleHarness {
	t.Helper()
	repo := memory.New()
	slackMock := newCollectorOnlyMockSlack()
	mentionDraft := usecase.NewMentionDraftUseCase(repo, registry, slackMock, newDraftUC(t, repo, llm))
	caseUC := usecase.NewCaseUseCase(repo, registry, slackMock, nil, "")
	slackUC := usecase.NewSlackUseCases(repo, registry, nil, mentionDraft, slackMock)
	return &lifecycleHarness{
		repo:         repo,
		registry:     registry,
		slackMock:    slackMock,
		mentionDraft: mentionDraft,
		slackUC:      slackUC,
		caseUC:       caseUC,
	}
}

func appMentionEvent(channel, user, text, ts string) *slackevents.EventsAPIEvent {
	return &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T1",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{
				Channel:   channel,
				User:      user,
				Text:      text,
				TimeStamp: ts,
			},
		},
	}
}

// schemaWithSeverity builds a single-field FieldSchema with a `severity`
// select for use across lifecycle tests. Two option IDs (low/high) give
// schema-validation assertions something to match against.
func schemaWithSeverity() *config.FieldSchema {
	return &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "severity", Type: types.FieldTypeSelect, Required: true,
			Options: []config.FieldOption{{ID: "low", Name: "Low"}, {ID: "high", Name: "High"}}},
	}}
}

// --- Scenario A: mention → investigate → post_question → thread reply → materialize ---
func TestLifecycle_DraftFlow_InvestigateQuestionResumeMaterialize(t *testing.T) {
	const channelID = "C-LIFE-A"
	const mentionTS = "1700000010.000000"
	const replyTS = "1700000020.000000"
	registry := newRegistryWithSchema("ws-1", "WS-1", schemaWithSeverity())

	llm := newScriptedPlannerLLM(t, []string{
		// Round 1 (mention): investigate one task.
		`{
            "reasoning": "need surrounding context first",
            "action": "investigate",
            "investigate": {
                "message": "Looking at the thread",
                "tasks": [{"id":"inv-1","title":"thread scan","description":"scan thread","acceptance_criteria":"got summary","tools":["slack_ro"]}]
            }
        }`,
		// Round 2 (after observation): ask the user.
		`{
            "reasoning": "still missing severity",
            "action": "question",
            "question": {
                "reason": "need severity to fill the schema",
                "items": [{"id":"q-sev","text":"What is the severity?","type":"select","options":["low","high"]}]
            }
        }`,
		// Round 3 (after thread reply): materialize.
		`{
            "reasoning": "user said high",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-1",
                "title": "Outage X",
                "description": "Service degraded since morning.",
                "custom_field_values": {"severity": "high"}
            }
        }`,
	}, map[string]string{
		"scan thread": "summary: all messages mention an outage but never name a severity.",
	})

	h := newLifecycleHarness(t, registry, llm)

	// --- Turn 1: app_mention drives investigate → post_question.
	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U1", "<@BOT> case please", mentionTS))).Required()
	async.Wait()

	// Session is persisted with LastAction = post_question so the dispatcher
	// will treat the next thread reply as a resume signal. The pending
	// question snapshot is the canonical record of what was asked — assert
	// against it rather than parsing rendered Slack blocks.
	ssn1, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn1).NotNil().Required()
	gt.Value(t, ssn1.LastAction).Equal(model.SessionEndedWithQuestion)
	gt.Value(t, ssn1.PendingQuestion).NotNil().Required()
	gt.Array(t, ssn1.PendingQuestion.Items).Length(1).Required()
	gt.Value(t, ssn1.PendingQuestion.Items[0].ID).Equal("q-sev")
	gt.String(t, ssn1.PendingQuestion.Items[0].Text).Contains("severity")
	gt.Array(t, ssn1.PendingQuestion.Items[0].Options).Length(2)
	gt.Value(t, ssn1.PendingQuestion.Items[0].Options[0]).Equal("low")
	gt.Value(t, ssn1.PendingQuestion.Items[0].Options[1]).Equal("high")

	// --- Turn 2: user replies in-thread without mentioning the bot.
	reply := &slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Channel: channelID, User: "U1",
				Text:            "high",
				TimeStamp:       replyTS,
				ThreadTimeStamp: mentionTS,
			},
		},
	}
	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(), reply)).Required()
	async.Wait()

	// Materialization persisted on the draft.
	ssn2, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn2.LastAction).Equal(model.SessionEndedWithMaterialize)
	gt.Value(t, ssn2.DraftID).NotEqual(model.CaseDraftID(""))

	d, err := h.repo.CaseDraft().Get(context.Background(), ssn2.DraftID)
	gt.NoError(t, err).Required()
	gt.Value(t, d).NotNil().Required()
	gt.Value(t, d.Materialization).NotNil().Required()
	gt.Value(t, d.Materialization.Title).Equal("Outage X")
	gt.Value(t, d.Materialization.CustomFieldValues["severity"].Value).Equal("high")
}

// --- Scenario F: mention → question → Submit-button drives the resume ---
func TestLifecycle_DraftFlow_QuestionFormSubmitResumesPlanner(t *testing.T) {
	const channelID = "C-LIFE-F"
	const mentionTS = "1700000010.000000"
	const formTS = "ts-thread"
	const submitTS = "1700000020.000000"
	registry := newRegistryWithSchema("ws-1", "WS-1", schemaWithSeverity())

	llm := newScriptedPlannerLLM(t, []string{
		// Round 1 (mention): ask the user.
		`{
            "reasoning": "need severity to fill the schema",
            "action": "question",
            "question": {
                "reason": "need severity",
                "items": [{"id":"q-sev","text":"What is the severity?","type":"select","options":["low","high"]}]
            }
        }`,
		// Round 2 (after Submit): materialize.
		`{
            "reasoning": "user said high",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-1",
                "title": "Outage F",
                "description": "Service degraded.",
                "custom_field_values": {"severity": "high"}
            }
        }`,
	}, nil)

	h := newLifecycleHarness(t, registry, llm)

	// --- Turn 1: mention → planner emits question.
	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U1", "<@BOT> case please", mentionTS))).Required()
	async.Wait()

	ssn1, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn1).NotNil().Required()
	gt.Value(t, ssn1.LastAction).Equal(model.SessionEndedWithQuestion)
	gt.Value(t, ssn1.PendingQuestion).NotNil().Required()
	gt.Value(t, ssn1.PendingQuestion.PostedMessageTS).Equal(formTS)

	// --- Turn 2: user clicks Submit on the form.
	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U1"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channelID}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: mentionTS}},
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
					usecase.ActionIDDraftQuestionChoice: {
						SelectedOption: goslack.OptionBlockObject{Value: "high"},
					},
				},
			},
		},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{ActionID: usecase.ActionIDDraftQuestionSubmit, Value: string(ssn1.DraftID)},
			},
		},
	}
	_ = submitTS // reserved for future per-submission ts attribution
	gt.NoError(t, h.mentionDraft.HandleQuestionSubmit(context.Background(), cb,
		cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	// PendingQuestion is cleared and the planner advanced to materialize.
	ssn2, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn2.LastAction).Equal(model.SessionEndedWithMaterialize)
	gt.Value(t, ssn2.PendingQuestion).Nil()

	// Form was rewritten into the answered view (one UpdateMessage just for
	// the form swap; further updates may follow from the materialize path).
	gt.Number(t, len(h.slackMock.updateBlockPosts)).GreaterOrEqual(1)

	// Materialization landed with the user's answer baked into custom fields.
	d, err := h.repo.CaseDraft().Get(context.Background(), ssn2.DraftID)
	gt.NoError(t, err).Required()
	gt.Value(t, d).NotNil().Required()
	gt.Value(t, d.Materialization).NotNil().Required()
	gt.Value(t, d.Materialization.Title).Equal("Outage F")
	gt.Value(t, d.Materialization.CustomFieldValues["severity"].Value).Equal("high")
}

// --- Scenario B: mention → materialize → ws-switch → re-materialize ---
func TestLifecycle_DraftFlow_MaterializeThenWorkspaceSwitch(t *testing.T) {
	const channelID = "C-LIFE-B"
	const mentionTS = "1700000010.000000"
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-A", Name: "WS-A"}, FieldSchema: schemaWithSeverity(),
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-B", Name: "WS-B"},
		FieldSchema: &config.FieldSchema{Fields: []config.FieldDefinition{
			{ID: "team", Type: types.FieldTypeText, Required: true},
		}},
	})

	llm := newScriptedPlannerLLM(t, []string{
		// Turn 1 (mention) → materialize for ws-A.
		`{
            "reasoning": "first guess workspace",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-A",
                "title": "Issue title",
                "description": "Initial description",
                "custom_field_values": {"severity": "low"}
            }
        }`,
		// Turn 2 (ws-switch) → re-materialize for ws-B with team field.
		`{
            "reasoning": "rebuild for ws-B schema",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-B",
                "title": "Issue title",
                "description": "Initial description",
                "custom_field_values": {"team": "platform"}
            }
        }`,
	}, nil)

	h := newLifecycleHarness(t, registry, llm)

	// Turn 1: mention.
	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U1", "<@BOT> please", mentionTS))).Required()
	async.Wait()

	ssn, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	d1, err := h.repo.CaseDraft().Get(context.Background(), ssn.DraftID)
	gt.NoError(t, err).Required()
	gt.Value(t, d1.SelectedWorkspaceID).Equal("ws-A")
	gt.Value(t, d1.Materialization.CustomFieldValues["severity"].Value).Equal("low")

	// Turn 2: workspace switch via the preview's static_select.
	cb := &goslack.InteractionCallback{
		Type:        goslack.InteractionTypeBlockActions,
		ResponseURL: "http://example.invalid/responseurl",
		User:        goslack.User{ID: "U1"},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{
					ActionID:       usecase.ActionIDDraftSelectWS,
					BlockID:        usecase.BlockIDDraftWSSelect + ":" + string(d1.ID),
					Value:          string(d1.ID),
					SelectedOption: goslack.OptionBlockObject{Value: "ws-B"},
				},
			},
		},
	}
	// The handler POSTs to ResponseURL — we use a local httptest server so
	// the lock-blocks render call succeeds (real HTTP would surface as a
	// non-fatal errutil.Handle but doesn't block the planner turn).
	respURL, _ := captureResponseURL(t)
	cb.ResponseURL = respURL
	wsErr := h.mentionDraft.HandleSelectWorkspace(context.Background(), cb, cb.ActionCallback.BlockActions[0])
	async.Wait()
	gt.NoError(t, wsErr).Required()

	d2, err := h.repo.CaseDraft().Get(context.Background(), ssn.DraftID)
	gt.NoError(t, err).Required()
	gt.Value(t, d2.SelectedWorkspaceID).Equal("ws-B")
	gt.Value(t, d2.Materialization).NotNil().Required()
	gt.Value(t, d2.Materialization.CustomFieldValues["team"].Value).Equal("platform")
	// The old severity field is no longer schema-relevant; the coercion
	// drops fields outside the active schema.
	_, hasSeverity := d2.Materialization.CustomFieldValues["severity"]
	gt.Bool(t, hasSeverity).False()
}

// --- Scenario C: mention → 2 parallel investigations → materialize ---
func TestLifecycle_DraftFlow_ParallelInvestigationsThenMaterialize(t *testing.T) {
	const channelID = "C-LIFE-C"
	const mentionTS = "1700000030.000000"
	registry := newRegistryWithSchema("ws-1", "WS-1", schemaWithSeverity())

	llm := newScriptedPlannerLLM(t, []string{
		`{
            "reasoning": "fan out two investigations",
            "action": "investigate",
            "investigate": {
                "message": "Looking up two angles",
                "tasks": [
                    {"id":"inv-A","title":"thread","description":"scan thread A","acceptance_criteria":"a","tools":["slack_ro"]},
                    {"id":"inv-B","title":"channel","description":"scan channel B","acceptance_criteria":"b","tools":["slack_ro"]}
                ]
            }
        }`,
		`{
            "reasoning": "got both observations",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-1",
                "title": "Combined finding",
                "description": "From thread + channel.",
                "custom_field_values": {"severity": "high"}
            }
        }`,
	}, map[string]string{
		"scan thread A":  "summary A: high signal",
		"scan channel B": "summary B: confirms",
	})

	h := newLifecycleHarness(t, registry, llm)

	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U1", "<@BOT> please", mentionTS))).Required()
	async.Wait()

	ssn, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.LastAction).Equal(model.SessionEndedWithMaterialize)

	d, err := h.repo.CaseDraft().Get(context.Background(), ssn.DraftID)
	gt.NoError(t, err).Required()
	gt.Value(t, d.Materialization.Title).Equal("Combined finding")
	gt.Value(t, d.Materialization.CustomFieldValues["severity"].Value).Equal("high")
}

// --- Scenario D: materialize terminal → thread reply must NOT resume (F8) ---
//
// Once the planner has produced a draft preview the conversation is over
// from the agent's perspective; further thread chatter without an explicit
// @mention should be ignored. Dispatcher F8 enforces this by checking
// session.ResumeOnReply() (true only when LastAction == post_question).
func TestLifecycle_DraftFlow_MaterializeEndsThenReplyIsDropped(t *testing.T) {
	const channelID = "C-LIFE-D"
	const mentionTS = "1700000040.000000"
	const replyTS = "1700000050.000000"
	registry := newRegistryWithSchema("ws-1", "WS-1", schemaWithSeverity())

	// Only ONE planner response is scripted. The dispatcher must drop the
	// follow-up MessageEvent (F8: LastAction != post_question) so the
	// planner is not re-invoked. If the dispatch leaks, the script
	// exhausts and the test fails.
	llm := newScriptedPlannerLLM(t, []string{
		`{
            "reasoning": "materialize directly",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-1",
                "title": "Case D",
                "description": "Done.",
                "custom_field_values": {"severity": "low"}
            }
        }`,
	}, nil)

	h := newLifecycleHarness(t, registry, llm)

	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U1", "<@BOT> hey", mentionTS))).Required()
	async.Wait()

	ssn, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn.LastAction).Equal(model.SessionEndedWithMaterialize)

	// Thread reply: F8 must drop. No additional LLM calls.
	reply := &slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Channel: channelID, User: "U1",
				Text:            "are you sure?",
				TimeStamp:       replyTS,
				ThreadTimeStamp: mentionTS,
			},
		},
	}
	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(), reply)).Required()
	async.Wait()

	// Session unchanged; LastAction still materialize.
	ssn2, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn2.LastAction).Equal(model.SessionEndedWithMaterialize)
}

// --- Scenario E: mention → materialize → HandleSubmit creates the Case ---
func TestLifecycle_DraftFlow_MaterializeThenSubmitCreatesCase(t *testing.T) {
	const channelID = "C-LIFE-E"
	const mentionTS = "1700000060.000000"
	registry := newRegistryWithSchema("ws-1", "WS-1", schemaWithSeverity())

	llm := newScriptedPlannerLLM(t, []string{
		`{
            "reasoning": "materialize directly",
            "action": "materialize",
            "materialize": {
                "workspace_id": "ws-1",
                "title": "Quick incident",
                "description": "Something broke briefly.",
                "custom_field_values": {"severity": "high"}
            }
        }`,
	}, nil)

	h := newLifecycleHarness(t, registry, llm)

	gt.NoError(t, h.slackUC.HandleSlackEvent(context.Background(),
		appMentionEvent(channelID, "U-AUTHOR", "<@BOT> case", mentionTS))).Required()
	async.Wait()

	ssn, err := h.repo.Session().GetByThread(context.Background(), channelID, mentionTS)
	gt.NoError(t, err).Required()
	d, err := h.repo.CaseDraft().Get(context.Background(), ssn.DraftID)
	gt.NoError(t, err).Required()

	// Submit via the preview's button — drives CreateCase end-to-end.
	respURL, _ := captureResponseURL(t)
	cb := &goslack.InteractionCallback{
		Type:        goslack.InteractionTypeBlockActions,
		ResponseURL: respURL,
		User:        goslack.User{ID: "U-AUTHOR"},
		Team:        goslack.Team{ID: "T1"},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{ActionID: usecase.ActionIDDraftSubmit, Value: string(d.ID)},
			},
		},
	}
	gt.NoError(t, h.mentionDraft.HandleSubmit(context.Background(), h.caseUC, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	// One case persisted with the materialized title and field value.
	cases, err := h.repo.Case().List(context.Background(), "ws-1")
	gt.NoError(t, err).Required()
	gt.Array(t, cases).Length(1).Required()
	gt.Value(t, cases[0].Title).Equal("Quick incident")
	gt.Value(t, cases[0].FieldValues["severity"].Value).Equal("high")
	gt.Array(t, cases[0].AssigneeIDs).Length(1).Required()
	gt.Value(t, cases[0].AssigneeIDs[0]).Equal("U-AUTHOR")
	gt.Value(t, cases[0].SlackChannelID).Equal("C-CREATED")

	// The post-create chat.update replaces the preview with a single
	// context block carrying a clickable mention of the case channel —
	// not a full re-render of the case body.
	gt.Number(t, len(h.slackMock.updateBlockPosts)).GreaterOrEqual(1).Required()
	finalUpdate := h.slackMock.updateBlockPosts[len(h.slackMock.updateBlockPosts)-1]
	gt.Array(t, finalUpdate.rawBlocks).Length(1).Required()
	finalCtx, ok := finalUpdate.rawBlocks[0].(*goslack.ContextBlock)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, finalCtx.ContextElements.Elements).Length(1).Required()
	finalText, ok := finalCtx.ContextElements.Elements[0].(*goslack.TextBlockObject)
	gt.Bool(t, ok).True().Required()
	gt.Bool(t, strings.Contains(finalText.Text, "<#C-CREATED>")).True()
	gt.Bool(t, strings.Contains(finalText.Text, "Quick incident")).True()

	// Draft is deleted after Submit.
	_, err = h.repo.CaseDraft().Get(context.Background(), d.ID)
	gt.Value(t, err).NotNil()
}

// TestBuildCaseCreatedTailBlocks verifies that the post-create thread
// message renders as a single context block carrying a Slack channel
// mention that links the user to the case's dedicated channel.
func TestBuildCaseCreatedTailBlocks(t *testing.T) {
	t.Run("with SlackChannelID renders mrkdwn channel mention", func(t *testing.T) {
		created := &model.Case{
			ID:             42,
			Title:          "Tanaka incident",
			SlackChannelID: "C0123ABCD",
		}
		blocks, fallback := usecase.BuildCaseCreatedTailBlocksForTest(context.Background(), created)
		gt.Array(t, blocks).Length(1).Required()
		gt.String(t, fallback).Contains("42")
		gt.String(t, fallback).Contains("Tanaka incident")

		ctxBlock, ok := blocks[0].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		gt.Value(t, ctxBlock.Type).Equal(goslack.MBTContext)
		gt.Array(t, ctxBlock.ContextElements.Elements).Length(1).Required()

		text, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		gt.Value(t, text.Type).Equal(goslack.MarkdownType)
		gt.String(t, text.Text).Contains("<#C0123ABCD>")
		gt.String(t, text.Text).Contains("42")
		gt.String(t, text.Text).Contains("Tanaka incident")
	})

	t.Run("without SlackChannelID falls back to plain created line", func(t *testing.T) {
		created := &model.Case{
			ID:    7,
			Title: "Solo incident",
		}
		blocks, _ := usecase.BuildCaseCreatedTailBlocksForTest(context.Background(), created)
		gt.Array(t, blocks).Length(1).Required()

		ctxBlock, ok := blocks[0].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		text, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		gt.String(t, text.Text).Contains("7")
		gt.String(t, text.Text).Contains("Solo incident")
		// No channel mention when SlackChannelID is empty.
		gt.Bool(t, strings.Contains(text.Text, "<#")).False()
	})

	t.Run("nil case returns no blocks and a fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildCaseCreatedTailBlocksForTest(context.Background(), nil)
		gt.Array(t, blocks).Length(0)
		gt.String(t, fallback).NotEqual("")
	})

	t.Run("escapes markdown characters in title", func(t *testing.T) {
		// Title contains characters (`*`, `_`, `~`, backtick) that would
		// otherwise break the surrounding `*%s*` markdown slot.
		created := &model.Case{
			ID:             3,
			Title:          "*bold* _italic_ ~strike~ `code`",
			SlackChannelID: "C-X",
		}
		blocks, _ := usecase.BuildCaseCreatedTailBlocksForTest(context.Background(), created)
		gt.Array(t, blocks).Length(1).Required()
		ctxBlock, ok := blocks[0].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		text, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		// Original markdown control chars are no longer present unescaped.
		// (escapeMarkdownInline prefixes them with `\` or strips them; the
		// exact escape form is its concern, but the raw characters must
		// not survive in a way that produces nested bold/italic spans.)
		gt.Bool(t, strings.Contains(text.Text, "*bold*")).False()
		gt.Bool(t, strings.Contains(text.Text, "_italic_")).False()
	})

	t.Run("empty title falls back to (untitled) placeholder", func(t *testing.T) {
		created := &model.Case{
			ID:             4,
			Title:          "   ",
			SlackChannelID: "C-Y",
		}
		blocks, fallback := usecase.BuildCaseCreatedTailBlocksForTest(context.Background(), created)
		gt.Array(t, blocks).Length(1).Required()
		ctxBlock, ok := blocks[0].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		text, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		gt.String(t, text.Text).Contains("(untitled)")
		gt.String(t, fallback).Contains("(untitled)")
	})
}
