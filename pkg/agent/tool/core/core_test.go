package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// mockActionMutator captures CreateAction and UpdateAction calls so the
// create / update / set-assignee / status tool tests can verify the tools
// route through the unified ActionUseCase entry point. The real
// ActionUseCase (and its Slack post / refresh / event recording) is
// exercised by usecase-level tests; here we only check the tool's
// argument-extraction behaviour.
type mockActionMutator struct {
	createFn func(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error)
	updateFn func(ctx context.Context, workspaceID string, actionID int64, params core.UpdateActionParams) (*model.Action, error)
}

func (m *mockActionMutator) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error) {
	if m.createFn != nil {
		return m.createFn(ctx, workspaceID, caseID, title, description, assigneeID, slackMessageTS, status, dueDate)
	}
	return &model.Action{
		ID: 1, CaseID: caseID, Title: title, Description: description,
		AssigneeID: assigneeID, SlackMessageTS: slackMessageTS,
		Status: status, DueDate: dueDate,
	}, nil
}

func (m *mockActionMutator) UpdateAction(ctx context.Context, workspaceID string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, workspaceID, actionID, params)
	}
	// Default: synthesize a minimal Action reflecting the requested edits.
	// Callers that need to assert on persisted state should provide their
	// own updateFn.
	a := &model.Action{ID: actionID}
	if params.Title != nil {
		a.Title = *params.Title
	}
	if params.Description != nil {
		a.Description = *params.Description
	}
	if params.AssigneeID != nil {
		a.AssigneeID = *params.AssigneeID
	}
	if params.Status != nil {
		a.Status = *params.Status
	}
	if params.DueDate != nil {
		a.DueDate = params.DueDate
	}
	return a, nil
}

// newCtxWithUpdateCapture returns a context that captures all update messages
// and a pointer to the slice where they are appended.
func newCtxWithUpdateCapture() (context.Context, *[]string) {
	var messages []string
	ctx := tool.WithUpdate(context.Background(), func(_ context.Context, msg string) {
		messages = append(messages, msg)
	})
	return ctx, &messages
}

const (
	testWorkspaceID = "ws-tool-test"
	testCaseID      = int64(100)
)

// ----- mock LLM client -----

type mockLLMClient struct {
	generateEmbeddingFn func(ctx context.Context, dimension int, input []string) ([][]float64, error)
}

func (m *mockLLMClient) NewSession(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error) {
	return nil, nil
}

func (m *mockLLMClient) GenerateEmbedding(ctx context.Context, dimension int, input []string) ([][]float64, error) {
	if m.generateEmbeddingFn != nil {
		return m.generateEmbeddingFn(ctx, dimension, input)
	}
	vec := make([]float64, dimension)
	for i := range vec {
		vec[i] = 0.1
	}
	return [][]float64{vec}, nil
}

// ----- mock ActionRepository -----

type mockActionRepo struct {
	createFn     func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)
	getFn        func(ctx context.Context, workspaceID string, id int64) (*model.Action, error)
	listFn       func(ctx context.Context, workspaceID string) ([]*model.Action, error)
	updateFn     func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)
	deleteFn     func(ctx context.Context, workspaceID string, id int64) error
	getByCaseFn  func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error)
	getByCasesFn func(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Action, error)
}

func (m *mockActionRepo) Create(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	if m.createFn != nil {
		return m.createFn(ctx, workspaceID, action)
	}
	created := *action
	created.ID = 1
	return &created, nil
}

func (m *mockActionRepo) Get(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
	if m.getFn != nil {
		return m.getFn(ctx, workspaceID, id)
	}
	return nil, errors.New("not found")
}

func (m *mockActionRepo) List(ctx context.Context, workspaceID string) ([]*model.Action, error) {
	if m.listFn != nil {
		return m.listFn(ctx, workspaceID)
	}
	return nil, nil
}

func (m *mockActionRepo) Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, workspaceID, action)
	}
	return action, nil
}

func (m *mockActionRepo) Delete(ctx context.Context, workspaceID string, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, workspaceID, id)
	}
	return nil
}

func (m *mockActionRepo) GetByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	if m.getByCaseFn != nil {
		return m.getByCaseFn(ctx, workspaceID, caseID)
	}
	return nil, nil
}

func (m *mockActionRepo) GetByCases(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Action, error) {
	if m.getByCasesFn != nil {
		return m.getByCasesFn(ctx, workspaceID, caseIDs)
	}
	return nil, nil
}

func (m *mockActionRepo) GetBySlackMessageTS(ctx context.Context, workspaceID string, ts string) (*model.Action, error) {
	return nil, nil
}

// ----- mock KnowledgeRepository -----

