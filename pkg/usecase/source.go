package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/github"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// ErrGitHubNotConfigured is returned when GitHub App is not configured
var ErrGitHubNotConfigured = goerr.New("GitHub App is not configured")

// SourceUseCase handles source-related business logic
type SourceUseCase struct {
	repo          interfaces.Repository
	notion        notion.Service
	slackService  slack.Service
	githubService github.Service
}

// NewSourceUseCase creates a new SourceUseCase instance
func NewSourceUseCase(repo interfaces.Repository, notionService notion.Service, slackService slack.Service, githubService github.Service) *SourceUseCase {
	return &SourceUseCase{
		repo:          repo,
		notion:        notionService,
		slackService:  slackService,
		githubService: githubService,
	}
}

// CreateNotionDBSourceInput represents input for creating a Notion DB source
type CreateNotionDBSourceInput struct {
	Name        string
	Description string
	DatabaseID  string
	Enabled     bool
}

// CreateNotionDBSource creates a new Notion DB source with validation
func (uc *SourceUseCase) CreateNotionDBSource(ctx context.Context, workspaceID string, input CreateNotionDBSourceInput) (*model.Source, error) {
	dbID, err := model.ParseNotionID(input.DatabaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid database ID or URL",
			goerr.V("input", input.DatabaseID))
	}
	input.DatabaseID = dbID

	var dbTitle, dbURL string
	if uc.notion != nil {
		metadata, err := uc.notion.GetDatabaseMetadata(ctx, input.DatabaseID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to validate Notion database")
		}
		dbTitle = metadata.Title
		dbURL = metadata.URL
	}

	name := input.Name
	if name == "" && dbTitle != "" {
		name = dbTitle
	}
	if name == "" {
		name = "Notion Database"
	}

	source := &model.Source{
		Name:        name,
		SourceType:  model.SourceTypeNotionDB,
		Description: input.Description,
		Enabled:     input.Enabled,
		NotionDBConfig: &model.NotionDBConfig{
			DatabaseID:    input.DatabaseID,
			DatabaseTitle: dbTitle,
			DatabaseURL:   dbURL,
		},
	}

	created, err := uc.repo.Source().Create(ctx, workspaceID, source)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return created, nil
}

// CreateNotionPageSourceInput represents input for creating a Notion Page source
type CreateNotionPageSourceInput struct {
	Name        string
	Description string
	PageID      string
	Enabled     bool
	Recursive   bool
	MaxDepth    int
}

// CreateNotionPageSource creates a new Notion Page source with validation
func (uc *SourceUseCase) CreateNotionPageSource(ctx context.Context, workspaceID string, input CreateNotionPageSourceInput) (*model.Source, error) {
	pageID, err := model.ParseNotionID(input.PageID)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid page ID or URL",
			goerr.V("input", input.PageID))
	}
	input.PageID = pageID

	var pageTitle, pageURL string
	if uc.notion != nil {
		metadata, err := uc.notion.GetPageMetadata(ctx, input.PageID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to validate Notion page")
		}
		pageTitle = metadata.Title
		pageURL = metadata.URL
	}

	name := input.Name
	if name == "" && pageTitle != "" {
		name = pageTitle
	}
	if name == "" {
		name = "Notion Page"
	}

	source := &model.Source{
		Name:        name,
		SourceType:  model.SourceTypeNotionPage,
		Description: input.Description,
		Enabled:     input.Enabled,
		NotionPageConfig: &model.NotionPageConfig{
			PageID:    input.PageID,
			PageTitle: pageTitle,
			PageURL:   pageURL,
			Recursive: input.Recursive,
			MaxDepth:  input.MaxDepth,
		},
	}

	created, err := uc.repo.Source().Create(ctx, workspaceID, source)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return created, nil
}

// NotionPageValidationResult represents the result of Notion page validation
type NotionPageValidationResult struct {
	Valid        bool
	PageTitle    string
	PageURL      string
	ErrorMessage string
}

