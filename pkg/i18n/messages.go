package i18n

import "fmt"

func init() {
	messages = map[Lang][msgKeyCount]string{
		LangEN: messagesEN,
		LangJA: messagesJA,
	}
	defaultLang = LangEN
	for lang, table := range messages {
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
	MsgActionChangeArchived:           ":file_cabinet: %s archived action %q",
	MsgActionChangeUnarchived:         ":outbox_tray: %s unarchived action %q",
	MsgActionChangeActorSystem:        "system",
	MsgActionStepAdded:                ":heavy_plus_sign: %s added step %q",
	MsgActionStepRemoved:              ":heavy_minus_sign: %s removed step %q",
	MsgActionStepDone:                 ":white_check_mark: %s completed step %q",
	MsgActionStepReopened:             ":arrow_backward: %s reopened step %q",
	MsgActionStepRenamed:              ":pencil2: %s renamed step %q -> %q",

	// Agent
	MsgAgentThinking:      "Thinking...",
	MsgAgentAnalyzing:     "Analyzing...",
	MsgAgentProcessing:    "Processing...",
	MsgAgentInvestigating: "Investigating...",
	MsgAgentLookingInto:   "Looking into it...",
	MsgAgentOnIt:          "On it...",
	MsgAgentError:         "An error occurred while processing your request. Please try again later.",
	MsgAgentSessionInfo:   "Session Info",
	MsgKeyAgentBusy:       ":hourglass_flowing_sand: Already handling your previous request. I'll respond, then please mention me again if more is needed.",

	// Draft (open-mode) trace lines
	MsgDraftTracePlanning:           "🤔 Planning…",
	MsgDraftTracePlannerRetry:       "⚠️ Planner output rejected; retrying",
	MsgDraftTracePlannerAction:      "→ %s — %s",
	MsgDraftTracePlannerTool:        "🛠 Planning — calling %s",
	MsgDraftTracePlannerMessage:     "🤔 Planning — %s",
	MsgDraftTracePhase:              "🧭 %s",
	MsgDraftTraceTaskPending:        "⏳ Task: %s",
	MsgDraftTraceTaskRunning:        "🔍 Task: %s — running…",
	MsgDraftTraceTaskRunningTool:    "🔍 Task: %s — 🛠 calling %s",
	MsgDraftTraceTaskRunningMessage: "🔍 Task: %s — %s",
	MsgDraftTraceTaskDone:           "✅ Task: %s — done (%s, %d/%d inner loops)",
	MsgDraftTraceTaskFailedPrompt:   "❌ Task: %s — failed (%s, build prompt): %v",
	MsgDraftTraceTaskFailed:         "❌ Task: %s — failed (%s, %d/%d inner loops): %v",

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
	MsgActionChangeArchived:           ":file_cabinet: %s がアクション %q をアーカイブしました",
	MsgActionChangeUnarchived:         ":outbox_tray: %s がアクション %q のアーカイブを解除しました",
	MsgActionChangeActorSystem:        "システム",
	MsgActionStepAdded:                ":heavy_plus_sign: %s がステップ %q を追加しました",
	MsgActionStepRemoved:              ":heavy_minus_sign: %s がステップ %q を削除しました",
	MsgActionStepDone:                 ":white_check_mark: %s がステップ %q を完了しました",
	MsgActionStepReopened:             ":arrow_backward: %s がステップ %q を未完に戻しました",
	MsgActionStepRenamed:              ":pencil2: %s がステップ %q を %q に変更しました",

	// Agent
	MsgAgentThinking:      "考え中...",
	MsgAgentAnalyzing:     "分析中...",
	MsgAgentProcessing:    "処理中...",
	MsgAgentInvestigating: "調査中...",
	MsgAgentLookingInto:   "確認中...",
	MsgAgentOnIt:          "対応中...",
	MsgAgentError:         "リクエストの処理中にエラーが発生しました。しばらくしてから再試行してください。",
	MsgAgentSessionInfo:   "セッション情報",
	MsgKeyAgentBusy:       ":hourglass_flowing_sand: 直前のリクエストを処理中です。完了後にもう一度メンションしてください。",

	// Draft (open-mode) trace lines
	MsgDraftTracePlanning:           "🤔 計画中…",
	MsgDraftTracePlannerRetry:       "⚠️ 出力が拒否されました。再試行します",
	MsgDraftTracePlannerAction:      "→ %s — %s",
	MsgDraftTracePlannerTool:        "🛠 計画中 — %s 呼び出し中",
	MsgDraftTracePlannerMessage:     "🤔 計画中 — %s",
	MsgDraftTracePhase:              "🧭 %s",
	MsgDraftTraceTaskPending:        "⏳ タスク: %s",
	MsgDraftTraceTaskRunning:        "🔍 タスク: %s — 実行中…",
	MsgDraftTraceTaskRunningTool:    "🔍 タスク: %s — 🛠 %s 呼び出し中",
	MsgDraftTraceTaskRunningMessage: "🔍 タスク: %s — %s",
	MsgDraftTraceTaskDone:           "✅ タスク: %s — 完了 (%s, %d/%d 内部ループ)",
	MsgDraftTraceTaskFailedPrompt:   "❌ タスク: %s — 失敗 (%s, プロンプト構築): %v",
	MsgDraftTraceTaskFailed:         "❌ タスク: %s — 失敗 (%s, %d/%d 内部ループ): %v",

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