type mockKnowledgeRepo struct {
	createFn             func(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error)
	getFn                func(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error)
	listByCaseIDFn       func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Knowledge, error)
	listByCaseIDsFn      func(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Knowledge, error)
	listBySourceIDFn     func(ctx context.Context, workspaceID string, sourceID model.SourceID) ([]*model.Knowledge, error)
	listWithPaginationFn func(ctx context.Context, workspaceID string, limit, offset int) ([]*model.Knowledge, int, error)
	deleteFn             func(ctx context.Context, workspaceID string, id model.KnowledgeID) error
	findByEmbeddingFn    func(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error)
}

func (m *mockKnowledgeRepo) Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
	if m.createFn != nil {
		return m.createFn(ctx, workspaceID, knowledge)
	}
	return knowledge, nil
}

func (m *mockKnowledgeRepo) Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	if m.getFn != nil {
		return m.getFn(ctx, workspaceID, id)
	}
	return nil, errors.New("not found")
}

func (m *mockKnowledgeRepo) ListByCaseID(ctx context.Context, workspaceID string, caseID int64) ([]*model.Knowledge, error) {
	if m.listByCaseIDFn != nil {
		return m.listByCaseIDFn(ctx, workspaceID, caseID)
	}
	return nil, nil
}

func (m *mockKnowledgeRepo) ListByCaseIDs(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	if m.listByCaseIDsFn != nil {
		return m.listByCaseIDsFn(ctx, workspaceID, caseIDs)
	}
	return nil, nil
}

func (m *mockKnowledgeRepo) ListBySourceID(ctx context.Context, workspaceID string, sourceID model.SourceID) ([]*model.Knowledge, error) {
	if m.listBySourceIDFn != nil {
		return m.listBySourceIDFn(ctx, workspaceID, sourceID)
	}
	return nil, nil
}

func (m *mockKnowledgeRepo) ListWithPagination(ctx context.Context, workspaceID string, limit, offset int) ([]*model.Knowledge, int, error) {
	if m.listWithPaginationFn != nil {
		return m.listWithPaginationFn(ctx, workspaceID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockKnowledgeRepo) Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, workspaceID, id)
	}
	return nil
}

func (m *mockKnowledgeRepo) FindByEmbedding(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error) {
	if m.findByEmbeddingFn != nil {
		return m.findByEmbeddingFn(ctx, workspaceID, embedding, limit)
	}
	return nil, nil
}

// ----- mock Repository -----

type mockRepo struct {
	actionRepo    interfaces.ActionRepository
	knowledgeRepo interfaces.KnowledgeRepository
}

func (m *mockRepo) Action() interfaces.ActionRepository       { return m.actionRepo }
func (m *mockRepo) Knowledge() interfaces.KnowledgeRepository { return m.knowledgeRepo }

// Unused methods — panic to catch accidental calls in tests
func (m *mockRepo) Case() interfaces.CaseRepository           { panic("unexpected call: Case()") }
func (m *mockRepo) Slack() interfaces.SlackRepository         { panic("unexpected call: Slack()") }
func (m *mockRepo) SlackUser() interfaces.SlackUserRepository { panic("unexpected call: SlackUser()") }
func (m *mockRepo) Source() interfaces.SourceRepository       { panic("unexpected call: Source()") }
func (m *mockRepo) CaseMessage() interfaces.CaseMessageRepository {
	panic("unexpected call: CaseMessage()")
}
func (m *mockRepo) ActionMessage() interfaces.ActionMessageRepository {
	panic("unexpected call: ActionMessage()")
}
func (m *mockRepo) ActionEvent() interfaces.ActionEventRepository {
	panic("unexpected call: ActionEvent()")
}
func (m *mockRepo) PutToken(ctx context.Context, token *auth.Token) error {
	panic("unexpected call: PutToken()")
}
func (m *mockRepo) GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error) {
	panic("unexpected call: GetToken()")
}
func (m *mockRepo) DeleteToken(ctx context.Context, tokenID auth.TokenID) error {
	panic("unexpected call: DeleteToken()")
}
func (m *mockRepo) Memory() interfaces.MemoryRepository {
	panic("unexpected call: Memory()")
}
func (m *mockRepo) AssistLog() interfaces.AssistLogRepository {
	panic("unexpected call: AssistLog()")
}
func (m *mockRepo) CaseDraft() interfaces.CaseDraftRepository {
	panic("unexpected call: CaseDraft()")
}
func (m *mockRepo) AgentSession() interfaces.AgentSessionRepository {
	panic("unexpected call: AgentSession()")
}
func (m *mockRepo) Close() error { return nil }

