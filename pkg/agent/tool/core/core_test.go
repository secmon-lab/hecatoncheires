package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

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
func (m *mockRepo) PutToken(ctx context.Context, token *auth.Token) error {
	panic("unexpected call: PutToken()")
}
func (m *mockRepo) GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error) {
	panic("unexpected call: GetToken()")
}
func (m *mockRepo) DeleteToken(ctx context.Context, tokenID auth.TokenID) error {
	panic("unexpected call: DeleteToken()")
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

func TestNew_ReturnsNineTools(t *testing.T) {
	repo := newMockRepo(nil, nil)
	llm := &mockLLMClient{}
	tools := core.New(repo, testWorkspaceID, testCaseID, llm)
	gt.Array(t, tools).Length(9)
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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		result, err := findTool(tools, "core__list_actions").Run(ctx, map[string]any{})
		gt.NoError(t, err)
		items := result["actions"].([]map[string]any)
		gt.Array(t, items).Length(0)
	})

	t.Run("returns actions from repository", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getByCaseFn: func(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
				return []*model.Action{
					{ID: 1, CaseID: caseID, Title: "Fix bug", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}},
					{ID: 2, CaseID: caseID, Title: "Write docs", Status: types.ActionStatusCompleted},
				}, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(999)})
		gt.Error(t, err)
	})

	t.Run("returns error when action_id is missing", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})
}

func TestCreateActionTool(t *testing.T) {
	ctx := context.Background()

	t.Run("creates action with correct fields", func(t *testing.T) {
		var captured *model.Action
		actionRepo := &mockActionRepo{
			createFn: func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
				gt.Value(t, workspaceID).Equal(testWorkspaceID)
				captured = action
				result := *action
				result.ID = 10
				return &result, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		result, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":        "New investigation",
			"description":  "Look into the alerts",
			"status":       "IN_PROGRESS",
			"assignee_ids": []any{"U001", "U002"},
		})
		gt.NoError(t, err)
		gt.Value(t, captured.CaseID).Equal(testCaseID)
		gt.Value(t, captured.Title).Equal("New investigation")
		gt.Value(t, captured.Description).Equal("Look into the alerts")
		gt.Value(t, captured.Status).Equal(types.ActionStatusInProgress)
		gt.Array(t, captured.AssigneeIDs).Has("U001")
		gt.Array(t, captured.AssigneeIDs).Has("U002")
		gt.Value(t, result["id"]).Equal(int64(10))
	})

	t.Run("defaults status to TODO when omitted", func(t *testing.T) {
		var captured *model.Action
		actionRepo := &mockActionRepo{
			createFn: func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
				captured = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{"title": "Quick task"})
		gt.NoError(t, err)
		gt.Value(t, captured.Status).Equal(types.ActionStatusTodo)
	})

	t.Run("returns error when title is missing", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":  "Test",
			"status": "FLYING",
		})
		gt.Error(t, err)
	})
}

func TestUpdateActionTool(t *testing.T) {
	ctx := context.Background()

	t.Run("updates title and description", func(t *testing.T) {
		original := &model.Action{ID: 5, CaseID: testCaseID, Title: "Old title", Description: "Old desc", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, id int64) (*model.Action, error) {
				gt.Value(t, id).Equal(int64(5))
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		result, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(5),
			"title":       "New title",
			"description": "New description",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.Title).Equal("New title")
		gt.Value(t, updated.Description).Equal("New description")
		gt.Value(t, result["title"]).Equal("New title")
		gt.Value(t, result["description"]).Equal("New description")
		// Assignees should not be changed
		gt.Array(t, updated.AssigneeIDs).Has("U001")
	})

	t.Run("replaces assignee_ids when provided", func(t *testing.T) {
		original := &model.Action{ID: 6, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001", "U002"}}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":    float64(6),
			"assignee_ids": []any{"U003"},
		})
		gt.NoError(t, err)
		gt.Array(t, updated.AssigneeIDs).Length(1)
		gt.Array(t, updated.AssigneeIDs).Has("U003")
	})

	t.Run("keeps description when empty string provided", func(t *testing.T) {
		original := &model.Action{ID: 7, CaseID: testCaseID, Title: "Task", Description: "Old desc", Status: types.ActionStatusTodo}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(7),
			"description": "",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.Description).Equal("Old desc")
	})

	t.Run("keeps title when not provided", func(t *testing.T) {
		original := &model.Action{ID: 8, CaseID: testCaseID, Title: "Keep this title", Status: types.ActionStatusTodo}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(8),
			"description": "Added desc",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.Title).Equal("Keep this title")
	})

	t.Run("returns error when action_id missing", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("returns error when action not found", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return nil, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{"action_id": float64(999)})
		gt.Error(t, err)
	})
}

