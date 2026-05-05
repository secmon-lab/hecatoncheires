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

// ----- mock Repository -----

type mockRepo struct {
	actionRepo interfaces.ActionRepository
}

func (m *mockRepo) Action() interfaces.ActionRepository { return m.actionRepo }

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
func newMockRepo(actionRepo interfaces.ActionRepository) *mockRepo {
	if actionRepo == nil {
		actionRepo = &mockActionRepo{}
	}
	return &mockRepo{actionRepo: actionRepo}
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

func TestNew_ReturnsSixTools(t *testing.T) {
	repo := newMockRepo(nil)
	tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})
	gt.Array(t, tools).Length(6)

	toolNames := make(map[string]bool)
	for _, tl := range tools {
		toolNames[tl.Spec().Name] = true
	}
	gt.Value(t, toolNames["core__list_actions"]).Equal(true)
	gt.Value(t, toolNames["core__get_action"]).Equal(true)
	gt.Value(t, toolNames["core__create_action"]).Equal(true)
	gt.Value(t, toolNames["core__update_action"]).Equal(true)
	gt.Value(t, toolNames["core__update_action_status"]).Equal(true)
	gt.Value(t, toolNames["core__set_action_assignee"]).Equal(true)
}

func TestNewForAssist_ReturnsSameSixTools(t *testing.T) {
	repo := newMockRepo(nil)
	tools := core.NewForAssist(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})
	gt.Array(t, tools).Length(6)
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__get_action").Run(ctx, map[string]any{"action_id": float64(999)})
		gt.Error(t, err)
	})

	t.Run("returns error when action_id is missing", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		result, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":       "New investigation",
			"description": "Look into the alerts",
			"status":      "IN_PROGRESS",
			"assignee_id": "U001",
		})
		gt.NoError(t, err)
		gt.Value(t, captured.CaseID).Equal(testCaseID)
		gt.Value(t, captured.Title).Equal("New investigation")
		gt.Value(t, captured.Description).Equal("Look into the alerts")
		gt.Value(t, captured.Status).Equal(types.ActionStatusInProgress)
		gt.Value(t, captured.AssigneeID).Equal("U001")
		gt.Value(t, result["id"]).Equal(int64(10))
	})

	t.Run("defaults status to BACKLOG when omitted", func(t *testing.T) {
		var captured *model.Action
		actionRepo := &mockActionRepo{
			createFn: func(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
				captured = action
				return action, nil
			},
		}
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{"title": "Quick task"})
		gt.NoError(t, err)
		// With no statusSet override, the tool falls back to
		// model.DefaultActionStatusSet() whose initial id is BACKLOG.
		gt.Value(t, captured.Status).Equal(types.ActionStatusBacklog)
	})

	t.Run("returns error when title is missing", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":  "Test",
			"status": "FLYING",
		})
		gt.Error(t, err)
	})

	t.Run("returns error when assignee_ids contains non-string element", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__create_action").Run(ctx, map[string]any{
			"title":       "Test",
			"assignee_id": 42,
		})
		gt.Error(t, err)
	})
}

func TestUpdateActionTool(t *testing.T) {
	ctx := context.Background()

	t.Run("updates title and description", func(t *testing.T) {
		original := &model.Action{ID: 5, CaseID: testCaseID, Title: "Old title", Description: "Old desc", Status: types.ActionStatusTodo, AssigneeID: "U001"}
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		// Assignee should not be changed
		gt.Value(t, updated.AssigneeID).Equal("U001")
	})

	t.Run("replaces assignee_id when provided", func(t *testing.T) {
		original := &model.Action{ID: 6, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeID: "U001"}
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(6),
			"assignee_id": "U003",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.AssigneeID).Equal("U003")
	})

	t.Run("clears assignee_id when empty string provided", func(t *testing.T) {
		original := &model.Action{ID: 60, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeID: "U001"}
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(60),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.AssigneeID).Equal("")
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(8),
			"description": "Added desc",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.Title).Equal("Keep this title")
	})

	t.Run("returns error when action_id missing", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("returns error when action not found", func(t *testing.T) {
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return nil, nil
			},
		}
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{"action_id": float64(999)})
		gt.Error(t, err)
	})

	t.Run("returns error when assignee_ids contains non-string element", func(t *testing.T) {
		original := &model.Action{ID: 10, Title: "T", Status: types.ActionStatusTodo}
		actionRepo := &mockActionRepo{
			getFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return original, nil
			},
		}
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action").Run(ctx, map[string]any{
			"action_id":   float64(10),
			"assignee_id": 99,
		})
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "COMPLETED",
		})
		gt.Error(t, err)
	})

	t.Run("returns error for invalid status string", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__update_action_status").Run(ctx, map[string]any{
			"action_id": float64(1),
			"status":    "UNKNOWN_STATUS",
		})
		gt.Error(t, err)
	})
}

func TestSetActionAssigneeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the assignee", func(t *testing.T) {
		original := &model.Action{ID: 3, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeID: "U001"}
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(3),
			"assignee_id": "U002",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.AssigneeID).Equal("U002")
	})

	t.Run("clears the assignee when empty string", func(t *testing.T) {
		original := &model.Action{ID: 4, CaseID: testCaseID, Title: "Task", Status: types.ActionStatusTodo, AssigneeID: "U001"}
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
		repo := newMockRepo(actionRepo)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(4),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		gt.Value(t, updated.AssigneeID).Equal("")
	})

	t.Run("returns error when assignee_id is missing", func(t *testing.T) {
		repo := newMockRepo(nil)
		tools := core.New(core.Deps{Repo: repo, WorkspaceID: testWorkspaceID, CaseID: testCaseID})

		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id": float64(1),
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
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		original := &model.Action{ID: 2, Title: "T", Status: types.ActionStatusTodo}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
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
		original := &model.Action{ID: 9, Title: "T", Status: types.ActionStatusTodo, AssigneeID: "U001"}
		actionRepo := &mockActionRepo{
			getFn:    func(_ context.Context, _ string, _ int64) (*model.Action, error) { return original, nil },
			updateFn: func(_ context.Context, _ string, a *model.Action) (*model.Action, error) { return a, nil },
		}
		tools := core.New(core.Deps{Repo: newMockRepo(actionRepo), WorkspaceID: testWorkspaceID, CaseID: testCaseID})
		_, err := findTool(tools, "core__set_action_assignee").Run(ctx, map[string]any{
			"action_id":   float64(9),
			"assignee_id": "",
		})
		gt.NoError(t, err)
		gt.Array(t, *msgs).Length(1)
		gt.Value(t, (*msgs)[0]).Equal("Clearing assignee on action #9")
	})
}
