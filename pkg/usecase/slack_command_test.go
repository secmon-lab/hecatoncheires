package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
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
	i18n.Init(i18n.LangEN)

	t.Run("workspace specified and valid opens case creation modal", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewTrigger).Equal("trigger-1")
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)
		gt.Value(t, slackMock.openViewRequest.Title.Text).Equal("Create Case")

		// Verify private_metadata contains workspace_id, channel_id, and source_team_id
		var meta struct {
			WorkspaceID  string `json:"workspace_id"`
			ChannelID    string `json:"channel_id"`
			SourceTeamID string `json:"source_team_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.WorkspaceID).Equal("risk")
		gt.Value(t, meta.ChannelID).Equal("C001")
		gt.Value(t, meta.SourceTeamID).Equal("")
	})

	t.Run("case creation modal includes private checkbox", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		// Title + Description + Private checkbox = 3 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(3)

		found := false
		for _, block := range slackMock.openViewRequest.Blocks.BlockSet {
			if inputBlock, ok := block.(*goslack.InputBlock); ok {
				if inputBlock.BlockID == usecase.SlackBlockIDCasePrivate {
					found = true
					break
				}
			}
		}
		gt.Bool(t, found).True()
	})

	t.Run("workspace specified but invalid returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "nonexistent", "")
		gt.Value(t, err).NotNil()
		gt.Bool(t, slackMock.openViewCalled).False()
	})

	t.Run("no workspace specified with zero workspaces returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "")
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

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "")
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

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		// Title + Description + Private checkbox + 3 custom fields = 6 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(6)
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

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDSelectWorkspace)
		gt.Value(t, slackMock.openViewRequest.Title.Text).Equal("Create Case")
		gt.Value(t, slackMock.openViewRequest.Submit.Text).Equal("Next")
	})

	t.Run("source team ID is preserved in private_metadata", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "TSOURCE")
		gt.NoError(t, err).Required()

		var meta struct {
			WorkspaceID  string `json:"workspace_id"`
			ChannelID    string `json:"channel_id"`
			SourceTeamID string `json:"source_team_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.SourceTeamID).Equal("TSOURCE")
	})
}

func TestSlackUseCases_HandleWorkspaceSelectSubmit(t *testing.T) {
	i18n.Init(i18n.LangEN)

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

		view, err := uc.HandleWorkspaceSelectSubmit(context.Background(), callback)
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

		_, err := uc.HandleWorkspaceSelectSubmit(context.Background(), callback)
		gt.Value(t, err).NotNil()
	})
}

func TestSlackUseCases_HandleCaseCreationSubmit(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("creates case and posts confirmation message", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

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
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

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

	t.Run("creates private case when private checkbox is checked", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

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
							"hc_case_title": {Value: "Private Case"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Secret stuff"},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "private"},
								},
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
		gt.Value(t, cases[0].Title).Equal("Private Case")
		gt.Bool(t, cases[0].IsPrivate).True()
	})

	t.Run("creates non-private case when private checkbox is not checked", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

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
							"hc_case_title": {Value: "Public Case"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Open stuff"},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{},
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
		gt.Value(t, cases[0].Title).Equal("Public Case")
		gt.Bool(t, cases[0].IsPrivate).False()
	})

	t.Run("returns error when title is empty", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

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

func TestSlackUseCases_HandleSlashCommand_EditCase(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("opens edit modal when channel has linked case", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "low", Name: "Low"},
					}},
				},
			},
		})

		// Create a case linked to a channel
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title:       "Existing Case",
			Description: "Existing description",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-CASE-CHANNEL"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE-CHANNEL", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
		gt.Value(t, slackMock.openViewRequest.Title.Text).Equal("Edit Case")
		gt.Value(t, slackMock.openViewRequest.Submit.Text).Equal("Save")

		// Verify private_metadata contains case_id
		var meta struct {
			WorkspaceID string `json:"workspace_id"`
			ChannelID   string `json:"channel_id"`
			CaseID      int64  `json:"case_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.WorkspaceID).Equal("risk")
		gt.Value(t, meta.ChannelID).Equal("C-CASE-CHANNEL")
		gt.Value(t, meta.CaseID).Equal(created.ID)

		// Verify blocks: Title + Description + 1 custom field = 3 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(3)
	})

	t.Run("opens create modal when channel has no linked case", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-NO-CASE", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)
	})

	t.Run("denies access to private case for non-member", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		// Create a private case
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title:          "Private Case",
			IsPrivate:      true,
			ChannelUserIDs: []string{"U-MEMBER"},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-PRIVATE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		// Non-member tries to access
		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U-OUTSIDER", "C-PRIVATE", "", "")
		gt.NoError(t, err).Required()

		// Should NOT have opened any modal
		gt.Bool(t, slackMock.openViewCalled).False()
		// Should have posted ephemeral error
		gt.Value(t, slackMock.ephemeralText).Equal("You don't have access to this case.")
	})

	t.Run("opens edit modal with workspace specified and case exists", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title: "WS Case",
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-WS-CASE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-WS-CASE", "risk", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
	})
}

func TestSlackUseCases_HandleCaseEditSubmit(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("updates case and posts confirmation", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "low", Name: "Low"},
					}},
				},
			},
		})

		// Create an existing case
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title:       "Original Title",
			Description: "Original description",
			AssigneeIDs: []string{"U-ASSIGNEE"},
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      created.ID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Updated Title"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Updated description"},
						},
						"hc_field_block_severity": {
							"hc_field_action_severity": {
								Type: "static_select",
								SelectedOption: goslack.OptionBlockObject{
									Value: "low",
								},
							},
						},
					},
				},
			},
		}

		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		// Verify case was updated
		updated, err := repo.Case().Get(context.Background(), "risk", created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated description")
		gt.Value(t, updated.FieldValues["severity"].Value).Equal("low")

		// Verify assignees are preserved
		gt.Array(t, updated.AssigneeIDs).Length(1)
		gt.Value(t, updated.AssigneeIDs[0]).Equal("U-ASSIGNEE")

		// Verify confirmation message
		gt.Array(t, slackMock.postedMessages).Length(1)
		gt.Value(t, slackMock.postedMessages[0].ChannelID).Equal("C-CASE")
		gt.String(t, slackMock.postedMessages[0].Text).Contains("Updated Title")
		gt.String(t, slackMock.postedMessages[0].Text).Contains("updated")
	})

	t.Run("returns error when case not found", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      99999,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Updated Title"},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.Value(t, err).NotNil()
	})
}

func TestBuildFieldInputBlockWithValue(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("text field with initial value", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "notes", Name: "Notes", Type: types.FieldTypeText},
				},
			},
		})

		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title: "Test",
			FieldValues: map[string]model.FieldValue{
				"notes": {FieldID: "notes", Type: types.FieldTypeText, Value: "initial text"},
			},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-TEST"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-TEST", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
		// Title + Description + 1 custom field = 3 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(3)
	})

	t.Run("date field with initial value", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "due", Name: "Due Date", Type: types.FieldTypeDate},
				},
			},
		})

		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title: "Test",
			FieldValues: map[string]model.FieldValue{
				"due": {FieldID: "due", Type: types.FieldTypeDate, Value: "2026-01-15"},
			},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-DATE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-DATE", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
	})

	t.Run("no field values shows empty fields", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "notes", Name: "Notes", Type: types.FieldTypeText},
				},
			},
		})

		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			Title: "No Fields",
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-NOFIELD"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-NOFIELD", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
		// Title + Description + 1 custom field = 3 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(3)
	})
}