// newMockRepo builds a mockRepo with default no-op sub-repos
func newMockRepo(actionRepo interfaces.ActionRepository, knowledgeRepo interfaces.KnowledgeRepository) *mockRepo {
	if actionRepo == nil {
		actionRepo = &mockActionRepo{}
	}
	if knowledgeRepo == nil {
		knowledgeRepo = &mockKnowledgeRepo{}
	}
	return &mockRepo{actionRepo: actionRepo, knowledgeRepo: knowledgeRepo}
}

// findTool returns the tool with the given name from the list
func findTool(tools []gollem.Tool, name string) gollem.Tool {
	for _, t := range tools {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// ----- tests -----

func TestNew_ReturnsEightTools(t *testing.T) {
	repo := newMockRepo(nil, nil)
	llm := &mockLLMClient{}
	tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: llm})
	gt.Array(t, tools).Length(8)
}

func TestListActionsTool(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty list when repository returns no actions", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getByCaseFn: func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
				gt.Value(t, workspaceID).Equal(testWorkspaceID)
				gt.Value(t, caseID).Equal(testCaseID)
				return []*model.Action{}, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__list_actions").Run(ctx, map[string]any{})
		gt.NoError(t, err)
		items := result["actions"].([]map[string]any)
		gt.Array(t, items).Length(0)
	})

	t.Run("returns actions from repository", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getByCaseFn: func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
				return []*model.Action{
					{ID: 1, CaseID: caseID, Title: "Fix bug", Status: types.ActionStatusTodo, AssigneeID: "U001"},
					{ID: 2, CaseID: caseID, Title: "Write docs", Status: types.ActionStatusCompleted},
				}, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__list_actions").Run(ctx, map[string]any{})
		gt.NoError(t, err)
		items := result["actions"].([]map[string]any)
		gt.Array(t, items).Length(2)
		gt.Value(t, items[0]["title"]).Equal("Fix bug")
		gt.Value(t, items[1]["title"]).Equal("Write docs")
	})

	t.Run("propagates repository error", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getByCaseFn: func(_ context.Context, _ string, _ int64) ([]*model.Action, error) {
				return nil, errors.New("database unavailable")
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__list_actions").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})
}

func TestGetActionTool(t *testing.T) {
	ctx := context.Background()

	t.Run("passes correct workspace and action ID to repository", func(t *testing.T) {
		var gotWorkspaceID string
		var gotActionID int64
		actionRepo := &mockActionRepo{
			getFn: func(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
				gotWorkspaceID = workspaceID
				gotActionID = id
				return &model.Action{ID: id, CaseID: testCaseID, Title: "My action", Status: types.ActionStatusInProgress}, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(42)})
		gt.NoError(t, err)
		gt.Value(t, gotWorkspaceID).Equal(testWorkspaceID)
		gt.Value(t, gotActionID).Equal(int64(42))
		gt.Value(t, result["title"]).Equal("My action")
		gt.Value(t, result["status"]).Equal("IN_PROGRESS")
	})

	t.Run("returns error when repository returns error", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, id int64) (*model.Action, error) {
				return nil, errors.New("not found")
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(999)})
		gt.Error(t, err)
	})

	t.Run("returns error when action_id is missing", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})
}

func TestCreateActionTool(t *testing.T) {
	ctx := context.Background()

	t.Run("creates action with correct fields", func(t *testing.T) {
		var (
			capturedWorkspaceID string
			capturedCaseID      int64
			capturedTitle       string
			capturedDescription string
			capturedAssigneeID  string
			capturedStatus      types.ActionStatus
		)
		creator := &mockActionMutator{
			createFn: func(_ context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, _ string, status types.ActionStatus, _ *time.Time) (*model.Action, error) {
				capturedWorkspaceID = workspaceID
				capturedCaseID = caseID
				capturedTitle = title
				capturedDescription = description
				capturedAssigneeID = assigneeID
				capturedStatus = status
				return &model.Action{ID: 10, CaseID: caseID, Title: title, Description: description, AssigneeID: assigneeID, Status: status}, nil
			},
		}
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: creator})

		result, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":       "New investigation",
			"description": "Look into the alerts",
			"status":      "IN_PROGRESS",
			"assignee_id": "U001",
		})
		gt.NoError(t, err)
		gt.Value(t, capturedWorkspaceID).Equal(testWorkspaceID)
		gt.Value(t, capturedCaseID).Equal(testCaseID)
		gt.Value(t, capturedTitle).Equal("New investigation")
		gt.Value(t, capturedDescription).Equal("Look into the alerts")
		gt.Value(t, capturedStatus).Equal(types.ActionStatusInProgress)
		gt.Value(t, capturedAssigneeID).Equal("U001")
		gt.Value(t, result["id"]).Equal(int64(10))
	})

	t.Run("defaults status to TODO when omitted", func(t *testing.T) {
		var capturedStatus types.ActionStatus
		creator := &mockActionMutator{
			createFn: func(_ context.Context, _ string, caseID int64, title, description string, assigneeID string, _ string, status types.ActionStatus, _ *time.Time) (*model.Action, error) {
				capturedStatus = status
				return &model.Action{ID: 1, CaseID: caseID, Title: title, Description: description, AssigneeID: assigneeID, Status: status}, nil
			},
		}
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: creator})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{"title": "Quick task"})
		gt.NoError(t, err)
		// With no statusSet override, the tool falls back to
		// model.DefaultActionStatusSet() whose initial id is BACKLOG.
		gt.Value(t, capturedStatus).Equal(types.ActionStatusBacklog)
	})

	t.Run("returns error when title is missing", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":  "Test",
			"status": "FLYING",
		})
		gt.Error(t, err)
	})

	t.Run("returns error when assignee_ids contains non-string element", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":       "Test",
			"assignee_id": 42,
		})
		gt.Error(t, err)
	})

	t.Run("returns error when ActionUC dependency is nil", func(t *testing.T) {
		// Guards against silent regression to the legacy repository-direct
		// path: if Deps.ActionUC is forgotten by a future caller, the tool
		// should fail loudly rather than skip Slack posting.
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title": "Should not reach repo",
		})
		gt.Error(t, err)
	})
}

