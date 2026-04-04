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

	// Agent
	MsgAgentThinking:      "Thinking...",
	MsgAgentAnalyzing:     "Analyzing...",
	MsgAgentProcessing:    "Processing...",
	MsgAgentInvestigating: "Investigating...",
	MsgAgentLookingInto:   "Looking into it...",
	MsgAgentOnIt:          "On it...",
	MsgAgentError:         "An error occurred while processing your request. Please try again later.",
	MsgAgentSessionInfo:   "Session Info",

	// Knowledge
	MsgKnowledgeHeader: "Knowledge: %s",
	MsgKnowledgeSource: "Source",
	MsgKnowledgeLink:   "\U0001f517 Link",

	// Bookmark
	MsgBookmarkOpenCase: "Open Case",

	// Errors
	MsgErrOpenDialog:         "Failed to open case creation dialog. Please try again.",
	MsgErrWorkspaceSelection: "Failed to process workspace selection. Please try again.",
	MsgErrCreateCase:         "Failed to create case. Please try again.",
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

	// Agent
	MsgAgentThinking:      "考え中...",
	MsgAgentAnalyzing:     "分析中...",
	MsgAgentProcessing:    "処理中...",
	MsgAgentInvestigating: "調査中...",
	MsgAgentLookingInto:   "確認中...",
	MsgAgentOnIt:          "対応中...",
	MsgAgentError:         "リクエストの処理中にエラーが発生しました。しばらくしてから再試行してください。",
	MsgAgentSessionInfo:   "セッション情報",

	// Knowledge
	MsgKnowledgeHeader: "ナレッジ: %s",
	MsgKnowledgeSource: "ソース",
	MsgKnowledgeLink:   "\U0001f517 リンク",

	// Bookmark
	MsgBookmarkOpenCase: "ケースを開く",

	// Errors
	MsgErrOpenDialog:         "ケース作成ダイアログを開けませんでした。もう一度お試しください。",
	MsgErrWorkspaceSelection: "ワークスペースの選択処理に失敗しました。もう一度お試しください。",
	MsgErrCreateCase:         "ケースの作成に失敗しました。もう一度お試しください。",
}
