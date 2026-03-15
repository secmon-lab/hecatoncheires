package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// commandTestSlackService extends mockSlackService with OpenView capture
type commandTestSlackService struct {
	mockSlackService
	openViewCalled  bool
	openViewTrigger string
	openViewRequest goslack.ModalViewRequest
	postedMessages  []commandTestPostedMessage
}

type commandTestPostedMessage struct {
	ChannelID string
	Text      string
}

func (m *commandTestSlackService) OpenView(_ context.Context, triggerID string, view goslack.ModalViewRequest) error {
	m.openViewCalled = true
	m.openViewTrigger = triggerID
	m.openViewRequest = view
	return nil
}

func (m *commandTestSlackService) PostMessage(_ context.Context, channelID string, _ []goslack.Block, text string) (string, error) {
	m.postedMessages = append(m.postedMessages, commandTestPostedMessage{
		ChannelID: channelID,
		Text:      text,
	})
	return "1234567890.123456", nil
}

func TestSlackUseCases_HandleSlashCommand(t *testing.T) {
	t.Run("workspace specified and valid opens case creation modal", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewTrigger).Equal("trigger-1")
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)
		gt.Value(t, slackMock.openViewRequest.Title.Text).Equal("Create Case")

		// Verify private_metadata contains workspace_id and channel_id
		var meta struct {
			WorkspaceID string `json:"workspace_id"`
			ChannelID   string `json:"channel_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.WorkspaceID).Equal("risk")
		gt.Value(t, meta.ChannelID).Equal("C001")
	})

	t.Run("workspace specified but invalid returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "nonexistent")
		gt.Value(t, err).NotNil()
		gt.Bool(t, slackMock.openViewCalled).False()
	})

	t.Run("no workspace specified with zero workspaces returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "")
		gt.Value(t, err).NotNil()
		gt.Bool(t, slackMock.openViewCalled).False()
	})

	t.Run("no workspace specified with single workspace opens case creation modal directly", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "only-ws", Name: "Only Workspace"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)

		var meta struct {
			WorkspaceID string `json:"workspace_id"`
			ChannelID   string `json:"channel_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.WorkspaceID).Equal("only-ws")
	})

	t.Run("workspace with field schema includes custom fields in modal", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "medium", Name: "Medium"},
						{ID: "low", Name: "Low"},
					}},
					{ID: "notes", Name: "Notes", Type: types.FieldTypeText, Required: false},
					{ID: "due_date", Name: "Due Date", Type: types.FieldTypeDate, Required: false},
				},
			},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		// Title + Description + 3 custom fields = 5 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(5)
	})

	t.Run("no workspace specified with multiple workspaces opens workspace select modal", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "incident", Name: "Incident Response"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDSelectWorkspace)
		gt.Value(t, slackMock.openViewRequest.Title.Text).Equal("Create Case")
		gt.Value(t, slackMock.openViewRequest.Submit.Text).Equal("Next")
	})
}

func TestSlackUseCases_HandleWorkspaceSelectSubmit(t *testing.T) {
	t.Run("returns case creation modal with selected workspace", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		meta, _ := json.Marshal(map[string]string{"channel_id": "C001"})
		callback := &goslack.InteractionCallback{
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_ws_select_block": {
							"hc_ws_radio": {
								SelectedOption: goslack.OptionBlockObject{
									Value: "risk",
								},
							},
						},
					},
				},
			},
		}

		view, err := uc.HandleWorkspaceSelectSubmit(callback)
		gt.NoError(t, err).Required()
		gt.Value(t, view).NotNil()
		gt.Value(t, view.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)

		var viewMeta struct {
			WorkspaceID string `json:"workspace_id"`
			ChannelID   string `json:"channel_id"`
		}
		err = json.Unmarshal([]byte(view.PrivateMetadata), &viewMeta)
		gt.NoError(t, err).Required()
		gt.Value(t, viewMeta.WorkspaceID).Equal("risk")
		gt.Value(t, viewMeta.ChannelID).Equal("C001")
	})

	t.Run("returns error when no workspace selected", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		meta, _ := json.Marshal(map[string]string{"channel_id": "C001"})
		callback := &goslack.InteractionCallback{
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_ws_select_block": {
							"hc_ws_radio": {
								SelectedOption: goslack.OptionBlockObject{
									Value: "",
								},
							},
						},
					},
				},
			},
		}

		_, err := uc.HandleWorkspaceSelectSubmit(callback)
		gt.Value(t, err).NotNil()
	})
}

func TestSlackUseCases_HandleCaseCreationSubmit(t *testing.T) {
	t.Run("creates case and posts confirmation message", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, "")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{
				ID: "U001",
			},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {
								Value: "Test Case Title",
							},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {
								Value: "Test description",
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		// Verify confirmation message was posted
		gt.Array(t, slackMock.postedMessages).Length(1)
		gt.Value(t, slackMock.postedMessages[0].ChannelID).Equal("C001")
		gt.String(t, slackMock.postedMessages[0].Text).Contains("Test Case Title")

		// Verify case was created
		cases, err := repo.Case().List(context.Background(), "risk")
		gt.NoError(t, err).Required()
		gt.Array(t, cases).Length(1)
		gt.Value(t, cases[0].Title).Equal("Test Case Title")
		gt.Value(t, cases[0].Description).Equal("Test description")
	})

	t.Run("creates case with custom field values", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "medium", Name: "Medium"},
					}},
					{ID: "notes", Name: "Notes", Type: types.FieldTypeText},
				},
			},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, "")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Risk Case"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "A risk case"},
						},
						"hc_field_block_severity": {
							"hc_field_action_severity": {
								Type: "static_select",
								SelectedOption: goslack.OptionBlockObject{
									Value: "high",
								},
							},
						},
						"hc_field_block_notes": {
							"hc_field_action_notes": {
								Type:  "plain_text_input",
								Value: "Important note",
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		cases, err := repo.Case().List(context.Background(), "risk")
		gt.NoError(t, err).Required()
		gt.Array(t, cases).Length(1)
		gt.Value(t, cases[0].Title).Equal("Risk Case")

		// Verify custom fields were set
		gt.Value(t, cases[0].FieldValues).NotNil()
		gt.Value(t, cases[0].FieldValues["severity"].Value).Equal("high")
		gt.Value(t, cases[0].FieldValues["notes"].Value).Equal("Important note")
	})

	t.Run("returns error when title is empty", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, "")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: ""},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.Value(t, err).NotNil()
	})
}