// ValidateNotionPage validates a Notion page ID (or URL) and returns metadata
func (uc *SourceUseCase) ValidateNotionPage(ctx context.Context, pageID string) (*NotionPageValidationResult, error) {
	parsedID, err := model.ParseNotionID(pageID)
	if err != nil {
		return &NotionPageValidationResult{
			Valid:        false,
			ErrorMessage: "invalid page ID or URL",
		}, nil
	}
	pageID = parsedID

	if uc.notion == nil {
		return &NotionPageValidationResult{
			Valid:        false,
			ErrorMessage: "Notion service is not configured",
		}, nil
	}

	metadata, err := uc.notion.GetPageMetadata(ctx, pageID)
	if err != nil {
		return &NotionPageValidationResult{
			Valid:        false,
			ErrorMessage: "failed to get page: " + err.Error(),
		}, nil
	}

	return &NotionPageValidationResult{
		Valid:     true,
		PageTitle: metadata.Title,
		PageURL:   metadata.URL,
	}, nil
}

// UpdateSourceInput represents input for updating a source
type UpdateSourceInput struct {
	ID          model.SourceID
	Name        *string
	Description *string
	Enabled     *bool
}

// UpdateSource updates source common fields
func (uc *SourceUseCase) UpdateSource(ctx context.Context, workspaceID string, input UpdateSourceInput) (*model.Source, error) {
	if input.ID == "" {
		return nil, goerr.New("source ID is required")
	}

	existing, err := uc.repo.Source().Get(ctx, workspaceID, input.ID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get source")
	}

	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}

	updated, err := uc.repo.Source().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update source")
	}

	return updated, nil
}

// DeleteSource removes a source
func (uc *SourceUseCase) DeleteSource(ctx context.Context, workspaceID string, id model.SourceID) error {
	if id == "" {
		return goerr.New("source ID is required")
	}

	if err := uc.repo.Source().Delete(ctx, workspaceID, id); err != nil {
		return goerr.Wrap(err, "failed to delete source")
	}

	return nil
}

// GetSource retrieves a source by ID
func (uc *SourceUseCase) GetSource(ctx context.Context, workspaceID string, id model.SourceID) (*model.Source, error) {
	if id == "" {
		return nil, goerr.New("source ID is required")
	}

	source, err := uc.repo.Source().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get source")
	}

	return source, nil
}

// ListSources retrieves all sources
func (uc *SourceUseCase) ListSources(ctx context.Context, workspaceID string) ([]*model.Source, error) {
	sources, err := uc.repo.Source().List(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list sources")
	}

	return sources, nil
}

// NotionDBValidationResult represents the result of Notion database validation
type NotionDBValidationResult struct {
	Valid         bool
	DatabaseTitle string
	DatabaseURL   string
	ErrorMessage  string
}

// ValidateNotionDB validates a Notion database ID (or URL) and returns metadata
func (uc *SourceUseCase) ValidateNotionDB(ctx context.Context, databaseID string) (*NotionDBValidationResult, error) {
	parsedID, err := model.ParseNotionID(databaseID)
	if err != nil {
		return &NotionDBValidationResult{
			Valid:        false,
			ErrorMessage: "invalid database ID or URL",
		}, nil
	}
	databaseID = parsedID

	if uc.notion == nil {
		return &NotionDBValidationResult{
			Valid:        false,
			ErrorMessage: "Notion service is not configured",
		}, nil
	}

	metadata, err := uc.notion.GetDatabaseMetadata(ctx, databaseID)
	if err != nil {
		return &NotionDBValidationResult{
			Valid:        false,
			ErrorMessage: "failed to get database: " + err.Error(),
		}, nil
	}

	return &NotionDBValidationResult{
		Valid:         true,
		DatabaseTitle: metadata.Title,
		DatabaseURL:   metadata.URL,
	}, nil
}

// CreateSlackSourceInput represents input for creating a Slack source
type CreateSlackSourceInput struct {
	Name        string
	Description string
	ChannelIDs  []string
	Enabled     bool
}

// SlackChannelInfo represents channel information returned from the API
type SlackChannelInfo struct {
	ID   string
	Name string
}

