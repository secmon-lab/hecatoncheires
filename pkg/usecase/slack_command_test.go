package usecase_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
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

// PostMessageWithAttachment funnels into the same recorder as PostMessage:
// Action card posts go through this path now, and the slash-command tests
// assert on "did Slack get *any* post for the action?" regardless of which
// API method delivered it.
func (m *commandTestSlackService) PostMessageWithAttachment(_ context.Context, channelID string, text string, _ goslack.Attachment) (string, error) {
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "", "")
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		// Title + Description + Options (checkbox group with private + draft) = 3 blocks.
		// The Save-as-Draft body button was removed; drafts now come from
		// the "Draft mode" option inside the Options group.
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(3)

		var optionsBlock *goslack.InputBlock
		for _, block := range slackMock.openViewRequest.Blocks.BlockSet {
			inputBlock, ok := block.(*goslack.InputBlock)
			if !ok || inputBlock.BlockID != usecase.SlackBlockIDCasePrivate {
				continue
			}
			optionsBlock = inputBlock
		}
		gt.Value(t, optionsBlock).NotNil().Required()

		// The checkbox group carries three options in order: Private case,
		// Draft mode, Test case. All share the same checkbox group element
		// (keyed by the legacy SlackBlockIDCasePrivate / SlackActionIDCasePrivate
		// constants); the values are distinguished only by their option
		// value strings.
		checkboxGroup, ok := optionsBlock.Element.(*goslack.CheckboxGroupsBlockElement)
		gt.Bool(t, ok).True()
		gt.Value(t, checkboxGroup.ActionID).Equal(usecase.SlackActionIDCasePrivate)
		gt.Array(t, checkboxGroup.Options).Length(3).Required()
		gt.Value(t, checkboxGroup.Options[0].Value).Equal("private")
		gt.Value(t, checkboxGroup.Options[1].Value).Equal("draft")
		gt.Value(t, checkboxGroup.Options[2].Value).Equal("test")

		// No body-level Save-as-Draft button remains on the modal.
		for _, block := range slackMock.openViewRequest.Blocks.BlockSet {
			if actionBlock, ok := block.(*goslack.ActionBlock); ok {
				gt.Value(t, actionBlock.BlockID).NotEqual(usecase.SlackBlockIDSaveAsDraftActions)
			}
			if sec, ok := block.(*goslack.SectionBlock); ok {
				gt.Value(t, sec.BlockID).NotEqual(usecase.SlackBlockIDSaveAsDraftActions)
			}
		}
	})

	t.Run("workspace specified but invalid returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "nonexistent", "", "")
		gt.Value(t, err).NotNil()
		gt.Bool(t, slackMock.openViewCalled).False()
	})

	t.Run("no workspace specified with zero workspaces returns error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "", "")
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "", "")
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		// Title + Description + Options + 3 custom fields = 6 blocks.
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "", "", "")
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C001", "risk", "TSOURCE", "")
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

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
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U001")

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
		// The slash-command path does not pass through the Web auth
		// middleware, so HandleCaseCreationSubmit MUST inject the
		// callback user as the auth-context Token before reaching
		// persistCase. Asserting on the persisted ReporterID is what
		// catches a regression where that injection is missing —
		// without this, the case lands with an empty reporter and the
		// UI silently shows an empty Reporter cell.
		gt.Value(t, cases[0].ReporterID).Equal("U001")
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
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U001")

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
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U001")

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
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U001")

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

	t.Run("creates a test case when the test option is checked", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U001")

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
							"hc_case_title": {Value: "Test Case"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Drill"},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "test"},
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
		gt.Value(t, cases[0].Title).Equal("Test Case")
		gt.Bool(t, cases[0].IsTest).True()
		// The test option alone must not also flip private.
		gt.Bool(t, cases[0].IsPrivate).False()
	})

	t.Run("returns error when title is empty", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
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

	t.Run("Draft mode option diverts submission to CreateDraft", func(t *testing.T) {
		// Ticking the "Draft mode" checkbox in the Options group must
		// route the submit through CreateDraft instead of CreateCase,
		// so the resulting record lands in status=DRAFT and no Slack
		// channel is created. The user-facing receipt is the ephemeral
		// posted to the originating channel; verify both the persisted
		// state and the ephemeral.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Draft Case Title"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "in progress"},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "draft"},
								},
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		// Exactly one draft persisted; no OPEN case.
		drafts, err := repo.Case().ListDrafts(context.Background(), "risk")
		gt.NoError(t, err).Required()
		gt.Array(t, drafts).Length(1).Required()
		gt.Value(t, drafts[0].Status).Equal(types.CaseStatusDraft)
		gt.Value(t, drafts[0].Title).Equal("Draft Case Title")
		gt.Value(t, drafts[0].Description).Equal("in progress")
		gt.Value(t, drafts[0].ReporterID).Equal("UREPORTER")
		gt.Value(t, drafts[0].IsPrivate).Equal(false)
		// The legacy CreateCase path would have created a Slack channel
		// or at least called PostMessage; the draft path posts only an
		// ephemeral to the originating channel, never a channel message.
		gt.Number(t, len(slackMock.postedMessages)).Equal(0)
		// Without a web baseURL the ephemeral falls back to the plain
		// "Open the Drafts page on the web" message, with no link.
		gt.Value(t, slackMock.ephemeralChannelID).Equal("C001")
		gt.String(t, slackMock.ephemeralText).Contains("draft #1")
		gt.Bool(t, strings.Contains(slackMock.ephemeralText, "|")).False()
	})

	t.Run("Draft mode ephemeral uses the draft title as the link label", func(t *testing.T) {
		// When the CaseUseCase has a baseURL configured and the user
		// gave the draft a title, the ephemeral receipt must surface
		// the title as the clickable link label so the message reads
		// like "Saved as draft #1: <URL|Linked Draft>" instead of a
		// generic "Open the draft on the web" placeholder. We assert
		// on the Slack mrkdwn link syntax to avoid coupling to the
		// surrounding translation text.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "https://example.test")
		seedSlackUsers(t, repo, "UREPORTER")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Linked Draft"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: ""},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "draft"},
								},
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		gt.String(t, slackMock.ephemeralText).Contains("<https://example.test/ws/risk/drafts/1|Linked Draft>")
	})

	t.Run("Draft mode link label falls back to placeholder when title is empty", func(t *testing.T) {
		// If the user submitted Draft mode without entering a title
		// (drafts allow that), the link label has nothing meaningful
		// to display, so it falls back to the localised
		// MsgDraftLinkFallbackLabel string. We verify the label is a
		// non-empty value distinct from the literal title.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "https://example.test")
		seedSlackUsers(t, repo, "UREPORTER")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: ""},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: ""},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "draft"},
								},
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		// Link is present, but with the fallback label (anything other
		// than an empty string between the pipe and the closing `>`).
		gt.String(t, slackMock.ephemeralText).Contains("<https://example.test/ws/risk/drafts/1|")
		// Specifically: the link does not collapse to an empty label.
		gt.Bool(t, strings.Contains(slackMock.ephemeralText, "|>")).False()
	})

	t.Run("Draft mode link label escapes Slack mrkdwn metacharacters in the title", func(t *testing.T) {
		// `<`, `>`, `&`, and `|` inside the title would otherwise break
		// the `<URL|label>` link syntax. The escape helper must
		// neutralise them so the rendered link stays well-formed.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "https://example.test")
		seedSlackUsers(t, repo, "UREPORTER")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "<bad>&pipe|here"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: ""},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "draft"},
								},
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		// The label must be the escaped form, not the raw title.
		gt.String(t, slackMock.ephemeralText).Contains("&lt;bad&gt;&amp;pipe/here")
		// And the raw `<`/`>` from the title must not leak through.
		gt.Bool(t, strings.Contains(slackMock.ephemeralText, "<bad>")).False()
	})

	t.Run("Draft mode + Private case yields a private draft", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Private Draft"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: ""},
						},
						usecase.SlackBlockIDCasePrivate: {
							usecase.SlackActionIDCasePrivate: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "private"},
									{Value: "draft"},
								},
							},
						},
					},
				},
			},
		}

		err := slackUC.HandleCaseCreationSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		drafts, err := repo.Case().ListDrafts(context.Background(), "risk")
		gt.NoError(t, err).Required()
		gt.Array(t, drafts).Length(1).Required()
		gt.Value(t, drafts[0].IsPrivate).Equal(true)
		gt.Value(t, drafts[0].Status).Equal(types.CaseStatusDraft)
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
			ReporterID:  "U-TEST-DEFAULT",
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
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE-CHANNEL", "", "", "update")
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

		// Verify blocks: Title + Description + Assignees + Test option + 1 custom field = 5 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(5)
	})

	t.Run("opens create modal when channel has no linked case", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-NO-CASE", "", "", "")
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
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Private Case",
			IsPrivate:      true,
			ChannelUserIDs: []string{"U-MEMBER"},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-PRIVATE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		// Non-member tries to access
		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U-OUTSIDER", "C-PRIVATE", "", "", "")
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
			ReporterID: "U-TEST-DEFAULT",
			Title:      "WS Case",
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-WS-CASE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-WS-CASE", "risk", "", "update")
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
			ReporterID:  "U-TEST-DEFAULT",
			Title:       "Original Title",
			Description: "Original description",
			AssigneeIDs: []string{"U-ASSIGNEE"},
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
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

	t.Run("toggles the test flag from the edit modal checkbox", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		// Existing non-test case.
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Existing",
			IsTest:     false,
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
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
							"hc_case_title": {Value: "Existing"},
						},
						usecase.SlackBlockIDCaseTest: {
							usecase.SlackActionIDCaseTest: {
								SelectedOptions: []goslack.OptionBlockObject{
									{Value: "test"},
								},
							},
						},
					},
				},
			},
		}

		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		updated, err := repo.Case().Get(context.Background(), "risk", created.ID)
		gt.NoError(t, err).Required()
		gt.Bool(t, updated.IsTest).True()

		// Submitting again with the checkbox unticked flips it back off.
		callback.View.State.Values[usecase.SlackBlockIDCaseTest][usecase.SlackActionIDCaseTest] = goslack.BlockAction{
			SelectedOptions: []goslack.OptionBlockObject{},
		}
		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		cleared, err := repo.Case().Get(context.Background(), "risk", created.ID)
		gt.NoError(t, err).Required()
		gt.Bool(t, cleared.IsTest).False()
	})

	t.Run("replaces assignees from multi-user select payload", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID:  "U-TEST-DEFAULT",
			Title:       "Original",
			AssigneeIDs: []string{"U-OLD-A", "U-OLD-B"},
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "U-NEW-A", "U-NEW-B", "U-NEW-C")

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
							"hc_case_title": {Value: "Original"},
						},
						usecase.SlackBlockIDCaseAssignees: {
							usecase.SlackActionIDCaseAssignees: {
								Type:          "multi_users_select",
								SelectedUsers: []string{"U-NEW-A", "U-NEW-B", "U-NEW-C"},
							},
						},
					},
				},
			},
		}

		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		updated, err := repo.Case().Get(context.Background(), "risk", created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.AssigneeIDs).Length(3).Required()
		gt.Value(t, updated.AssigneeIDs[0]).Equal("U-NEW-A")
		gt.Value(t, updated.AssigneeIDs[1]).Equal("U-NEW-B")
		gt.Value(t, updated.AssigneeIDs[2]).Equal("U-NEW-C")
	})

	t.Run("clears assignees when multi-user select is empty", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID:  "U-TEST-DEFAULT",
			Title:       "Original",
			AssigneeIDs: []string{"U-OLD"},
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
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
							"hc_case_title": {Value: "Original"},
						},
						usecase.SlackBlockIDCaseAssignees: {
							usecase.SlackActionIDCaseAssignees: {
								Type:          "multi_users_select",
								SelectedUsers: []string{},
							},
						},
					},
				},
			},
		}

		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		updated, err := repo.Case().Get(context.Background(), "risk", created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.AssigneeIDs).Length(0)
	})

	t.Run("date custom field is stored as RFC3339", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "incident", Name: "Incident"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "detected_at", Name: "Detected At", Type: types.FieldTypeDate},
				},
			},
		})
		created, err := repo.Case().Create(context.Background(), "incident", &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "T"})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "incident",
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
							"hc_case_title": {Value: "T"},
						},
						"hc_field_block_detected_at": {
							"hc_field_action_detected_at": {
								Type:         "datepicker",
								SelectedDate: "2026-05-25",
							},
						},
					},
				},
			},
		}
		err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, callback)
		gt.NoError(t, err).Required()

		updated, err := repo.Case().Get(context.Background(), "incident", created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.FieldValues["detected_at"].Value).Equal("2026-05-25T00:00:00Z")
	})

	t.Run("returns error when case not found", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
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
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Test",
			FieldValues: map[string]model.FieldValue{
				"notes": {FieldID: "notes", Type: types.FieldTypeText, Value: "initial text"},
			},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-TEST"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-TEST", "", "", "update")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
		// Title + Description + Assignees + Test option + 1 custom field = 5 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(5)
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
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Test",
			FieldValues: map[string]model.FieldValue{
				"due": {FieldID: "due", Type: types.FieldTypeDate, Value: "2026-01-15"},
			},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-DATE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-DATE", "", "", "update")
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
			ReporterID: "U-TEST-DEFAULT",
			Title:      "No Fields",
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-NOFIELD"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-NOFIELD", "", "", "update")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
		// Title + Description + Assignees + Test option + 1 custom field = 5 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(5)
	})
}

