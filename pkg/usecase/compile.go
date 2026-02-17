package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack"
)

// CompileUseCase orchestrates knowledge extraction from external sources
type CompileUseCase struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	notion            notion.Service
	knowledgeService  knowledge.Service
	slackService      slack.Service
	baseURL           string
}

// NewCompileUseCase creates a new CompileUseCase
func NewCompileUseCase(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	notionSvc notion.Service,
	knowledgeSvc knowledge.Service,
	slackSvc slack.Service,
	baseURL string,
) *CompileUseCase {
	return &CompileUseCase{
		repo:              repo,
		workspaceRegistry: registry,
		notion:            notionSvc,
		knowledgeService:  knowledgeSvc,
		slackService:      slackSvc,
		baseURL:           baseURL,
	}
}

// CompileOption holds options for the Compile operation
type CompileOption struct {
	Since       time.Time
	WorkspaceID string // If empty, process all workspaces
}

// CompileResult holds the overall result of a compile operation
type CompileResult struct {
	WorkspaceResults []WorkspaceCompileResult
}

// WorkspaceCompileResult holds the result for a single workspace
type WorkspaceCompileResult struct {
	WorkspaceID      string
	SourcesProcessed int
	PagesProcessed   int
	KnowledgeCreated int
	Notifications    int
	Errors           int
}

// Compile orchestrates the knowledge extraction process across workspaces
func (uc *CompileUseCase) Compile(ctx context.Context, opts CompileOption) (*CompileResult, error) {
	entries := uc.workspaceRegistry.List()

	result := &CompileResult{}

	for _, entry := range entries {
		wsID := entry.Workspace.ID

		// Filter by workspace if specified
		if opts.WorkspaceID != "" && wsID != opts.WorkspaceID {
			continue
		}

		wsResult, err := uc.compileWorkspace(ctx, entry, opts.Since)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to compile workspace",
				goerr.V("workspaceID", wsID))
		}

		result.WorkspaceResults = append(result.WorkspaceResults, *wsResult)

		logging.Default().Info("Workspace compile completed",
			"workspaceID", wsID,
			"sourcesProcessed", wsResult.SourcesProcessed,
			"pagesProcessed", wsResult.PagesProcessed,
			"knowledgeCreated", wsResult.KnowledgeCreated,
			"notifications", wsResult.Notifications,
			"errors", wsResult.Errors,
		)
	}

	return result, nil
}

func (uc *CompileUseCase) compileWorkspace(ctx context.Context, entry *model.WorkspaceEntry, since time.Time) (*WorkspaceCompileResult, error) {
	wsID := entry.Workspace.ID
	wsResult := &WorkspaceCompileResult{WorkspaceID: wsID}

	// Get all sources for this workspace
	sources, err := uc.repo.Source().List(ctx, wsID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list sources")
	}

	if len(sources) == 0 {
		logging.Default().Warn("No sources found for workspace", "workspaceID", wsID)
		return wsResult, nil
	}

	// Get OPEN cases for this workspace
	cases, err := uc.repo.Case().List(ctx, wsID, interfaces.WithStatus(types.CaseStatusOpen))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list cases")
	}

	if len(cases) == 0 {
		logging.Default().Info("No OPEN cases found for workspace, skipping", "workspaceID", wsID)
		return wsResult, nil
	}

	// Process each enabled source
	for _, source := range sources {
		if !source.Enabled {
			continue
		}

		var pagesProcessed, knowledgeCreated, notifications, errors int

		switch source.SourceType {
		case model.SourceTypeNotionDB:
			if source.NotionDBConfig == nil {
				continue
			}
			wsResult.SourcesProcessed++
			pagesProcessed, knowledgeCreated, notifications, errors = uc.processNotionSource(ctx, wsID, entry, source, cases, since)

		case model.SourceTypeSlack:
			if source.SlackConfig == nil || len(source.SlackConfig.Channels) == 0 {
				continue
			}
			wsResult.SourcesProcessed++
			pagesProcessed, knowledgeCreated, notifications, errors = uc.processSlackSource(ctx, wsID, entry, source, cases, since)

		default:
			continue
		}

		wsResult.PagesProcessed += pagesProcessed
		wsResult.KnowledgeCreated += knowledgeCreated
		wsResult.Notifications += notifications
		wsResult.Errors += errors
	}

	return wsResult, nil
}