// CreateSlackSource creates a new Slack source
func (uc *SourceUseCase) CreateSlackSource(ctx context.Context, workspaceID string, input CreateSlackSourceInput) (*model.Source, error) {
	if len(input.ChannelIDs) == 0 {
		return nil, goerr.New("at least one channel ID is required")
	}

	// Resolve channel names if slack service is available
	var channels []model.SlackChannel
	if uc.slackService != nil {
		names, err := uc.slackService.GetChannelNames(ctx, input.ChannelIDs)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to get channel names")
		}

		for _, id := range input.ChannelIDs {
			name := names[id]
			if name == "" {
				name = id // Use ID as fallback if name not found
			}
			channels = append(channels, model.SlackChannel{
				ID:   id,
				Name: name,
			})
		}
	} else {
		// No slack service, just store IDs without names
		for _, id := range input.ChannelIDs {
			channels = append(channels, model.SlackChannel{
				ID:   id,
				Name: id,
			})
		}
	}

	name := input.Name
	if name == "" {
		name = "Slack Source"
	}

	source := &model.Source{
		Name:        name,
		SourceType:  model.SourceTypeSlack,
		Description: input.Description,
		Enabled:     input.Enabled,
		SlackConfig: &model.SlackConfig{
			Channels: channels,
		},
	}

	created, err := uc.repo.Source().Create(ctx, workspaceID, source)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return created, nil
}

// UpdateSlackSourceInput represents input for updating a Slack source
type UpdateSlackSourceInput struct {
	ID          model.SourceID
	Name        *string
	Description *string
	ChannelIDs  []string
	Enabled     *bool
}

// UpdateSlackSource updates a Slack source
func (uc *SourceUseCase) UpdateSlackSource(ctx context.Context, workspaceID string, input UpdateSlackSourceInput) (*model.Source, error) {
	if input.ID == "" {
		return nil, goerr.New("source ID is required")
	}

	existing, err := uc.repo.Source().Get(ctx, workspaceID, input.ID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get source")
	}

	if existing.SourceType != model.SourceTypeSlack {
		return nil, goerr.New("source is not a Slack source", goerr.V("sourceType", existing.SourceType))
	}

	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}

	// Update channels if provided
	if input.ChannelIDs != nil {
		var channels []model.SlackChannel
		if uc.slackService != nil && len(input.ChannelIDs) > 0 {
			names, err := uc.slackService.GetChannelNames(ctx, input.ChannelIDs)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to get channel names")
			}

			for _, id := range input.ChannelIDs {
				name := names[id]
				if name == "" {
					name = id
				}
				channels = append(channels, model.SlackChannel{
					ID:   id,
					Name: name,
				})
			}
		} else {
			for _, id := range input.ChannelIDs {
				channels = append(channels, model.SlackChannel{
					ID:   id,
					Name: id,
				})
			}
		}
		existing.SlackConfig = &model.SlackConfig{
			Channels: channels,
		}
	}

	updated, err := uc.repo.Source().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update source")
	}

	return updated, nil
}

// ListSlackChannels lists available Slack channels
func (uc *SourceUseCase) ListSlackChannels(ctx context.Context) ([]SlackChannelInfo, error) {
	if uc.slackService == nil {
		return nil, goerr.New("Slack service is not configured")
	}

	channels, err := uc.slackService.ListJoinedChannels(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list Slack channels")
	}

	result := make([]SlackChannelInfo, len(channels))
	for i, ch := range channels {
		result[i] = SlackChannelInfo{
			ID:   ch.ID,
			Name: ch.Name,
		}
	}

	return result, nil
}

// GetSlackChannelNames retrieves channel names for given IDs
func (uc *SourceUseCase) GetSlackChannelNames(ctx context.Context, ids []string) (map[string]string, error) {
	if uc.slackService == nil {
		return nil, goerr.New("Slack service is not configured")
	}

	return uc.slackService.GetChannelNames(ctx, ids)
}

// CreateGitHubSourceInput represents input for creating a GitHub source
type CreateGitHubSourceInput struct {
	Name         string
	Description  string
	Repositories []string // "owner/repo" or GitHub URL format
	Enabled      bool
}

