package usecase

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

//go:embed prompts/assist_system.md
var assistSystemPromptTmpl string

var assistSystemPrompt = template.Must(template.New("assist_system").Parse(assistSystemPromptTmpl))

// AssistOption holds options for the assist command
type AssistOption struct {
	WorkspaceID  string
	LogCount     int
	MessageCount int
}

// AssistUseCase handles periodic AI-powered case assistance
type AssistUseCase struct {
	deps AssistDeps
}

// AssistDeps groups the dependencies AssistUseCase needs. Required fields are
// marked below; the rest can be left zero to disable the corresponding tool
// or behaviour.
//
// SlackRetriever, when supplied, lets slack__get_messages read public channels
// without bot membership via the User token (Slack contract: only user tokens
// can access public channels they are not in).
type AssistDeps struct {
	Repo     interfaces.Repository    // required
	Registry *model.WorkspaceRegistry // required
	LLM      gollem.LLMClient         // required

	// ActionUC routes core__create_action through the unified usecase entry
	// point so assist-driven creates trigger the same Slack post and event
	// records as GraphQL/Slack-modal creates. Required.
	ActionUC *ActionUseCase

	// Optional Slack tool clients. SlackService is the Bot-token client;
	// SlackSearch and SlackRetriever sit on the User OAuth Token.
	SlackService   slack.Service
	SlackSearch    slacktool.SearchService
	SlackRetriever slacktool.MessageRetriever

	// Optional integrations.
	NotionTool     notiontool.Client
	GitHubClient   *githubtool.Client
	WebFetchClient *webfetch.Client
	EmbedClient    interfaces.EmbedClient
}

// NewAssistUseCase creates a new AssistUseCase from a deps bundle. See AssistDeps.
func NewAssistUseCase(deps AssistDeps) *AssistUseCase {
	return &AssistUseCase{deps: deps}
}

// RunAssist iterates all workspaces and open cases, running the assist agent for each
func (uc *AssistUseCase) RunAssist(ctx context.Context, opts AssistOption) error {
	logger := logging.From(ctx)

	if opts.LogCount <= 0 {
		opts.LogCount = 7
	}
	if opts.MessageCount <= 0 {
		opts.MessageCount = 50
	}

	entries := uc.deps.Registry.List()

	// Filter by workspace if specified
	if opts.WorkspaceID != "" {
		entry, err := uc.deps.Registry.Get(opts.WorkspaceID)
		if err != nil {
			return goerr.Wrap(err, "specified workspace not found",
				goerr.V("workspaceID", opts.WorkspaceID),
			)
		}
		entries = []*model.WorkspaceEntry{entry}
	}

	for _, entry := range entries {
		wsID := entry.Workspace.ID
		if entry.AssistPrompt == "" {
			logger.Info("skipping workspace without [assist] config", "workspaceID", wsID)
			continue
		}

		if err := uc.processWorkspace(ctx, entry, opts); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to process workspace", goerr.V("workspaceID", wsID)), "failed to process workspace")
		}
	}

	return nil
}

func (uc *AssistUseCase) processWorkspace(ctx context.Context, entry *model.WorkspaceEntry, opts AssistOption) error {
	logger := logging.From(ctx)
	wsID := entry.Workspace.ID

	openStatus := types.CaseStatusOpen
	cases, err := uc.deps.Repo.Case().List(ctx, wsID, interfaces.WithStatus(openStatus))
	if err != nil {
		return goerr.Wrap(err, "failed to list open cases",
			goerr.V("workspaceID", wsID),
		)
	}

	logger.Info("processing workspace", "workspaceID", wsID, "openCases", len(cases))

	for _, c := range cases {
		if err := uc.processCase(ctx, entry, c, opts); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to process case",
				goerr.V("workspaceID", wsID),
				goerr.V("caseID", c.ID),
				goerr.V("caseTitle", c.Title),
			), "failed to process case")
			// Continue processing remaining cases
		}
	}

	return nil
}