func (uc *CompileUseCase) processNotionSource(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	since time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	dbID := source.NotionDBConfig.DatabaseID

	logging.Default().Info("Processing Notion source",
		"workspaceID", wsID,
		"sourceID", source.ID,
		"sourceName", source.Name,
		"databaseID", dbID,
	)

	// Pre-index cases by ID for O(1) lookup in notifySlack
	caseMap := make(map[int64]*model.Case, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}

	// Query updated pages from Notion
	for page, pageErr := range uc.notion.QueryUpdatedPages(ctx, dbID, since) {
		if pageErr != nil {
			errutil.Handle(ctx, pageErr, "failed to fetch Notion page")
			errors++
			// Page fetch error: abort this source and move to next
			break
		}

		pagesProcessed++

		// Convert page to markdown
		markdown := page.ToMarkdown()

		// Build source data
		sourceData := knowledge.SourceData{
			SourceID:   source.ID,
			SourceURLs: []string{page.URL},
			SourcedAt:  page.LastEditedTime,
			Content:    markdown,
		}

		// Extract knowledge using LLM
		input := knowledge.Input{
			Cases:      cases,
			SourceData: sourceData,
			Prompt:     entry.CompilePrompt,
		}

		results, extractErr := uc.knowledgeService.Extract(ctx, input)
		if extractErr != nil {
			errutil.Handle(ctx, extractErr, "failed to extract knowledge from page")
			errors++
			continue
		}

		// Save each result as Knowledge
		for _, result := range results {
			k := &model.Knowledge{
				CaseID:     result.CaseID,
				SourceID:   source.ID,
				SourceURLs: sourceData.SourceURLs,
				Title:      result.Title,
				Summary:    result.Summary,
				Embedding:  result.Embedding,
				SourcedAt:  sourceData.SourcedAt,
			}

			created, createErr := uc.repo.Knowledge().Create(ctx, wsID, k)
			if createErr != nil {
				errutil.Handle(ctx, createErr, "failed to save knowledge")
				errors++
				continue
			}

			knowledgeCreated++

			// Send Slack notification (best-effort)
			if uc.notifySlack(ctx, wsID, created, caseMap) {
				notifications++
			}
		}
	}

	return pagesProcessed, knowledgeCreated, notifications, errors
}

// notifySlack sends a Slack notification for newly created knowledge.
// Returns true if notification was sent successfully.
func (uc *CompileUseCase) notifySlack(ctx context.Context, wsID string, k *model.Knowledge, caseMap map[int64]*model.Case) bool {
	if uc.slackService == nil {
		return false
	}

	targetCase := caseMap[k.CaseID]
	if targetCase == nil || targetCase.SlackChannelID == "" {
		return false
	}

	blocks := uc.buildKnowledgeNotificationBlocks(wsID, k, targetCase)
	fallbackText := fmt.Sprintf("Knowledge: %s", k.Title)

	_, postErr := uc.slackService.PostMessage(ctx, targetCase.SlackChannelID, blocks, fallbackText)
	if postErr != nil {
		errutil.Handle(ctx, postErr, "failed to post Slack notification for knowledge")
		return false
	}

	return true
}