func TestBuildFieldInputBlock_Markdown(t *testing.T) {
	t.Run("markdown field renders a multiline plain-text input", func(t *testing.T) {
		block := usecase.BuildFieldInputBlockForTest(config.FieldDefinition{
			ID:   "body",
			Name: "Body",
			Type: types.FieldTypeMarkdown,
		})
		inputBlock, ok := block.(*goslack.InputBlock)
		gt.Bool(t, ok).True().Required()
		element, ok := inputBlock.Element.(*goslack.PlainTextInputBlockElement)
		gt.Bool(t, ok).True().Required()
		gt.Bool(t, element.Multiline).True()
	})

	t.Run("markdown field with value injects the raw source as initial value", func(t *testing.T) {
		block := usecase.BuildFieldInputBlockWithValueForTest(
			config.FieldDefinition{ID: "body", Name: "Body", Type: types.FieldTypeMarkdown},
			&model.FieldValue{FieldID: "body", Type: types.FieldTypeMarkdown, Value: "# Title\n\ncontent"},
		)
		inputBlock, ok := block.(*goslack.InputBlock)
		gt.Bool(t, ok).True().Required()
		element, ok := inputBlock.Element.(*goslack.PlainTextInputBlockElement)
		gt.Bool(t, ok).True().Required()
		gt.Bool(t, element.Multiline).True()
		gt.String(t, element.InitialValue).Equal("# Title\n\ncontent")
	})
}