func TestUpdateActionTool(t *testing.T) {
	ctx := context.Background()

	// captureUpdateMutator returns a mutator whose UpdateAction stores the
	// invocation arguments into the supplied pointers. Tests then assert on
	// what was captured rather than reaching for repository.Update.
	captureUpdateMutator := func(capID *int64, capParams *core.UpdateActionParams) *mockActionMutator {
		return &mockActionMutator{
			updateFn: func(_ context.Context, _ string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
				*capID = actionID
				*capParams = params
				return &model.Action{ID: actionID}, nil
			},
		}
	}

	t.Run("forwards title and description as pointer fields", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(5),
			"title":       "New title",
			"description": "New description",
		})
		gt.NoError(t, err)
		gt.Value(t, capID).Equal(int64(5))
		gt.Value(t, capParams.Title).NotNil().Required()
		gt.Value(t, *capParams.Title).Equal("New title")
		gt.Value(t, capParams.Description).NotNil().Required()
		gt.Value(t, *capParams.Description).Equal("New description")
		// Assignee fields untouched: tool must signal "no change", not
		// accidentally clear or reassign.
		gt.Value(t, capParams.AssigneeID).Nil()
		gt.Bool(t, capParams.ClearAssignee).False()
	})

	t.Run("forwards non-empty assignee_id as AssigneeID", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(6),
			"assignee_id": "U003",
		})
		gt.NoError(t, err)
		gt.Value(t, capParams.AssigneeID).NotNil().Required()
		gt.Value(t, *capParams.AssigneeID).Equal("U003")
		gt.Bool(t, capParams.ClearAssignee).False()
	})

	t.Run("translates empty assignee_id into ClearAssignee=true", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(60),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		// AssigneeID stays nil — the explicit clear flag is what the
		// usecase reads.
		gt.Value(t, capParams.AssigneeID).Nil()
		gt.Bool(t, capParams.ClearAssignee).True()
	})

	t.Run("translates empty due_date into ClearDueDate=true", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id": float64(61),
			"due_date":  "",
		})
		gt.NoError(t, err)
		gt.Value(t, capParams.DueDate).Nil()
		gt.Bool(t, capParams.ClearDueDate).True()
	})

	t.Run("treats empty description as no-change", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(7),
			"description": "",
		})
		gt.NoError(t, err)
		// No-change semantics: pointer stays nil so usecase keeps the
		// stored description.
		gt.Value(t, capParams.Description).Nil()
	})

	t.Run("omits unspecified fields entirely", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := captureUpdateMutator(&capID, &capParams)
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		desc := "Added desc"
		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(8),
			"description": desc,
		})
		gt.NoError(t, err)
		gt.Value(t, capParams.Title).Nil()
		gt.Value(t, capParams.AssigneeID).Nil()
		gt.Value(t, capParams.Status).Nil()
		gt.Value(t, capParams.DueDate).Nil()
		gt.Bool(t, capParams.ClearAssignee).False()
		gt.Bool(t, capParams.ClearDueDate).False()
	})

	t.Run("returns error when action_id missing", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})
		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("propagates ActionUseCase error", func(t *testing.T) {
		mutator := &mockActionMutator{
			updateFn: func(_ context.Context, _ string, _ int64, _ core.UpdateActionParams) (*model.Action, error) {
				return nil, errors.New("not found")
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{"action_id": float64(999), "title": "X"})
		gt.Error(t, err)
	})

	t.Run("returns error when assignee_id is non-string", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})
		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(10),
			"assignee_id": 99,
		})
		gt.Error(t, err)
	})

	t.Run("returns error when ActionUC dependency is nil", func(t *testing.T) {
		// Same regression guard as create_action: nil ActionUC must error
		// loudly, not silently fall back to repo.Action().Update.
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id": float64(1),
			"title":     "Anything",
		})
		gt.Error(t, err)
	})
}

