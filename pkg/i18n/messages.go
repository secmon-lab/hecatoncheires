package i18n

import "fmt"

func init() {
	for lang, table := range map[Lang][msgKeyCount]string{
		LangEN: messagesEN,
		LangJA: messagesJA,
	} {
		for key := range MsgKey(msgKeyCount) {
			if table[key] == "" {
				panic(fmt.Sprintf("i18n: missing translation for lang=%s key=%d", lang, key))
			}
		}
	}
}

var messagesEN = [msgKeyCount]string{
	// Slash Command Modal
	MsgModalCreateCaseTitle:  "Create Case",
	MsgModalCreateCaseSubmit: "Create",
	MsgModalCreateCaseCancel: "Cancel",
	MsgModalNextButton:       "Next",
	MsgFieldTitle:            "Title",
	MsgFieldDescription:      "Description",
	MsgFieldTitlePlaceholder: "Enter case title",
	MsgFieldDescPlaceholder:  "Enter case description (optional)",
	MsgFieldWorkspace:        "Workspace",

	// Case creation confirmation
	MsgCaseCreated:            "Case #%d *%s* has been created.",
	MsgCaseCreatedWithChannel: "Case #%d *%s* has been created. Channel: <#%s>",

	// Action notifications
	MsgActionHeader:     "Action: %s %s",
	MsgActionAssignToMe: "Assign to me",
	MsgActionInProgress: "In Progress",
	MsgActionCompleted:  "Completed",
	MsgActionNoAssign:   "No Assign",
	MsgActionStatus:     "Status: %s",
	MsgActionNew:        "New action: %s",
	MsgActionUpdated:    "Action updated: %s",

	// Action interactive controls
	MsgActionOpenInWeb:           "Open in Web",
	MsgActionStatusPlaceholder:   "Select status",
	MsgActionAssigneePlaceholder: "Select assignee",

	// Action change notifications
	MsgActionChangeTitle:              ":pencil2: %s changed title: %q -> %q",
	MsgActionChangeStatus:             ":arrows_counterclockwise: %s changed status: %s -> %s",
	MsgActionChangeAssigneeAssigned:   ":bust_in_silhouette: %s assigned %s",
	MsgActionChangeAssigneeUnassigned: ":bust_in_silhouette: %s unassigned %s",
	MsgActionChangeAssigneeReplaced:   ":bust_in_silhouette: %s changed assignee: %s -> %s",
	MsgActionChangeActorSystem:        "system",

	// Agent
	MsgAgentThinking:      "Thinking...",
	MsgAgentAnalyzing:     "Analyzing...",
	MsgAgentProcessing:    "Processing...",
	MsgAgentInvestigating: "Investigating...",
	MsgAgentLookingInto:   "Looking into it...",
	MsgAgentOnIt:          "On it...",
	MsgAgentError:         "An error occurred while processing your request. Please try again later.",
	MsgAgentSessionInfo:   "Session Info",

	// Bookmark
	MsgBookmarkOpenCase: "Open Case",

	// Cross-workspace
	MsgCrossWorkspaceConnectUnavailable: "The case channel was created in a different workspace. To access it, please ask an admin to connect the channel to your workspace, or manually add it via channel settings.",

	// Edit Case Modal
	MsgModalEditCaseTitle:   "Edit Case",
	MsgModalEditCaseSubmit:  "Save",
	MsgCaseUpdated:          "Case #%d *%s* has been updated.",
	MsgErrCaseNotAccessible: "You don't have access to this case.",

	// Private case
	MsgFieldPrivateCase:     "Private case",
	MsgFieldPrivateCaseDesc: "Only channel members can access this case",

	// Case assignees
	MsgFieldCaseAssignees: "Assignees",

	// Errors
	MsgErrOpenDialog:         "Failed to open case creation dialog. Please try again.",
	MsgErrWorkspaceSelection: "Failed to process workspace selection. Please try again.",
	MsgErrCreateCase:         "Failed to create case. Please try again.",
	MsgErrEditCase:           "Failed to update case. Please try again.",

	// Command choice modal
	MsgModalCommandChoiceTitle: "Choose Action",
	MsgFieldCommandChoice:      "What would you like to do?",
	MsgChoiceUpdateCase:        "Edit case",
	MsgChoiceCreateAction:      "Create action",

	// Action creation modal
	MsgModalCreateActionTitle:      "Create Action",
	MsgModalCreateActionSubmit:     "Create",
	MsgFieldAction:                 "Action",
	MsgFieldActionTitle:            "Title",
	MsgFieldActionTitlePlaceholder: "Enter action title",
	MsgFieldActionDescription:      "Description",
	MsgFieldActionDescPlaceholder:  "Enter action description (optional)",
	MsgFieldActionAssignee:         "Assignee",
	MsgFieldActionStatusLabel:      "Status",
	MsgFieldActionDueDate:          "Due date",

	// Errors related to commands
	MsgErrUnknownSubcommand: "Unknown subcommand: %q. Available: `update`, `action`.",
	MsgErrCreateAction:      "Failed to create action. Please try again.",
}