func TestBuildFieldOptions_ClampsLongDescription(t *testing.T) {
	maxRunes := usecase.SlackOptionDescriptionMaxRunesForTest

	t.Run("long description is clamped to Slack ceiling", func(t *testing.T) {
		// A description authored by a workspace operator that exceeds
		// Slack's 75-rune option-description cap would otherwise make
		// views.open fail with invalid_arguments, locking users out of
		// every Create / Edit / Draft Edit modal that uses this field.
		longDesc := strings.Repeat("a", maxRunes+50)
		got := usecase.BuildFieldOptionsForTest([]config.FieldOption{
			{ID: "high", Name: "High", Description: longDesc},
		})
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].Description).NotNil()
		gt.Number(t, len([]rune(got[0].Description.Text))).Equal(maxRunes)
	})

	t.Run("short description passes through unchanged", func(t *testing.T) {
		short := "Severity above SLO"
		got := usecase.BuildFieldOptionsForTest([]config.FieldOption{
			{ID: "high", Name: "High", Description: short},
		})
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].Description).NotNil()
		gt.String(t, got[0].Description.Text).Equal(short)
	})

	t.Run("blank description still yields no description object", func(t *testing.T) {
		// The "omit description when blank" branch existed before this fix
		// and must be preserved — otherwise we would emit an empty
		// description block that Slack also rejects.
		got := usecase.BuildFieldOptionsForTest([]config.FieldOption{
			{ID: "high", Name: "High", Description: ""},
		})
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].Description).Nil()
	})

	t.Run("option label is left untouched", func(t *testing.T) {
		// The fix targets only description; the option label and value
		// must continue to pass through unchanged.
		got := usecase.BuildFieldOptionsForTest([]config.FieldOption{
			{ID: "high", Name: "High", Description: strings.Repeat("x", maxRunes+10)},
		})
		gt.Array(t, got).Length(1).Required()
		gt.String(t, got[0].Text.Text).Equal("High")
		gt.String(t, got[0].Value).Equal("high")
	})
}

