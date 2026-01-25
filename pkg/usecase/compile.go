package usecase

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// CompileUseCase handles knowledge compilation from data sources
type CompileUseCase struct {
	repo             interfaces.Repository
	notionService    notion.Service
	knowledgeService knowledge.Service
}

// NewCompileUseCase creates a new CompileUseCase instance
func NewCompileUseCase(repo interfaces.Repository, notionService notion.Service, knowledgeService knowledge.Service) *CompileUseCase {
	return &CompileUseCase{
		repo:             repo,
		notionService:    notionService,
		knowledgeService: knowledgeService,
	}
}

// CompileInput represents input for the compile operation
type CompileInput struct {
	SourceIDs []model.SourceID
	Since     time.Time
	Until     time.Time
}

// CompileResult represents the result of the compile operation
type CompileResult struct {
	Sources    []*model.Source    // Processed sources
	Knowledges []*model.Knowledge // Created knowledge entries
	Errors     []CompileError     // Errors encountered during processing
}

// CompileError represents an error that occurred during compilation
type CompileError struct {
	SourceID model.SourceID
	PageURL  string
	Err      error
}

func (e CompileError) Error() string {
	return e.Err.Error()
}

// Execute runs the knowledge compilation process
func (uc *CompileUseCase) Execute(ctx context.Context, input CompileInput) (*CompileResult, error) {
	logger := logging.From(ctx)

	if uc.knowledgeService == nil {
		return nil, goerr.New("knowledge service is not configured")
	}

	// Get sources to process
	sources, err := uc.getSources(ctx, input.SourceIDs)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get sources")
	}

	if len(sources) == 0 {
		logger.Info("No enabled sources to process")
		return &CompileResult{
			Sources:    []*model.Source{},
			Knowledges: []*model.Knowledge{},
			Errors:     []CompileError{},
		}, nil
	}

	// Get all risks for knowledge extraction
	risks, err := uc.repo.Risk().List(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list risks")
	}

	if len(risks) == 0 {
		logger.Info("No risks defined, skipping knowledge extraction")
		return &CompileResult{
			Sources:    sources,
			Knowledges: []*model.Knowledge{},
			Errors:     []CompileError{},
		}, nil
	}

	result := &CompileResult{
		Sources:    sources,
		Knowledges: []*model.Knowledge{},
		Errors:     []CompileError{},
	}

	// Process each source
	for _, source := range sources {
		if !source.Enabled {
			continue
		}

		switch source.SourceType {
		case model.SourceTypeNotionDB:
			knowledges, errs := uc.processNotionSource(ctx, source, risks, input.Since, input.Until)
			result.Knowledges = append(result.Knowledges, knowledges...)
			result.Errors = append(result.Errors, errs...)

		default:
			logger.Warn("Unsupported source type",
				"sourceID", source.ID,
				"sourceType", source.SourceType)
		}
	}

	return result, nil
}

// getSources retrieves sources to process
func (uc *CompileUseCase) getSources(ctx context.Context, sourceIDs []model.SourceID) ([]*model.Source, error) {
	if len(sourceIDs) > 0 {
		// Get specific sources
		var sources []*model.Source
		for _, id := range sourceIDs {
			source, err := uc.repo.Source().Get(ctx, id)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to get source", goerr.V("sourceID", id))
			}
			sources = append(sources, source)
		}
		return sources, nil
	}

	// Get all enabled sources
	allSources, err := uc.repo.Source().List(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list sources")
	}

	var enabled []*model.Source
	for _, source := range allSources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}

	return enabled, nil
}

// processSourceData extracts and saves knowledge from source data (common logic for all source types)
func (uc *CompileUseCase) processSourceData(
	ctx context.Context,
	risks []*model.Risk,
	sourceData knowledge.SourceData,
) ([]*model.Knowledge, error) {
	logger := logging.From(ctx)

	results, err := uc.knowledgeService.Extract(ctx, knowledge.Input{
		Risks:      risks,
		SourceData: sourceData,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to extract knowledge")
	}

	var knowledges []*model.Knowledge
	for _, result := range results {
		k := &model.Knowledge{
			RiskID:    result.RiskID,
			SourceID:  sourceData.SourceID,
			SourceURL: sourceData.SourceURL,
			Title:     result.Title,
			Summary:   result.Summary,
			Embedding: result.Embedding,
			SourcedAt: sourceData.SourcedAt,
		}

		created, err := uc.repo.Knowledge().Create(ctx, k)
		if err != nil {
			return knowledges, goerr.Wrap(err, "failed to create knowledge")
		}

		knowledges = append(knowledges, created)
		logger.Info("Created knowledge",
			"knowledgeID", created.ID,
			"riskID", created.RiskID,
			"title", created.Title)
	}

	return knowledges, nil
}

// processNotionSource processes a Notion database source
func (uc *CompileUseCase) processNotionSource(
	ctx context.Context,
	source *model.Source,
	risks []*model.Risk,
	since, until time.Time,
) ([]*model.Knowledge, []CompileError) {
	logger := logging.From(ctx)

	if uc.notionService == nil {
		return nil, []CompileError{{
			SourceID: source.ID,
			Err:      goerr.New("Notion service is not configured"),
		}}
	}

	if source.NotionDBConfig == nil {
		return nil, []CompileError{{
			SourceID: source.ID,
			Err:      goerr.New("Notion DB config is missing"),
		}}
	}

	var knowledges []*model.Knowledge
	var errors []CompileError

	for page, err := range uc.notionService.QueryUpdatedPages(ctx, source.NotionDBConfig.DatabaseID, since) {
		if err != nil {
			errors = append(errors, CompileError{
				SourceID: source.ID,
				Err:      goerr.Wrap(err, "failed to query pages"),
			})
			continue
		}

		if !until.IsZero() && page.LastEditedTime.After(until) {
			continue
		}

		logger.Info("Processing page",
			"sourceID", source.ID,
			"pageID", page.ID,
			"pageURL", page.URL)

		sourceData := knowledge.SourceData{
			SourceID:  source.ID,
			SourceURL: page.URL,
			SourcedAt: page.LastEditedTime,
			Content:   page.ToMarkdown(),
		}

		created, err := uc.processSourceData(ctx, risks, sourceData)
		if err != nil {
			errors = append(errors, CompileError{
				SourceID: source.ID,
				PageURL:  page.URL,
				Err:      err,
			})
			continue
		}

		knowledges = append(knowledges, created...)
	}

	return knowledges, errors
}