func (uc *AssistUseCase) processCase(ctx context.Context, entry *model.WorkspaceEntry, c *model.Case, opts AssistOption) error {
	logger := logging.From(ctx)
	wsID := entry.Workspace.ID

	logger.Info("processing case", "workspaceID", wsID, "caseID", c.ID, "caseTitle", c.Title)

	// Build system prompt
	systemPrompt, err := uc.buildAssistSystemPrompt(ctx, entry, c, opts)
	if err != nil {
		return goerr.Wrap(err, "failed to build system prompt")
	}

	// Build tools — core (action) plus Slack (read-only + post_message)
	// plus Notion when configured.
	coreTools := core.NewForAssist(core.Deps{
		Repo:        uc.deps.Repo,
		WorkspaceID: wsID,
		CaseID:      c.ID,
		StatusSet:   entry.ActionStatusSet,
		ActionUC:    NewActionToolAdapter(uc.deps.ActionUC),
	})
	slackTools := slacktool.NewForAssist(slacktool.Deps{
		Bot:       uc.deps.SlackService,
		Search:    uc.deps.SlackSearch,
		Retriever: uc.deps.SlackRetriever,
		ChannelID: c.SlackChannelID,
	})
	notionTools := notiontool.New(notiontool.Deps{Client: uc.deps.NotionTool})
	githubTools := githubtool.New(uc.deps.GitHubClient)
	webfetchTools := webfetch.New(uc.deps.WebFetchClient)

	allTools := make([]gollem.Tool, 0, len(coreTools)+len(slackTools)+len(notionTools)+len(githubTools)+len(webfetchTools))
	allTools = append(allTools, coreTools...)
	allTools = append(allTools, slackTools...)
	allTools = append(allTools, notionTools...)
	allTools = append(allTools, githubTools...)
	allTools = append(allTools, webfetchTools...)

	// Create and execute the agent
	agent := gollem.New(uc.deps.LLM,
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithTools(allTools...),
	)

	resp, err := agent.Execute(ctx, gollem.Text(entry.AssistPrompt))
	if err != nil {
		return goerr.Wrap(err, "failed to execute assist agent",
			goerr.V("caseID", c.ID),
		)
	}

	// Generate and save execution log
	if err := uc.saveAssistLog(ctx, wsID, c.ID, entry.AssistLanguage, resp); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to save assist log",
			goerr.V("workspaceID", wsID),
			goerr.V("caseID", c.ID),
		), "failed to save assist log")
	}

	return nil
}

// assistPromptMessage represents a message for the assist system prompt template
type assistPromptMessage struct {
	Timestamp   string
	ThreadTS    string
	DisplayName string
	Text        string
}

// assistPromptAction represents an action for the assist system prompt template
type assistPromptAction struct {
	ID          int64
	Title       string
	Status      string
	StatusEmoji string
	Assignees   string
	DueDate     string
}

// assistPromptAssistLog represents a previous assist log for the template
type assistPromptAssistLog struct {
	CreatedAt string
	Summary   string
	Actions   string
	Reasoning string
	NextSteps string
}

// promptField represents a case field for template rendering.
type promptField struct {
	Name  string
	Value any
}

// assistPromptData holds all data for the assist system prompt template
type assistPromptData struct {
	CurrentTime  string
	Case         *model.Case
	Fields       []promptField
	Actions      []assistPromptAction
	Messages     []assistPromptMessage
	AssistLogs   []assistPromptAssistLog
	AssistPrompt string
	Language     string
}

func (uc *AssistUseCase) buildAssistSystemPrompt(ctx context.Context, entry *model.WorkspaceEntry, c *model.Case, opts AssistOption) (string, error) {
	wsID := entry.Workspace.ID

	data := assistPromptData{
		CurrentTime:  time.Now().UTC().Format(time.RFC3339),
		Case:         c,
		AssistPrompt: entry.AssistPrompt,
		Language:     entry.AssistLanguage,
	}

	// Build field values
	if entry.FieldSchema != nil && len(c.FieldValues) > 0 {
		fieldNames := make(map[string]string)
		for _, fd := range entry.FieldSchema.Fields {
			fieldNames[fd.ID] = fd.Name
		}
		for fieldID, fv := range c.FieldValues {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			data.Fields = append(data.Fields, promptField{Name: name, Value: fv.Value})
		}
	}

	// Fetch actions (archived actions are intentionally excluded — the
	// assist prompt summarises the active state of a case)
	actions, err := uc.deps.Repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{})
	if err != nil {
		return "", goerr.Wrap(err, "failed to get actions for case")
	}
	statusSet := resolveActionStatusSet(uc.deps.Registry, wsID)
	for _, a := range actions {
		dueDate := ""
		if a.DueDate != nil {
			dueDate = a.DueDate.Format("2006-01-02")
		}
		data.Actions = append(data.Actions, assistPromptAction{
			ID:          a.ID,
			Title:       a.Title,
			Status:      a.Status.String(),
			StatusEmoji: statusSet.Emoji(string(a.Status)),
			Assignees:   a.AssigneeID,
			DueDate:     dueDate,
		})
	}

	// Fetch recent messages from CaseMessageRepository
	msgs, _, err := uc.deps.Repo.CaseMessage().List(ctx, wsID, c.ID, opts.MessageCount, "")
	if err != nil {
		return "", goerr.Wrap(err, "failed to get case messages")
	}
	// Messages are returned newest-first; reverse for oldest-first in prompt
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		displayName := m.UserName()
		if displayName == "" {
			displayName = m.UserID()
		}
		data.Messages = append(data.Messages, assistPromptMessage{
			Timestamp:   m.EventTS(),
			ThreadTS:    m.ThreadTS(),
			DisplayName: displayName,
			Text:        m.Text(),
		})
	}

	// Fetch recent assist logs
	assistLogs, _, err := uc.deps.Repo.AssistLog().List(ctx, wsID, c.ID, opts.LogCount, 0)
	if err != nil {
		return "", goerr.Wrap(err, "failed to get assist logs")
	}
	for _, l := range assistLogs {
		data.AssistLogs = append(data.AssistLogs, assistPromptAssistLog{
			CreatedAt: l.CreatedAt.Format(time.RFC3339),
			Summary:   l.Summary,
			Actions:   l.Actions,
			Reasoning: l.Reasoning,
			NextSteps: l.NextSteps,
		})
	}

	var buf bytes.Buffer
	if err := assistSystemPrompt.Execute(&buf, data); err != nil {
		return "", goerr.Wrap(err, "failed to execute assist system prompt template")
	}

	return buf.String(), nil
}