// buildKnowledgeNotificationBlocks constructs Block Kit blocks for a knowledge notification
func (uc *CompileUseCase) buildKnowledgeNotificationBlocks(wsID string, k *model.Knowledge, targetCase *model.Case) []goslack.Block {
	blocks := []goslack.Block{
		// Header: "Knowledge: {title}"
		goslack.NewHeaderBlock(
			goslack.NewTextBlockObject(goslack.PlainTextType, "Knowledge: "+k.Title, true, false),
		),
	}

	// Section: summary
	if k.Summary != "" {
		blocks = append(blocks, goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, k.Summary, false, false),
			nil, nil,
		))
	}

	// Context: source URLs
	if len(k.SourceURLs) > 0 {
		var links []string
		for _, u := range k.SourceURLs {
			links = append(links, fmt.Sprintf("<%s|Source>", u))
		}
		blocks = append(blocks, goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, "Source: "+strings.Join(links, ", "), false, false),
		))
	}

	// Actions: link button to case page
	if uc.baseURL != "" {
		caseURL := fmt.Sprintf("%s/ws/%s/cases/%d", uc.baseURL, wsID, targetCase.ID)
		linkBtn := goslack.NewButtonBlockElement("", "link_case",
			goslack.NewTextBlockObject(goslack.PlainTextType, "🔗 Link", true, false),
		)
		linkBtn.URL = caseURL
		blocks = append(blocks, goslack.NewActionBlock("", linkBtn))
	}

	return blocks
}

func (uc *CompileUseCase) processSlackSource(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	since time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	logging.Default().Info("Processing Slack source",
		"workspaceID", wsID,
		"sourceID", source.ID,
		"sourceName", source.Name,
		"channels", len(source.SlackConfig.Channels),
	)

	// Pre-index cases by ID for O(1) lookup in notifySlack
	caseMap := make(map[int64]*model.Case, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}

	now := time.Now()

	for _, ch := range source.SlackConfig.Channels {
		messages, fetchErr := uc.fetchChannelMessages(ctx, ch.ID, since, now)
		if fetchErr != nil {
			errutil.Handle(ctx, fetchErr, "failed to fetch Slack messages")
			errors++
			continue
		}

		if len(messages) == 0 {
			continue
		}

		pagesProcessed++ // 1 channel = 1 "page"

		markdown := buildThreadedMarkdown(messages, ch)
		sourceURLs := buildSlackSourceURLs(messages)

		sourceData := knowledge.SourceData{
			SourceID:   source.ID,
			SourceURLs: sourceURLs,
			SourcedAt:  now,
			Content:    markdown,
		}

		input := knowledge.Input{
			Cases:      cases,
			SourceData: sourceData,
			Prompt:     entry.CompilePrompt,
		}

		results, extractErr := uc.knowledgeService.Extract(ctx, input)
		if extractErr != nil {
			errutil.Handle(ctx, extractErr, "failed to extract knowledge from Slack messages")
			errors++
			continue
		}

		for _, result := range results {
			k := &model.Knowledge{
				CaseID:     result.CaseID,
				SourceID:   source.ID,
				SourceURLs: sourceData.SourceURLs,
				Title:      result.Title,
				Summary:    result.Summary,
				Embedding:  result.Embedding,
				SourcedAt:  sourceData.SourcedAt,
			}

			created, createErr := uc.repo.Knowledge().Create(ctx, wsID, k)
			if createErr != nil {
				errutil.Handle(ctx, createErr, "failed to save knowledge")
				errors++
				continue
			}

			knowledgeCreated++

			if uc.notifySlack(ctx, wsID, created, caseMap) {
				notifications++
			}
		}
	}

	return pagesProcessed, knowledgeCreated, notifications, errors
}

// fetchChannelMessages retrieves all messages from a channel within a time range using cursor pagination.
func (uc *CompileUseCase) fetchChannelMessages(ctx context.Context, channelID string, since, end time.Time) ([]*slackmodel.Message, error) {
	const pageSize = 100
	var allMessages []*slackmodel.Message
	cursor := ""

	for {
		messages, nextCursor, err := uc.repo.Slack().ListMessages(ctx, channelID, since, end, pageSize, cursor)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list Slack messages",
				goerr.V("channelID", channelID))
		}

		allMessages = append(allMessages, messages...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allMessages, nil
}