func TestSlackUseCases_HandleSlashCommand_Subcommands(t *testing.T) {
	i18n.Init(i18n.LangEN)

	setupCaseChannel := func(t *testing.T) (*memory.Memory, *model.WorkspaceRegistry, int64) {
		t.Helper()
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Existing Case",
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-CASE"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()
		return repo, registry, created.ID
	}

	t.Run("text=update opens case edit modal", func(t *testing.T) {
		repo, registry, _ := setupCaseChannel(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE", "", "", "update")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
	})

	t.Run("text=action opens action creation modal", func(t *testing.T) {
		repo, registry, _ := setupCaseChannel(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE", "", "", "action")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateAction)
		// Title + Description + Assignee + Status + DueDate = 5 blocks
		gt.Number(t, len(slackMock.openViewRequest.Blocks.BlockSet)).Equal(5)
	})

	t.Run("text empty opens command choice modal", func(t *testing.T) {
		repo, registry, caseID := setupCaseChannel(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE", "", "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).True()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCommandChoice)

		var meta struct {
			WorkspaceID string `json:"workspace_id"`
			ChannelID   string `json:"channel_id"`
			CaseID      int64  `json:"case_id"`
		}
		err = json.Unmarshal([]byte(slackMock.openViewRequest.PrivateMetadata), &meta)
		gt.NoError(t, err).Required()
		gt.Value(t, meta.WorkspaceID).Equal("risk")
		gt.Value(t, meta.ChannelID).Equal("C-CASE")
		gt.Value(t, meta.CaseID).Equal(caseID)
	})

	t.Run("subcommand is case-insensitive and trimmed", func(t *testing.T) {
		repo, registry, _ := setupCaseChannel(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE", "", "", "  UPDATE  ")
		gt.NoError(t, err).Required()
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
	})

	t.Run("unknown subcommand posts ephemeral and does not open view", func(t *testing.T) {
		repo, registry, _ := setupCaseChannel(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-CASE", "", "", "bogus")
		gt.NoError(t, err).Required()

		gt.Bool(t, slackMock.openViewCalled).False()
		gt.String(t, slackMock.ephemeralText).Contains("bogus")
		gt.Value(t, slackMock.ephemeralChannelID).Equal("C-CASE")
		gt.Value(t, slackMock.ephemeralUserID).Equal("U001")
	})

	t.Run("subcommand is ignored outside case channel (falls through to create flow)", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err := uc.HandleSlashCommand(context.Background(), "trigger-1", "U001", "C-NEW", "risk", "", "action")
		gt.NoError(t, err).Required()

		// Falls through to case creation (no linked case in this channel)
		gt.Value(t, slackMock.openViewRequest.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)
	})

	t.Run("private case denies non-member regardless of subcommand", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Private Case",
			IsPrivate:      true,
			ChannelUserIDs: []string{"U-MEMBER"},
		})
		gt.NoError(t, err).Required()
		created.SlackChannelID = "C-PRIV"
		_, err = repo.Case().Update(context.Background(), "risk", created)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		err = uc.HandleSlashCommand(context.Background(), "trigger-1", "U-OUTSIDER", "C-PRIV", "", "", "action")
		gt.NoError(t, err).Required()
		gt.Bool(t, slackMock.openViewCalled).False()
		gt.Value(t, slackMock.ephemeralText).Equal("You don't have access to this case.")
	})
}