// assistLogSummary is the JSON structure for summarizing the agent session
type assistLogSummary struct {
	Summary   string `json:"summary"`
	Actions   string `json:"actions"`
	Reasoning string `json:"reasoning"`
	NextSteps string `json:"next_steps"`
}

func (uc *AssistUseCase) saveAssistLog(ctx context.Context, wsID string, caseID int64, language string, resp *gollem.ExecuteResponse) error {
	// Build summary from agent response
	agentOutput := strings.Join(resp.Texts, "\n")

	// Create a new session with JSON response schema to generate structured summary
	schema := &gollem.Parameter{
		Title:       "AssistLogSummary",
		Description: "Structured summary of an assist agent session",
		Type:        gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"summary": {
				Type:        gollem.TypeString,
				Description: "One-line plain text summary of this session (no markdown). Keep it short and descriptive.",
				Required:    true,
			},
			"actions": {
				Type:        gollem.TypeString,
				Description: "Bulleted list of side-effecting actions taken in this session in markdown format. Only include actions that modified data, sent messages/mentions, or changed state. Do NOT include read-only or reference operations. Use '- ' prefix for each item. Empty string if no side-effecting actions were taken.",
				Required:    true,
			},
			"reasoning": {
				Type:        gollem.TypeString,
				Description: "Rationale behind decisions made in markdown format.",
				Required:    true,
			},
			"next_steps": {
				Type:        gollem.TypeString,
				Description: "Bulleted list of items to address in future sessions with clear action criteria for each in markdown format. Empty string if nothing to carry over.",
				Required:    true,
			},
		},
	}

	session, err := uc.deps.LLM.NewSession(ctx,
		gollem.WithSessionContentType(gollem.ContentTypeJSON),
		gollem.WithSessionResponseSchema(schema),
	)
	if err != nil {
		return goerr.Wrap(err, "failed to create session for assist log summary")
	}

	languageInstruction := ""
	if language != "" {
		languageInstruction = fmt.Sprintf("\nYou MUST write all output in %s.\n", language)
	}

	prompt := fmt.Sprintf(`Summarize the following assist agent session output into four parts.
All output for actions, reasoning, and next_steps MUST be in markdown format.
%s
1. summary: A single-line plain text summary of the session. No markdown.
2. actions: Bulleted list of side-effecting actions only (data changes, messages sent, mentions, state modifications). Do NOT include read-only or reference operations (e.g. reading data, checking status). Use "- " prefix. If no side-effecting actions were taken, return an empty string "".
3. reasoning: Why these actions were taken.
4. next_steps: Bulleted list of items to carry over to future sessions. Each item MUST include a clear action criteria (what condition triggers the action). If there is nothing to carry over, return an empty string "".

Agent output:
%s`, languageInstruction, agentOutput)

	summaryResp, err := session.Generate(ctx, []gollem.Input{gollem.Text(prompt)})
	if err != nil {
		return goerr.Wrap(err, "failed to generate assist log summary")
	}

	if len(summaryResp.Texts) == 0 {
		return fmt.Errorf("assist log summary generation returned empty result")
	}

	var summary assistLogSummary
	if err := json.Unmarshal([]byte(summaryResp.Texts[0]), &summary); err != nil {
		return goerr.Wrap(err, "failed to parse assist log summary JSON",
			goerr.V("response", summaryResp.Texts[0]),
		)
	}

	log := &model.AssistLog{
		CaseID:    caseID,
		Summary:   summary.Summary,
		Actions:   summary.Actions,
		Reasoning: summary.Reasoning,
		NextSteps: summary.NextSteps,
		CreatedAt: time.Now().UTC(),
	}

	if _, err := uc.deps.Repo.AssistLog().Create(ctx, wsID, caseID, log); err != nil {
		return goerr.Wrap(err, "failed to save assist log")
	}

	return nil
}