// buildThreadedMarkdown converts Slack messages into threaded Markdown format.
func buildThreadedMarkdown(messages []*slackmodel.Message, ch model.SlackChannel) string {
	// Group messages: root messages and their replies
	type thread struct {
		root    *slackmodel.Message
		replies []*slackmodel.Message
	}

	threadMap := make(map[string]*thread)   // key: root message ID
	var rootOrder []string                  // maintain insertion order for roots
	var orphanReplies []*slackmodel.Message // replies without a root in range

	for _, msg := range messages {
		if msg.ThreadTS() == "" {
			// Root message
			threadMap[msg.ID()] = &thread{root: msg}
			rootOrder = append(rootOrder, msg.ID())
		}
	}

	for _, msg := range messages {
		if msg.ThreadTS() != "" {
			// Reply message
			if t, ok := threadMap[msg.ThreadTS()]; ok {
				t.replies = append(t.replies, msg)
			} else {
				// Parent not in range — treat as standalone
				orphanReplies = append(orphanReplies, msg)
			}
		}
	}

	// Sort roots by CreatedAt ascending
	sort.Slice(rootOrder, func(i, j int) bool {
		return threadMap[rootOrder[i]].root.CreatedAt().Before(threadMap[rootOrder[j]].root.CreatedAt())
	})

	// Sort replies within each thread by CreatedAt ascending
	for _, t := range threadMap {
		sort.Slice(t.replies, func(i, j int) bool {
			return t.replies[i].CreatedAt().Before(t.replies[j].CreatedAt())
		})
	}

	// Sort orphan replies by CreatedAt ascending
	sort.Slice(orphanReplies, func(i, j int) bool {
		return orphanReplies[i].CreatedAt().Before(orphanReplies[j].CreatedAt())
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Slack Channel: #%s (%s)\n\n", ch.Name, ch.ID))

	first := true
	for _, rootID := range rootOrder {
		t := threadMap[rootID]
		if !first {
			sb.WriteString("\n---\n\n")
		}
		first = false

		sb.WriteString(fmt.Sprintf("## Message by %s at %s\n%s\n",
t.root.UserName(), t.root.CreatedAt().Format(time.DateTime), t.root.Text()))

		for _, reply := range t.replies {
			sb.WriteString(fmt.Sprintf("\n### Reply by %s at %s\n%s\n",
				reply.UserName(), reply.CreatedAt().Format(time.RFC3339), reply.Text()))
		}
	}

	// Append orphan replies as standalone messages
	for _, msg := range orphanReplies {
		if !first {
			sb.WriteString("\n---\n\n")
		}
		first = false

		sb.WriteString(fmt.Sprintf("## Message by %s at %s\n%s\n",
			msg.UserName(), msg.CreatedAt().Format(time.RFC3339), msg.Text()))
	}

	return sb.String()
}

// buildSlackSourceURLs constructs permalink URLs for root messages.
func buildSlackSourceURLs(messages []*slackmodel.Message) []string {
	// Get teamID from the first message
	var teamID string
	for _, msg := range messages {
		if msg.TeamID() != "" {
			teamID = msg.TeamID()
			break
		}
	}
	if teamID == "" {
		return nil
	}

	var urls []string
	for _, msg := range messages {
		if msg.ThreadTS() != "" {
			continue // Skip thread replies
		}
		// Build permalink: https://app.slack.com/client/{teamID}/{channelID}/p{timestamp_without_dot}
		tsNoDot := strings.ReplaceAll(msg.ID(), ".", "")
		url := fmt.Sprintf("https://app.slack.com/client/%s/%s/p%s", teamID, msg.ChannelID(), tsNoDot)
		urls = append(urls, url)
	}

	return urls
}