func TestUpdateActionStatusTool(t *testing.T) {
	ctx := context.Background()

	t.Run("forwards status as a pointer to ActionStatus", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := &mockActionMutator{
			updateFn: func(_ context.Context, _ string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
				capID = actionID
				capParams = params
				return &model.Action{ID: actionID, Status: *params.Status}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		result, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(5),
			"status":    "COMPLETED",
		})
		gt.NoError(t, err)
		gt.Value(t, capID).Equal(int64(5))
		gt.Value(t, capParams.Status).NotNil().Required()
		gt.Value(t, *capParams.Status).Equal(types.ActionStatusCompleted)
		// All other fields should be empty: status update must not
		// accidentally clear the assignee or zero the title.
		gt.Value(t, capParams.Title).Nil()
		gt.Value(t, capParams.AssigneeID).Nil()
		gt.Bool(t, capParams.ClearAssignee).False()
		gt.Value(t, result["status"]).Equal("COMPLETED")
	})

	t.Run("propagates ActionUseCase error", func(t *testing.T) {
		mutator := &mockActionMutator{
			updateFn: func(_ context.Context, _ string, _ int64, _ core.UpdateActionParams) (*model.Action, error) {
				return nil, errors.New("usecase error")
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "COMPLETED",
		})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status string", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "UNKNOWN_STATUS",
		})
		gt.Error(t, err)
	})

	t.Run("returns error when ActionUC dependency is nil", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "COMPLETED",
		})
		gt.Error(t, err)
	})
}

func TestSetActionAssigneeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("forwards non-empty assignee_id as AssigneeID", func(t *testing.T) {
		var capID int64
		var capParams core.UpdateActionParams
		mutator := &mockActionMutator{
			updateFn: func(_ context.Context, _ string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
				capID = actionID
				capParams = params
				return &model.Action{ID: actionID, AssigneeID: *params.AssigneeID}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(3),
			"assignee_id": "U002",
		})
		gt.NoError(t, err)
		gt.Value(t, capID).Equal(int64(3))
		gt.Value(t, capParams.AssigneeID).NotNil().Required()
		gt.Value(t, *capParams.AssigneeID).Equal("U002")
		gt.Bool(t, capParams.ClearAssignee).False()
	})

	t.Run("translates empty assignee_id into ClearAssignee=true", func(t *testing.T) {
		var capParams core.UpdateActionParams
		mutator := &mockActionMutator{
			updateFn: func(_ context.Context, _ string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
				capParams = params
				return &model.Action{ID: actionID}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(4),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		gt.Value(t, capParams.AssigneeID).Nil()
		gt.Bool(t, capParams.ClearAssignee).True()
	})

	t.Run("returns error when assignee_id is missing", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: &mockActionMutator{}})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id": float64(1),
		})
		gt.Error(t, err)
	})

	t.Run("returns error when ActionUC dependency is nil", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(1),
			"assignee_id": "U001",
		})
		gt.Error(t, err)
	})
}

func TestSearchKnowledgeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("generates embedding and calls FindByEmbedding with correct args", func(t *testing.T) {
		var gotQuery string
		var gotDimension int
		var gotLimit int

		knowledgeRepo := &mockKnowledgeRepo{
			findByEmbeddingFn: func(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error) {
				gt.Value(t, workspaceID).Equal(testWorkspaceID)
				gotLimit = limit
				return []*model.Knowledge{
					{ID: "k-001", Title: "Incident playbook", Summary: "Step by step guide"},
				}, nil
			},
		}
		llm := &mockLLMClient{
			generateEmbeddingFn: func(ctx context.Context, dimension int, input []string) ([][]float64, error) {
				gotDimension = dimension
				gotQuery = input[0]
				vec := make([]float64, dimension)
				for i := range vec {
					vec[i] = 0.5
				}
				return [][]float64{vec}, nil
			},
		}
		repo := newMockRepo(nil, knowledgeRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: llm})

		result, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{
			"query": "incident response",
			"limit": float64(3),
		})
		gt.NoError(t, err)
		gt.Value(t, gotQuery).Equal("incident response")
		gt.Value(t, gotDimension).Equal(model.EmbeddingDimension)
		gt.Value(t, gotLimit).Equal(3)
		items := result["knowledges"].([]map[string]any)
		gt.Array(t, items).Length(1)
		gt.Value(t, items[0]["title"]).Equal("Incident playbook")
		gt.Value(t, items[0]["summary"]).Equal("Step by step guide")
	})

	t.Run("uses default limit of 5 when omitted", func(t *testing.T) {
		var gotLimit int
		knowledgeRepo := &mockKnowledgeRepo{
			findByEmbeddingFn: func(_ context.Context, _ string, _ []float32, limit int) ([]*model.Knowledge, error) {
				gotLimit = limit
				return nil, nil
			},
		}
		repo := newMockRepo(nil, knowledgeRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": "test query"})
		gt.NoError(t, err)
		gt.Value(t, gotLimit).Equal(5)
	})

	t.Run("returns error when query is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": ""})
		gt.Error(t, err)
	})

	t.Run("propagates embedding generation error", func(t *testing.T) {
		llm := &mockLLMClient{
			generateEmbeddingFn: func(_ context.Context, _ int, _ []string) ([][]float64, error) {
				return nil, errors.New("embedding API down")
			},
		}
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: llm})

		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": "test"})
		gt.Error(t, err)
	})

	t.Run("propagates FindByEmbedding error", func(t *testing.T) {
		knowledgeRepo := &mockKnowledgeRepo{
			findByEmbeddingFn: func(_ context.Context, _ string, _ []float32, _ int) ([]*model.Knowledge, error) {
				return nil, errors.New("vector index unavailable")
			},
		}
		repo := newMockRepo(nil, knowledgeRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": "test"})
		gt.Error(t, err)
	})
}

func TestGetKnowledgeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("passes correct workspace and knowledge ID to repository", func(t *testing.T) {
		var gotWorkspaceID string
		var gotKnowledgeID model.KnowledgeID
		knowledgeRepo := &mockKnowledgeRepo{
			getFn: func(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
				gotWorkspaceID = workspaceID
				gotKnowledgeID = id
				return &model.Knowledge{
					ID:         id,
					CaseID:     testCaseID,
					Title:      "Root cause analysis",
					Summary:    "Detailed findings",
					SourceURLs: []string{"https://wiki.example.com/rca"},
				}, nil
			},
		}
		repo := newMockRepo(nil, knowledgeRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{
			"knowledge_id": "k-abc-123",
		})
		gt.NoError(t, err)
		gt.Value(t, gotWorkspaceID).Equal(testWorkspaceID)
		gt.Value(t, gotKnowledgeID).Equal(model.KnowledgeID("k-abc-123"))
		gt.Value(t, result["title"]).Equal("Root cause analysis")
		gt.Value(t, result["summary"]).Equal("Detailed findings")
		gt.Value(t, result["id"]).Equal("k-abc-123")
	})

	t.Run("propagates repository error", func(t *testing.T) {
		knowledgeRepo := &mockKnowledgeRepo{
			getFn: func(_ context.Context, _ string, _ model.KnowledgeID) (*model.Knowledge, error) {
				return nil, errors.New("not found")
			},
		}
		repo := newMockRepo(nil, knowledgeRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{
			"knowledge_id": "non-existent",
		})
		gt.Error(t, err)
	})

	t.Run("returns error when knowledge_id is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{
			"knowledge_id": "",
		})
		gt.Error(t, err)
	})
}

// ----- tool.Update call verification tests -----

func TestToolUpdateCalls(t *testing.T) {
	t.Run("list_actions posts update message", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		actionRepo := &mockActionRepo{
			getByCaseFn: func(_ context.Context, _ string, _ int64) ([]*model.Action, error) {
				return []*model.Action{}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__list_actions").Run(ctx, map[string]any{})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Listing actions...")
	})

	t.Run("get_action posts update message with action ID", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, id int64) (*model.Action, error) {
				return &model.Action{ID: id, Title: "T", Status: types.ActionStatusTodo}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(7)})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Getting action #7...")
	})

	t.Run("create_action posts update message with title", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		creator := &mockActionMutator{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: creator})
		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{"title": "Deploy fix"})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Creating action: Deploy fix")
	})

	t.Run("update_action posts update message with action ID", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		mutator := &mockActionMutator{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})
		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(11),
			"description": "Updated desc",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Updating action #11...")
	})

	t.Run("update_action_status posts update message with ID and status", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		mutator := &mockActionMutator{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})
		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(3),
			"status":    "COMPLETED",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Updating action #3 status -> COMPLETED")
	})

	t.Run("set_action_assignee posts update message when setting", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		mutator := &mockActionMutator{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})
		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(2),
			"assignee_id": "U005",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Setting assignee U005 on action #2")
	})

	t.Run("set_action_assignee posts update message when clearing", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		mutator := &mockActionMutator{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}, ActionUC: mutator})
		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(9),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Clearing assignee on action #9")
	})

	t.Run("search_knowledge posts update message with query", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		knowledgeRepo := &mockKnowledgeRepo{
			findByEmbeddingFn: func(_ context.Context, _ string, _ []float32, _ int) ([]*model.Knowledge, error) {
				return nil, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, knowledgeRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": "firewall rules"})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Searching knowledge: firewall rules")
	})

	t.Run("get_knowledge posts update message with ID", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		knowledgeRepo := &mockKnowledgeRepo{
			getFn: func(_ context.Context, _ string, id model.KnowledgeID) (*model.Knowledge, error) {
				return &model.Knowledge{ID: id, Title: "Test"}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil, knowledgeRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})
		_, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{"knowledge_id": "k-xyz"})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Getting knowledge k-xyz...")
	})
}

