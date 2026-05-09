package usecase_test

import (
	"context"
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
	return "", nil
}
func (m *collectorOnlyMockSlack) GetConversationMembers(context.Context, string) ([]string, error) {
	return nil, nil
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
	})
	return nil
}
func (m *collectorOnlyMockSlack) PostThreadReply(context.Context, string, string, string) (string, error) {
	return "", nil
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