func TestLifecycle_CommandChoiceToCaseEdit(t *testing.T) {
	i18n.Init(i18n.LangEN)

	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
	})
	created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
		ReporterID:  "U-TEST-DEFAULT",
		Title:       "Original Title",
		Description: "Original Desc",
		AssigneeIDs: []string{"U-OLD"},
	})
	gt.NoError(t, err).Required()

	slackMock := &commandTestSlackService{}
	slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
	caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
	seedSlackUsers(t, repo, "U-NEW")

	// Step 1: User submits the command choice modal with "update_case".
	choiceMeta, _ := json.Marshal(map[string]any{
		"workspace_id": "risk",
		"channel_id":   "C-CASE",
		"case_id":      created.ID,
	})
	choiceCallback := &goslack.InteractionCallback{
		User: goslack.User{ID: "U001"},
		View: goslack.View{
			PrivateMetadata: string(choiceMeta),
			State: &goslack.ViewState{
				Values: map[string]map[string]goslack.BlockAction{
					usecase.SlackBlockIDCommandChoice: {
						usecase.SlackActionIDCommandChoice: {
							SelectedOption: goslack.OptionBlockObject{Value: "update_case"},
						},
					},
				},
			},
		},
	}
	editView, err := slackUC.HandleCommandChoiceSubmit(context.Background(), choiceCallback)
	gt.NoError(t, err).Required()
	gt.Value(t, editView.CallbackID).Equal(usecase.SlackCallbackIDEditCase)

	// Step 2: Simulate Slack delivering the user-submitted edit modal back.
	// The view's private_metadata carries the same case_id we put in.
	editCallback := &goslack.InteractionCallback{
		User: goslack.User{ID: "U001"},
		View: goslack.View{
			CallbackID:      editView.CallbackID,
			PrivateMetadata: editView.PrivateMetadata,
			State: &goslack.ViewState{
				Values: map[string]map[string]goslack.BlockAction{
					"hc_case_title_block": {
						"hc_case_title": {Value: "New Title"},
					},
					"hc_case_desc_block": {
						"hc_case_desc": {Value: "New Desc"},
					},
					usecase.SlackBlockIDCaseAssignees: {
						usecase.SlackActionIDCaseAssignees: {
							Type:          "multi_users_select",
							SelectedUsers: []string{"U-NEW"},
						},
					},
				},
			},
		},
	}
	err = slackUC.HandleCaseEditSubmit(context.Background(), caseUC, editCallback)
	gt.NoError(t, err).Required()

	// Step 3: Verify persisted state was updated end-to-end.
	updated, err := repo.Case().Get(context.Background(), "risk", created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, updated.Title).Equal("New Title")
	gt.Value(t, updated.Description).Equal("New Desc")
	gt.Array(t, updated.AssigneeIDs).Length(1).Required()
	gt.Value(t, updated.AssigneeIDs[0]).Equal("U-NEW")
}