// ----- mock MemoryRepository -----

type mockMemoryRepo struct {
	createFn          func(ctx context.Context, workspaceID string, caseID int64, memory *model.Memory) (*model.Memory, error)
	deleteFn          func(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) error
	listFn            func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Memory, error)
	findByEmbeddingFn func(ctx context.Context, workspaceID string, caseID int64, embedding []float32, limit int) ([]*model.Memory, error)
}

func (m *mockMemoryRepo) Create(ctx context.Context, workspaceID string, caseID int64, memory *model.Memory) (*model.Memory, error) {
	if m.createFn != nil {
		return m.createFn(ctx, workspaceID, caseID, memory)
	}
	created := *memory
	created.ID = model.NewMemoryID()
	return &created, nil
}

func (m *mockMemoryRepo) Get(_ context.Context, _ string, _ int64, _ model.MemoryID) (*model.Memory, error) {
	return nil, errors.New("not found")
}

func (m *mockMemoryRepo) Delete(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, workspaceID, caseID, memoryID)
	}
	return nil
}

func (m *mockMemoryRepo) List(ctx context.Context, workspaceID string, caseID int64) ([]*model.Memory, error) {
	if m.listFn != nil {
		return m.listFn(ctx, workspaceID, caseID)
	}
	return nil, nil
}

func (m *mockMemoryRepo) FindByEmbedding(ctx context.Context, workspaceID string, caseID int64, embedding []float32, limit int) ([]*model.Memory, error) {
	if m.findByEmbeddingFn != nil {
		return m.findByEmbeddingFn(ctx, workspaceID, caseID, embedding, limit)
	}
	return nil, nil
}

// ----- mock AssistLogRepository -----

type mockAssistLogRepo struct{}

func (m *mockAssistLogRepo) Create(_ context.Context, _ string, _ int64, log *model.AssistLog) (*model.AssistLog, error) {
	return log, nil
}
func (m *mockAssistLogRepo) List(_ context.Context, _ string, _ int64, _, _ int) ([]*model.AssistLog, int, error) {
	return nil, 0, nil
}

// ----- mockRepoWithMemory extends mockRepo with Memory and AssistLog support -----

type mockRepoWithMemory struct {
	mockRepo
	memoryRepo    interfaces.MemoryRepository
	assistLogRepo interfaces.AssistLogRepository
}

func (m *mockRepoWithMemory) Memory() interfaces.MemoryRepository       { return m.memoryRepo }
func (m *mockRepoWithMemory) AssistLog() interfaces.AssistLogRepository { return m.assistLogRepo }

func newMockRepoForAssist(knowledgeRepo interfaces.KnowledgeRepository, memoryRepo interfaces.MemoryRepository) *mockRepoWithMemory {
	if knowledgeRepo == nil {
		knowledgeRepo = &mockKnowledgeRepo{}
	}
	if memoryRepo == nil {
		memoryRepo = &mockMemoryRepo{}
	}
	return &mockRepoWithMemory{
		mockRepo: mockRepo{
			actionRepo:    &mockActionRepo{},
			knowledgeRepo: knowledgeRepo,
		},
		memoryRepo:    memoryRepo,
		assistLogRepo: &mockAssistLogRepo{},
	}
}

// ----- NewForAssist tests -----

func TestNewForAssist_ReturnsAllTools(t *testing.T) {
	repo := newMockRepoForAssist(nil, nil)
	llm := &mockLLMClient{}
	tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: llm})

	// 8 base tools + 2 knowledge write + 4 memory tools = 14.
	// Slack post_message lives in pkg/agent/tool/slack now and is registered
	// separately by the assist usecase, so it is not counted here.
	gt.Array(t, tools).Length(14)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Spec().Name] = true
	}
	gt.Value(t, toolNames["core__create_knowledge"]).Equal(true)
	gt.Value(t, toolNames["core__update_knowledge"]).Equal(true)
	gt.Value(t, toolNames["core__create_memory"]).Equal(true)
	gt.Value(t, toolNames["core__delete_memory"]).Equal(true)
	gt.Value(t, toolNames["core__search_memory"]).Equal(true)
	gt.Value(t, toolNames["core__list_memories"]).Equal(true)
}

