package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
)

// SourceUseCase handles source-related business logic
type SourceUseCase struct {
	repo   interfaces.Repository
	notion notion.Service
}

// NewSourceUseCase creates a new SourceUseCase instance
func NewSourceUseCase(repo interfaces.Repository, notionService notion.Service) *SourceUseCase {
	return &SourceUseCase{
		repo:   repo,
		notion: notionService,
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
func (uc *SourceUseCase) CreateNotionDBSource(ctx context.Context, input CreateNotionDBSourceInput) (*model.Source, error) {
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

	created, err := uc.repo.Source().Create(ctx, source)
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
func (uc *SourceUseCase) UpdateSource(ctx context.Context, input UpdateSourceInput) (*model.Source, error) {
	if input.ID == "" {
		return nil, goerr.New("source ID is required")
	}

	existing, err := uc.repo.Source().Get(ctx, input.ID)
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

	updated, err := uc.repo.Source().Update(ctx, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update source")
	}

	return updated, nil
}

// DeleteSource removes a source
func (uc *SourceUseCase) DeleteSource(ctx context.Context, id model.SourceID) error {
	if id == "" {
		return goerr.New("source ID is required")
	}

	if err := uc.repo.Source().Delete(ctx, id); err != nil {
		return goerr.Wrap(err, "failed to delete source")
	}

	return nil
}

// GetSource retrieves a source by ID
func (uc *SourceUseCase) GetSource(ctx context.Context, id model.SourceID) (*model.Source, error) {
	if id == "" {
		return nil, goerr.New("source ID is required")
	}

	source, err := uc.repo.Source().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get source")
	}

	return source, nil
}

// ListSources retrieves all sources
func (uc *SourceUseCase) ListSources(ctx context.Context) ([]*model.Source, error) {
	sources, err := uc.repo.Source().List(ctx)
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