func TestSlackUseCases_HandleCommandChoiceSubmit(t *testing.T) {
	i18n.Init(i18n.LangEN)

	setup := func(t *testing.T) (*memory.Memory, *model.WorkspaceRegistry, int64) {
		t.Helper()
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Existing Case",
		})
		gt.NoError(t, err).Required()
		return repo, registry, created.ID
	}

	t.Run("update_case returns case edit modal view", func(t *testing.T) {
		repo, registry, caseID := setup(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      caseID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDCommandChoice: {
							usecase.SlackActionIDCommandChoice: {
								SelectedOption: goslack.OptionBlockObject{Value: "update_case"},
							},
						},
					},
				},
			},
		}

		view, err := uc.HandleCommandChoiceSubmit(context.Background(), callback)
		gt.NoError(t, err).Required()
		gt.Value(t, view).NotNil()
		gt.Value(t, view.CallbackID).Equal(usecase.SlackCallbackIDEditCase)
	})

	t.Run("create_action returns action creation modal view", func(t *testing.T) {
		repo, registry, caseID := setup(t)
		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      caseID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDCommandChoice: {
							usecase.SlackActionIDCommandChoice: {
								SelectedOption: goslack.OptionBlockObject{Value: "create_action"},
							},
						},
					},
				},
			},
		}

		view, err := uc.HandleCommandChoiceSubmit(context.Background(), callback)
		gt.NoError(t, err).Required()
		gt.Value(t, view).NotNil()
		gt.Value(t, view.CallbackID).Equal(usecase.SlackCallbackIDCreateAction)
	})

	t.Run("non-member of private case is rejected", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		created, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Private",
			IsPrivate:      true,
			ChannelUserIDs: []string{"U-MEMBER"},
		})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-PRIV",
			"case_id":      created.ID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U-OUTSIDER"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDCommandChoice: {
							usecase.SlackActionIDCommandChoice: {
								SelectedOption: goslack.OptionBlockObject{Value: "update_case"},
							},
						},
					},
				},
			},
		}

		_, err = uc.HandleCommandChoiceSubmit(context.Background(), callback)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("renders next modal in user's Slack locale", func(t *testing.T) {
		repo, registry, caseID := setup(t)
		slackMock := &commandTestSlackService{
			mockSlackService: mockSlackService{
				getUserInfoFn: func(_ context.Context, userID string) (*slack.User, error) {
					return &slack.User{ID: userID, Locale: "ja-JP"}, nil
				},
			},
		}
		uc := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      caseID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDCommandChoice: {
							usecase.SlackActionIDCommandChoice: {
								SelectedOption: goslack.OptionBlockObject{Value: "create_action"},
							},
						},
					},
				},
			},
		}

		view, err := uc.HandleCommandChoiceSubmit(context.Background(), callback)
		gt.NoError(t, err).Required()
		gt.Value(t, view).NotNil().Required()

		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		gt.Value(t, view.Title.Text).Equal(i18n.T(jaCtx, i18n.MsgModalCreateActionTitle))
		gt.Value(t, view.Title.Text).NotEqual(i18n.T(context.Background(), i18n.MsgModalCreateActionTitle))
	})
}

