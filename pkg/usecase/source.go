package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// SourceUseCase handles source-related business logic
type SourceUseCase struct {
	repo         interfaces.Repository
	notion       notion.Service
	slackService slack.Service
}

// NewSourceUseCase creates a new SourceUseCase instance
func NewSourceUseCase(repo interfaces.Repository, notionService notion.Service, slackService slack.Service) *SourceUseCase {
	return &SourceUseCase{
		repo:         repo,
		notion:       notionService,
		slackService: slackService,
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
	if input.DatabaseID == "" {
		return nil, goerr.New("database ID is required")
	}

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

// ValidateNotionDB validates a Notion database ID and returns metadata
func (uc *SourceUseCase) ValidateNotionDB(ctx context.Context, databaseID string) (*NotionDBValidationResult, error) {
	if databaseID == "" {
		return &NotionDBValidationResult{
			Valid:        false,
			ErrorMessage: "database ID is required",
		}, nil
	}

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