var messagesJA = [msgKeyCount]string{
	// Slash Command Modal
	MsgModalCreateCaseTitle:  "ケース作成",
	MsgModalCreateCaseSubmit: "作成",
	MsgModalCreateCaseCancel: "キャンセル",
	MsgModalNextButton:       "次へ",
	MsgFieldTitle:            "タイトル",
	MsgFieldDescription:      "説明",
	MsgFieldTitlePlaceholder: "ケースタイトルを入力",
	MsgFieldDescPlaceholder:  "ケースの説明を入力（任意）",
	MsgFieldWorkspace:        "ワークスペース",

	// Case creation confirmation
	MsgCaseCreated:            "ケース #%d *%s* が作成されました。",
	MsgCaseCreatedWithChannel: "ケース #%d *%s* が作成されました。チャンネル: <#%s>",

	// Action notifications
	MsgActionHeader:     "アクション: %s %s",
	MsgActionAssignToMe: "自分に割り当て",
	MsgActionInProgress: "進行中",
	MsgActionCompleted:  "完了",
	MsgActionNoAssign:   "未割り当て",
	MsgActionStatus:     "ステータス: %s",
	MsgActionNew:        "新しいアクション: %s",
	MsgActionUpdated:    "アクション更新: %s",

	// Action interactive controls
	MsgActionOpenInWeb:           "Web で開く",
	MsgActionStatusPlaceholder:   "ステータスを選択",
	MsgActionAssigneePlaceholder: "担当者を選択",

	// Action change notifications
	MsgActionChangeTitle:              ":pencil2: %s がタイトルを変更しました: %q → %q",
	MsgActionChangeStatus:             ":arrows_counterclockwise: %s がステータスを変更しました: %s → %s",
	MsgActionChangeAssigneeAssigned:   ":bust_in_silhouette: %s が %s をアサインしました",
	MsgActionChangeAssigneeUnassigned: ":bust_in_silhouette: %s が %s のアサインを解除しました",
	MsgActionChangeAssigneeReplaced:   ":bust_in_silhouette: %s が担当者を変更しました: %s → %s",
	MsgActionChangeActorSystem:        "システム",

	// Agent
	MsgAgentThinking:      "考え中...",
	MsgAgentAnalyzing:     "分析中...",
	MsgAgentProcessing:    "処理中...",
	MsgAgentInvestigating: "調査中...",
	MsgAgentLookingInto:   "確認中...",
	MsgAgentOnIt:          "対応中...",
	MsgAgentError:         "リクエストの処理中にエラーが発生しました。しばらくしてから再試行してください。",
	MsgAgentSessionInfo:   "セッション情報",

	// Bookmark
	MsgBookmarkOpenCase: "ケースを開く",

	// Cross-workspace
	MsgCrossWorkspaceConnectUnavailable: "ケースチャンネルが別のワークスペースに作成されました。アクセスするには、管理者にチャンネルのコネクトを依頼するか、チャンネル設定から手動で追加してください。",

	// Edit Case Modal
	MsgModalEditCaseTitle:   "ケース編集",
	MsgModalEditCaseSubmit:  "保存",
	MsgCaseUpdated:          "ケース #%d *%s* が更新されました。",
	MsgErrCaseNotAccessible: "このケースにアクセスする権限がありません。",

	// Private case
	MsgFieldPrivateCase:     "プライベートケース",
	MsgFieldPrivateCaseDesc: "チャンネルメンバーのみアクセスできます",

	// Case assignees
	MsgFieldCaseAssignees: "担当者",

	// Errors
	MsgErrOpenDialog:         "ケース作成ダイアログを開けませんでした。もう一度お試しください。",
	MsgErrWorkspaceSelection: "ワークスペースの選択処理に失敗しました。もう一度お試しください。",
	MsgErrCreateCase:         "ケースの作成に失敗しました。もう一度お試しください。",
	MsgErrEditCase:           "ケースの更新に失敗しました。もう一度お試しください。",

	// Command choice modal
	MsgModalCommandChoiceTitle: "操作を選択",
	MsgFieldCommandChoice:      "何をしますか？",
	MsgChoiceUpdateCase:        "ケースを編集",
	MsgChoiceCreateAction:      "アクションを作成",

	// Action creation modal
	MsgModalCreateActionTitle:      "アクション作成",
	MsgModalCreateActionSubmit:     "作成",
	MsgFieldAction:                 "アクション",
	MsgFieldActionTitle:            "タイトル",
	MsgFieldActionTitlePlaceholder: "アクションタイトルを入力",
	MsgFieldActionDescription:      "説明",
	MsgFieldActionDescPlaceholder:  "アクションの説明を入力（任意）",
	MsgFieldActionAssignee:         "担当者",
	MsgFieldActionStatusLabel:      "ステータス",
	MsgFieldActionDueDate:          "期日",

	// Errors related to commands
	MsgErrUnknownSubcommand: "不明なサブコマンドです: %q。利用可能: `update`, `action`。",
	MsgErrCreateAction:      "アクションの作成に失敗しました。もう一度お試しください。",
}