func TestUpdateActionStatusTool(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches action then updates with new status", func(t *testing.T) {
		original := &model.Action{ID: 5, CaseID: testCaseID, Title: "Old task", Status: types.ActionStatusTodo}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
				gt.Value(t, id).Equal(int64(5))
				return original, nil
			},
			updateFn: func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		result, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(5),
			"status":    "COMPLETED",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.Status).Equal(types.ActionStatusCompleted)
		gt.Value(t, result["status"]).Equal("COMPLETED")
	})

	t.Run("returns error when Get fails", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return nil, errors.New("db error")
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "COMPLETED",
		})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status string", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "UNKNOWN_STATUS",
		})
		gt.Error(t, err)
	})
}

func TestAddActionAssigneeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("adds new assignee to action", func(t *testing.T) {
		original := &model.Action{ID: 3, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__add_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(3),
			"assignee_id": "U002",
		})
		gt.NoError(t, err)
		gt.Array(t, updated.AssigneeIDs).Has("U001")
		gt.Array(t, updated.AssigneeIDs).Has("U002")
		gt.Array(t, updated.AssigneeIDs).Length(2)
	})

	t.Run("does not call Update when assignee already present", func(t *testing.T) {
		original := &model.Action{ID: 3, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}}
		updateCalled := false
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updateCalled = true
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		result, err := findTool(tools, "core__add_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(3),
			"assignee_id": "U001", // already present
		})
		gt.NoError(t, err)
		gt.Value(t, updateCalled).Equal(false)
		ids := result["assignee_ids"].([]string)
		gt.Array(t, ids).Length(1)
	})

	t.Run("returns error when assignee_id is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__add_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(1),
			"assignee_id": "",
		})
		gt.Error(t, err)
	})
}

func TestRemoveActionAssigneeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("removes specified assignee from action", func(t *testing.T) {
		original := &model.Action{ID: 4, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001", "U002", "U003"}}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__remove_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(4),
			"assignee_id": "U002",
		})
		gt.NoError(t, err)
		gt.Array(t, updated.AssigneeIDs).Length(2)
		gt.Array(t, updated.AssigneeIDs).Has("U001")
		gt.Array(t, updated.AssigneeIDs).Has("U003")
	})

	t.Run("no-op when removing non-existent assignee", func(t *testing.T) {
		original := &model.Action{ID: 4, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}}
		var updated *model.Action
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
			updateFn: func(_ context.Context, _ string, action *model.Action) (*model.Action, error) {
				updated = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__remove_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(4),
			"assignee_id": "U999",
		})
		gt.NoError(t, err)
		gt.Array(t, updated.AssigneeIDs).Length(1)
	})

	t.Run("returns error when assignee_id is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__remove_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(1),
			"assignee_id": "",
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
		tools := core.New(repo, testWorkspaceID, testCaseID, llm)

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__search_knowledge").Run(ctx, map[string]any{"query": "test query"})
		gt.NoError(t, err)
		gt.Value(t, gotLimit).Equal(5)
	})

	t.Run("returns error when query is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, llm)

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

		_, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{
			"knowledge_id": "non-existent",
		})
		gt.Error(t, err)
	})

	t.Run("returns error when knowledge_id is empty", func(t *testing.T) {
		repo := newMockRepo(nil, nil)
		tools := core.New(repo, testWorkspaceID, testCaseID, &mockLLMClient{})

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
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
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
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(7)})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Getting action #7...")
	})

	t.Run("create_action posts update message with title", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		actionRepo := &mockActionRepo{
			createFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) {
				return a, nil
			},
		}
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{"title": "Deploy fix"})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Creating action: Deploy fix")
	})

	t.Run("update_action posts update message with action ID", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		original := &model.Action{ID: 11, Title: "T", Status: types.ActionStatusTodo}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
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
		original := &model.Action{ID: 3, Title: "T", Status: types.ActionStatusTodo}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(3),
			"status":    "COMPLETED",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Updating action #3 status → COMPLETED")
	})

	t.Run("add_action_assignee posts update message", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		original := &model.Action{ID: 2, Title: "T", Status: types.ActionStatusTodo, AssigneeIDs: []string{}}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__add_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(2),
			"assignee_id": "U005",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Adding assignee U005 to action #2")
	})

	t.Run("remove_action_assignee posts update message", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		original := &model.Action{ID: 9, Title: "T", Status: types.ActionStatusTodo, AssigneeIDs: []string{"U001"}}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(newMockRepo(actionRepo, nil), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__remove_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(9),
			"assignee_id": "U001",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Removing assignee U001 from action #9")
	})

	t.Run("search_knowledge posts update message with query", func(t *testing.T) {
		ctx, msgs := newCtxWithUpdateCapture()
		knowledgeRepo := &mockKnowledgeRepo{
			findByEmbeddingFn: func(_ context.Context, _ string, _ []float32, _ int) ([]*model.Knowledge, error) {
				return nil, nil
			},
		}
		tools := core.New(newMockRepo(nil, knowledgeRepo), testWorkspaceID, testCaseID, &mockLLMClient{})
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
		tools := core.New(newMockRepo(nil, knowledgeRepo), testWorkspaceID, testCaseID, &mockLLMClient{})
		_, err := findTool(tools, "core__get_knowledge").Run(ctx, map[string]any{"knowledge_id": "k-xyz"})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Getting knowledge k-xyz...")
	})
}