func TestCreateMemoryTool(t *testing.T) {
	ctx := context.Background()

	t.Run("creates memory with claim", func(t *testing.T) {
		var captured *model.Memory
		memoryRepo := &mockMemoryRepo{
			createFn: func(_ context.Context, wsID string, caseID int64, mem *model.Memory) (*model.Memory, error) {
				gt.Value(t, wsID).Equal(testWorkspaceID)
				gt.Value(t, caseID).Equal(testCaseID)
				captured = mem
				created := *mem
				created.ID = "mem-1"
				return &created, nil
			},
		}
		repo := newMockRepoForAssist(nil, memoryRepo)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__create_memory").Run(ctx, map[string]any{"claim": "The server restart is scheduled for Friday"})
		gt.NoError(t, err)
		gt.Value(t, captured.Claim).Equal("The server restart is scheduled for Friday")
		gt.Value(t, captured.CaseID).Equal(testCaseID)
		gt.Value(t, len(captured.Embedding) > 0).Equal(true)
		gt.Value(t, result["claim"]).Equal("The server restart is scheduled for Friday")
	})

	t.Run("returns error when claim is empty", func(t *testing.T) {
		repo := newMockRepoForAssist(nil, nil)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__create_memory").Run(ctx, map[string]any{"claim": ""})
		gt.Error(t, err)
	})
}

func TestDeleteMemoryTool(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes memory by ID", func(t *testing.T) {
		var gotMemoryID model.MemoryID
		memoryRepo := &mockMemoryRepo{
			deleteFn: func(_ context.Context, _ string, _ int64, memID model.MemoryID) error {
				gotMemoryID = memID
				return nil
			},
		}
		repo := newMockRepoForAssist(nil, memoryRepo)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__delete_memory").Run(ctx, map[string]any{"memory_id": "mem-123"})
		gt.NoError(t, err)
		gt.Value(t, gotMemoryID).Equal(model.MemoryID("mem-123"))
		gt.Value(t, result["deleted"]).Equal(true)
	})

	t.Run("returns error when memory_id is empty", func(t *testing.T) {
		repo := newMockRepoForAssist(nil, nil)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__delete_memory").Run(ctx, map[string]any{"memory_id": ""})
		gt.Error(t, err)
	})
}

func TestListMemoriesTool(t *testing.T) {
	ctx := context.Background()

	t.Run("lists memories for case", func(t *testing.T) {
		memoryRepo := &mockMemoryRepo{
			listFn: func(_ context.Context, wsID string, caseID int64) ([]*model.Memory, error) {
				gt.Value(t, wsID).Equal(testWorkspaceID)
				gt.Value(t, caseID).Equal(testCaseID)
				return []*model.Memory{
					{ID: "m1", CaseID: caseID, Claim: "fact one"},
					{ID: "m2", CaseID: caseID, Claim: "fact two"},
				}, nil
			},
		}
		repo := newMockRepoForAssist(nil, memoryRepo)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__list_memories").Run(ctx, map[string]any{})
		gt.NoError(t, err)
		items := result["memories"].([]map[string]any)
		gt.Array(t, items).Length(2)
		gt.Value(t, items[0]["claim"]).Equal("fact one")
		gt.Value(t, result["count"]).Equal(2)
	})
}

func TestCreateKnowledgeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("creates knowledge with title and summary", func(t *testing.T) {
		var captured *model.Knowledge
		knowledgeRepo := &mockKnowledgeRepo{
			createFn: func(_ context.Context, wsID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
				gt.Value(t, wsID).Equal(testWorkspaceID)
				captured = knowledge
				return knowledge, nil
			},
		}
		repo := newMockRepoForAssist(knowledgeRepo, nil)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		result, err := findTool(tools, "core__create_knowledge").Run(ctx, map[string]any{
			"title":   "Root Cause",
			"summary": "The issue was caused by a misconfiguration",
		})
		gt.NoError(t, err)
		gt.Value(t, captured.Title).Equal("Root Cause")
		gt.Value(t, captured.Summary).Equal("The issue was caused by a misconfiguration")
		gt.Value(t, captured.CaseID).Equal(testCaseID)
		gt.Value(t, result["title"]).Equal("Root Cause")
	})

	t.Run("returns error when title is empty", func(t *testing.T) {
		repo := newMockRepoForAssist(nil, nil)
		tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID, EmbedClient: &mockLLMClient{}})

		_, err := findTool(tools, "core__create_knowledge").Run(ctx, map[string]any{"title": "", "summary": "text"})
		gt.Error(t, err)
	})
}
