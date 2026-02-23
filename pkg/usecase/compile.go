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
	githubsvc "github.com/secmon-lab/hecatoncheires/pkg/service/github"
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
	githubService     githubsvc.Service
	baseURL           string
}

// NewCompileUseCase creates a new CompileUseCase
func NewCompileUseCase(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	notionSvc notion.Service,
	knowledgeSvc knowledge.Service,
	slackSvc slack.Service,
	githubSvc githubsvc.Service,
	baseURL string,
) *CompileUseCase {
	return &CompileUseCase{
		repo:              repo,
		workspaceRegistry: registry,
		notion:            notionSvc,
		knowledgeService:  knowledgeSvc,
		slackService:      slackSvc,
		githubService:     githubSvc,
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

		case model.SourceTypeNotionPage:
			if source.NotionPageConfig == nil {
				continue
			}
			wsResult.SourcesProcessed++
			pagesProcessed, knowledgeCreated, notifications, errors = uc.processNotionPageSource(ctx, wsID, entry, source, cases, since)

		case model.SourceTypeSlack:
			if source.SlackConfig == nil || len(source.SlackConfig.Channels) == 0 {
				continue
			}
			wsResult.SourcesProcessed++
			pagesProcessed, knowledgeCreated, notifications, errors = uc.processSlackSource(ctx, wsID, entry, source, cases, since)

		case model.SourceTypeGitHub:
			if source.GitHubConfig == nil || len(source.GitHubConfig.Repositories) == 0 {
				continue
			}
			if uc.githubService == nil {
				logging.Default().Warn("GitHub source found but GitHub App is not configured, skipping",
					"workspaceID", wsID,
					"sourceID", source.ID,
					"sourceName", source.Name,
				)
				continue
			}
			wsResult.SourcesProcessed++
			pagesProcessed, knowledgeCreated, notifications, errors = uc.processGitHubSource(ctx, wsID, entry, source, cases, since)

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

func (uc *CompileUseCase) processNotionPageSource(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	since time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	cfg := source.NotionPageConfig
	pageID := cfg.PageID

	logging.Default().Info("Processing Notion Page source",
		"workspaceID", wsID,
		"sourceID", source.ID,
		"sourceName", source.Name,
		"pageID", pageID,
		"recursive", cfg.Recursive,
		"maxDepth", cfg.MaxDepth,
	)

	caseMap := make(map[int64]*model.Case, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}

	for page, pageErr := range uc.notion.QueryUpdatedPagesFromPage(ctx, pageID, since, cfg.Recursive, cfg.MaxDepth) {
		if pageErr != nil {
			errutil.Handle(ctx, pageErr, "failed to fetch Notion page from page source")
			errors++
			break
		}

		pagesProcessed++

		markdown := page.ToMarkdown()

		sourceData := knowledge.SourceData{
			SourceID:   source.ID,
			SourceURLs: []string{page.URL},
			SourcedAt:  page.LastEditedTime,
			Content:    markdown,
		}

		input := knowledge.Input{
			Cases:      cases,
			SourceData: sourceData,
			Prompt:     entry.CompilePrompt,
		}

		results, extractErr := uc.knowledgeService.Extract(ctx, input)
		if extractErr != nil {
			errutil.Handle(ctx, extractErr, "failed to extract knowledge from Notion page")
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
	fmt.Fprintf(&sb, "# Slack Channel: #%s (%s)\n\n", ch.Name, ch.ID)

	first := true
	for _, rootID := range rootOrder {
		t := threadMap[rootID]
		if !first {
			sb.WriteString("\n---\n\n")
		}
		first = false

		fmt.Fprintf(&sb, "## Message by %s at %s\n%s\n",
			t.root.UserName(), t.root.CreatedAt().Format(time.DateTime), t.root.Text())

		for _, reply := range t.replies {
			fmt.Fprintf(&sb, "\n### Reply by %s at %s\n%s\n",
				reply.UserName(), reply.CreatedAt().Format(time.DateTime), reply.Text())
		}
	}

	// Append orphan replies as standalone messages
	for _, msg := range orphanReplies {
		if !first {
			sb.WriteString("\n---\n\n")
		}
		first = false

		fmt.Fprintf(&sb, "## Message by %s at %s\n%s\n",
			msg.UserName(), msg.CreatedAt().Format(time.DateTime), msg.Text())
	}

	return sb.String()
}

func (uc *CompileUseCase) processGitHubSource(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	since time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	logging.Default().Info("Processing GitHub source",
		"workspaceID", wsID,
		"sourceID", source.ID,
		"sourceName", source.Name,
		"repositories", len(source.GitHubConfig.Repositories),
	)

	caseMap := make(map[int64]*model.Case, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}

	for _, repo := range source.GitHubConfig.Repositories {
		p, k, n, e := uc.processGitHubRepository(ctx, wsID, entry, source, cases, caseMap, repo, since)
		pagesProcessed += p
		knowledgeCreated += k
		notifications += n
		errors += e
	}

	return pagesProcessed, knowledgeCreated, notifications, errors
}

func (uc *CompileUseCase) processGitHubRepository(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	caseMap map[int64]*model.Case,
	repo model.GitHubRepository,
	since time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	owner := repo.Owner
	repoName := repo.Repo

	// Track processed numbers to exclude from updated comments query
	processedNumbers := make(map[int]struct{})

	// Phase 1a: Fetch recently created PRs
	for pr, fetchErr := range uc.githubService.FetchRecentPullRequests(ctx, owner, repoName, since) {
		if fetchErr != nil {
			errutil.Handle(ctx, fetchErr, "failed to fetch GitHub pull requests")
			errors++
			break
		}

		processedNumbers[pr.Number] = struct{}{}
		pagesProcessed++

		markdown := buildPRMarkdown(pr, owner, repoName)
		sourceURLs := []string{pr.URL}
		for _, c := range pr.Comments {
			sourceURLs = append(sourceURLs, c.URL)
		}

		p, k, n, e := uc.extractAndSaveKnowledge(ctx, wsID, entry, source, cases, caseMap, markdown, sourceURLs, time.Now())
		knowledgeCreated += k
		notifications += n
		errors += e
		_ = p
	}

	// Phase 1b: Fetch recently created Issues
	for issue, fetchErr := range uc.githubService.FetchRecentIssues(ctx, owner, repoName, since) {
		if fetchErr != nil {
			errutil.Handle(ctx, fetchErr, "failed to fetch GitHub issues")
			errors++
			break
		}

		processedNumbers[issue.Number] = struct{}{}
		pagesProcessed++

		markdown := buildIssueMarkdown(issue, owner, repoName)
		sourceURLs := []string{issue.URL}
		for _, c := range issue.Comments {
			sourceURLs = append(sourceURLs, c.URL)
		}

		p, k, n, e := uc.extractAndSaveKnowledge(ctx, wsID, entry, source, cases, caseMap, markdown, sourceURLs, time.Now())
		knowledgeCreated += k
		notifications += n
		errors += e
		_ = p
	}

	// Phase 2: Fetch issues/PRs with new comments
	for iwc, fetchErr := range uc.githubService.FetchUpdatedIssueComments(ctx, owner, repoName, since, processedNumbers) {
		if fetchErr != nil {
			errutil.Handle(ctx, fetchErr, "failed to fetch updated GitHub comments")
			errors++
			break
		}

		pagesProcessed++

		markdown := buildUpdatedDiscussionMarkdown(iwc, owner, repoName)
		sourceURLs := []string{iwc.URL}
		for _, c := range iwc.Comments {
			sourceURLs = append(sourceURLs, c.URL)
		}

		p, k, n, e := uc.extractAndSaveKnowledge(ctx, wsID, entry, source, cases, caseMap, markdown, sourceURLs, time.Now())
		knowledgeCreated += k
		notifications += n
		errors += e
		_ = p
	}

	return pagesProcessed, knowledgeCreated, notifications, errors
}

func (uc *CompileUseCase) extractAndSaveKnowledge(
	ctx context.Context,
	wsID string,
	entry *model.WorkspaceEntry,
	source *model.Source,
	cases []*model.Case,
	caseMap map[int64]*model.Case,
	markdown string,
	sourceURLs []string,
	sourcedAt time.Time,
) (pagesProcessed, knowledgeCreated, notifications, errors int) {
	sourceData := knowledge.SourceData{
		SourceID:   source.ID,
		SourceURLs: sourceURLs,
		SourcedAt:  sourcedAt,
		Content:    markdown,
	}

	input := knowledge.Input{
		Cases:      cases,
		SourceData: sourceData,
		Prompt:     entry.CompilePrompt,
	}

	results, extractErr := uc.knowledgeService.Extract(ctx, input)
	if extractErr != nil {
		errutil.Handle(ctx, extractErr, "failed to extract knowledge from GitHub data")
		errors++
		return
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

	return
}

// buildPRMarkdown converts a PullRequest to Markdown format
func buildPRMarkdown(pr *githubsvc.PullRequest, owner, repo string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Pull Request: %s\n\n", pr.Title)
	fmt.Fprintf(&sb, "- **Repository**: %s/%s\n", owner, repo)
	fmt.Fprintf(&sb, "- **Author**: %s\n", pr.Author)
	fmt.Fprintf(&sb, "- **State**: %s\n", pr.State)
	fmt.Fprintf(&sb, "- **Created**: %s\n", pr.CreatedAt.Format(time.DateTime))
	fmt.Fprintf(&sb, "- **URL**: %s\n", pr.URL)
	if len(pr.Labels) > 0 {
		fmt.Fprintf(&sb, "- **Labels**: %s\n", strings.Join(pr.Labels, ", "))
	}

	sb.WriteString("\n## Description\n\n")
	if pr.Body != "" {
		sb.WriteString(pr.Body)
	} else {
		sb.WriteString("(no description)")
	}
	sb.WriteString("\n")

	if len(pr.Comments) > 0 {
		fmt.Fprintf(&sb, "\n## Comments (%d comments)\n\n", len(pr.Comments))
		for i, c := range pr.Comments {
			if i > 0 {
				sb.WriteString("\n---\n\n")
			}
			fmt.Fprintf(&sb, "### Comment by %s at %s\n%s\n", c.Author, c.CreatedAt.Format(time.DateTime), c.Body)
		}
	}

	if len(pr.Reviews) > 0 {
		fmt.Fprintf(&sb, "\n## Reviews (%d reviews)\n\n", len(pr.Reviews))
		for i, r := range pr.Reviews {
			if i > 0 {
				sb.WriteString("\n---\n\n")
			}
			fmt.Fprintf(&sb, "### Review by %s at %s [%s]\n", r.Author, r.CreatedAt.Format(time.DateTime), r.State)
			if r.Body != "" {
				sb.WriteString(r.Body)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// buildIssueMarkdown converts an Issue to Markdown format
func buildIssueMarkdown(issue *githubsvc.Issue, owner, repo string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Issue: %s\n\n", issue.Title)
	fmt.Fprintf(&sb, "- **Repository**: %s/%s\n", owner, repo)
	fmt.Fprintf(&sb, "- **Author**: %s\n", issue.Author)
	fmt.Fprintf(&sb, "- **State**: %s\n", issue.State)
	fmt.Fprintf(&sb, "- **Created**: %s\n", issue.CreatedAt.Format(time.DateTime))
	fmt.Fprintf(&sb, "- **URL**: %s\n", issue.URL)
	if len(issue.Labels) > 0 {
		fmt.Fprintf(&sb, "- **Labels**: %s\n", strings.Join(issue.Labels, ", "))
	}

	sb.WriteString("\n## Description\n\n")
	if issue.Body != "" {
		sb.WriteString(issue.Body)
	} else {
		sb.WriteString("(no description)")
	}
	sb.WriteString("\n")

	if len(issue.Comments) > 0 {
		fmt.Fprintf(&sb, "\n## Comments (%d comments)\n\n", len(issue.Comments))
		for i, c := range issue.Comments {
			if i > 0 {
				sb.WriteString("\n---\n\n")
			}
			fmt.Fprintf(&sb, "### Comment by %s at %s\n%s\n", c.Author, c.CreatedAt.Format(time.DateTime), c.Body)
		}
	}

	return sb.String()
}

// buildUpdatedDiscussionMarkdown converts an IssueWithComments to Markdown format
func buildUpdatedDiscussionMarkdown(iwc *githubsvc.IssueWithComments, owner, repo string) string {
	var sb strings.Builder

	kind := "Issue"
	if iwc.IsPR {
		kind = "PR"
	}

	fmt.Fprintf(&sb, "# Updated Discussion on %s: %s\n\n", kind, iwc.Title)
	fmt.Fprintf(&sb, "- **Repository**: %s/%s\n", owner, repo)
	fmt.Fprintf(&sb, "- **URL**: %s\n", iwc.URL)
	fmt.Fprintf(&sb, "- **State**: %s\n", iwc.State)
	fmt.Fprintf(&sb, "- **Originally Created**: %s\n", iwc.CreatedAt.Format(time.DateTime))

	sb.WriteString("\n## Description\n\n")
	body := iwc.Body
	if len(body) > 2000 {
		body = body[:2000] + "\n\n...(truncated)"
	}
	if body != "" {
		sb.WriteString(body)
	} else {
		sb.WriteString("(no description)")
	}
	sb.WriteString("\n")

	if len(iwc.Comments) > 0 {
		fmt.Fprintf(&sb, "\n## Full Comment History (%d comments)\n\n", len(iwc.Comments))
		for i, c := range iwc.Comments {
			if i > 0 {
				sb.WriteString("\n---\n\n")
			}
			newMarker := ""
			if !c.CreatedAt.Before(iwc.Since) {
				newMarker = " [NEW]"
			}
			fmt.Fprintf(&sb, "### Comment by %s at %s%s\n%s\n", c.Author, c.CreatedAt.Format(time.DateTime), newMarker, c.Body)
		}
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