// CreateGitHubSource creates a new GitHub source
func (uc *SourceUseCase) CreateGitHubSource(ctx context.Context, workspaceID string, input CreateGitHubSourceInput) (*model.Source, error) {
	if uc.githubService == nil {
		return nil, ErrGitHubNotConfigured
	}

	if len(input.Repositories) == 0 {
		return nil, goerr.New("at least one repository is required")
	}

	repos, err := parseGitHubRepositories(input.Repositories)
	if err != nil {
		return nil, err
	}

	name := input.Name
	if name == "" {
		name = "GitHub Source"
	}

	source := &model.Source{
		Name:        name,
		SourceType:  model.SourceTypeGitHub,
		Description: input.Description,
		Enabled:     input.Enabled,
		GitHubConfig: &model.GitHubConfig{
			Repositories: repos,
		},
	}

	created, err := uc.repo.Source().Create(ctx, workspaceID, source)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return created, nil
}

// UpdateGitHubSourceInput represents input for updating a GitHub source
type UpdateGitHubSourceInput struct {
	ID           model.SourceID
	Name         *string
	Description  *string
	Repositories []string // "owner/repo" or GitHub URL format; nil means no change
	Enabled      *bool
}

// UpdateGitHubSource updates a GitHub source
func (uc *SourceUseCase) UpdateGitHubSource(ctx context.Context, workspaceID string, input UpdateGitHubSourceInput) (*model.Source, error) {
	if uc.githubService == nil {
		return nil, ErrGitHubNotConfigured
	}

	if input.ID == "" {
		return nil, goerr.New("source ID is required")
	}

	existing, err := uc.repo.Source().Get(ctx, workspaceID, input.ID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get source")
	}

	if existing.SourceType != model.SourceTypeGitHub {
		return nil, goerr.New("source is not a GitHub source", goerr.V("sourceType", existing.SourceType))
	}

	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}

	if input.Repositories != nil {
		repos, err := parseGitHubRepositories(input.Repositories)
		if err != nil {
			return nil, err
		}
		existing.GitHubConfig = &model.GitHubConfig{
			Repositories: repos,
		}
	}

	updated, err := uc.repo.Source().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update source")
	}

	return updated, nil
}

// GitHubRepoValidationResult represents the result of GitHub repository validation
type GitHubRepoValidationResult struct {
	Valid                bool
	Owner                string
	Repo                 string
	FullName             string
	Description          string
	IsPrivate            bool
	PullRequestCount     int
	IssueCount           int
	CanFetchPullRequests bool
	CanFetchIssues       bool
	ErrorMessage         string
}

// ValidateGitHubRepo validates a GitHub repository and returns metadata
func (uc *SourceUseCase) ValidateGitHubRepo(ctx context.Context, repository string) (*GitHubRepoValidationResult, error) {
	if uc.githubService == nil {
		return &GitHubRepoValidationResult{
			Valid:        false,
			ErrorMessage: "GitHub App is not configured",
		}, nil
	}

	owner, repo, err := model.ParseGitHubRepo(repository)
	if err != nil {
		return &GitHubRepoValidationResult{
			Valid:        false,
			ErrorMessage: "invalid repository format: use 'owner/repo' or GitHub URL",
		}, nil
	}

	validation, err := uc.githubService.ValidateRepository(ctx, owner, repo)
	if err != nil {
		return &GitHubRepoValidationResult{
			Valid:        false,
			Owner:        owner,
			Repo:         repo,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &GitHubRepoValidationResult{
		Valid:                validation.Valid,
		Owner:                validation.Owner,
		Repo:                 validation.Repo,
		FullName:             validation.FullName,
		Description:          validation.Description,
		IsPrivate:            validation.IsPrivate,
		PullRequestCount:     validation.PullRequestCount,
		IssueCount:           validation.IssueCount,
		CanFetchPullRequests: validation.CanFetchPullRequests,
		CanFetchIssues:       validation.CanFetchIssues,
		ErrorMessage:         validation.ErrorMessage,
	}, nil
}

func parseGitHubRepositories(inputs []string) ([]model.GitHubRepository, error) {
	repos := make([]model.GitHubRepository, 0, len(inputs))
	for _, input := range inputs {
		owner, repo, err := model.ParseGitHubRepo(input)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid repository",
				goerr.V("input", input))
		}
		repos = append(repos, model.GitHubRepository{
			Owner: owner,
			Repo:  repo,
		})
	}
	return repos, nil
}