func TestSlackUseCases_HandleActionCreationSubmit(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("creates action with all fields", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})

		// Create the parent case linked to a channel so CreateAction's Slack
		// notification can find a target.
		caseRecord, err := repo.Case().Create(context.Background(), "risk", &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			Title:      "Parent Case",
		})
		gt.NoError(t, err).Required()
		caseRecord.SlackChannelID = "C-CASE"
		_, err = repo.Case().Update(context.Background(), "risk", caseRecord)
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		actionUC := usecase.NewActionUseCase(repo, registry, slackMock, "", nil)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      caseRecord.ID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDActionTitle: {
							usecase.SlackActionIDActionTitle: {Value: "Investigate alert"},
						},
						usecase.SlackBlockIDActionDesc: {
							usecase.SlackActionIDActionDesc: {Value: "Look at logs from yesterday"},
						},
						usecase.SlackBlockIDActionAssignee: {
							usecase.SlackActionIDActionAssignee: {SelectedUser: "U-ASSIGNEE"},
						},
						usecase.SlackBlockIDActionStatusInput: {
							usecase.SlackActionIDActionStatusIn: {
								Type: "static_select",
								SelectedOption: goslack.OptionBlockObject{
									Value: "TODO",
								},
							},
						},
						usecase.SlackBlockIDActionDueDate: {
							usecase.SlackActionIDActionDueDate: {SelectedDate: "2026-12-31"},
						},
					},
				},
			},
		}

		err = slackUC.HandleActionCreationSubmit(context.Background(), actionUC, callback)
		gt.NoError(t, err).Required()

		// Verify action persisted with the exact field values from the modal.
		actions, err := repo.Action().GetByCase(context.Background(), "risk", caseRecord.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, actions).Length(1).Required()
		gt.Value(t, actions[0].Title).Equal("Investigate alert")
		gt.Value(t, actions[0].Description).Equal("Look at logs from yesterday")
		gt.Value(t, actions[0].AssigneeID).Equal("U-ASSIGNEE")
		gt.Value(t, actions[0].Status).Equal(types.ActionStatus("TODO"))
		gt.Value(t, actions[0].DueDate).NotNil()

		// CreateAction posts the action's own Block Kit message into the
		// case channel; the slash handler does not add a separate
		// confirmation post.
		gt.Array(t, slackMock.postedMessages).Length(1).Required()
		gt.Value(t, slackMock.postedMessages[0].ChannelID).Equal("C-CASE")
		gt.String(t, slackMock.postedMessages[0].Text).Contains("Investigate alert")
	})

	t.Run("returns error when title is empty", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		caseRecord, err := repo.Case().Create(context.Background(), "risk", &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "Parent"})
		gt.NoError(t, err).Required()

		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		actionUC := usecase.NewActionUseCase(repo, registry, slackMock, "", nil)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      caseRecord.ID,
		})
		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDActionTitle: {
							usecase.SlackActionIDActionTitle: {Value: ""},
						},
					},
				},
			},
		}

		err = slackUC.HandleActionCreationSubmit(context.Background(), actionUC, callback)
		gt.Value(t, err).NotNil()
	})

	t.Run("returns error when actionUC is nil", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		slackMock := &commandTestSlackService{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		callback := &goslack.InteractionCallback{
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				PrivateMetadata: "{}",
				State:           &goslack.ViewState{Values: map[string]map[string]goslack.BlockAction{}},
			},
		}

		err := slackUC.HandleActionCreationSubmit(context.Background(), nil, callback)
		gt.Value(t, err).NotNil()
	})
}
