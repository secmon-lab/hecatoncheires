package cli

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdCompile() *cli.Command {
	var end string
	var duration string
	var sourceIDs []string
	var projectID string
	var databaseID string
	var notionToken string
	var geminiProject string
	var geminiLocation string

	return &cli.Command{
		Name:    "compile",
		Aliases: []string{"c"},
		Usage:   "Compile knowledge from data sources",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "end",
				Aliases:     []string{"e"},
				Usage:       "Data collection end time (RFC3339 format, default: now)",
				Sources:     cli.EnvVars("HECATONCHEIRES_COMPILE_END"),
				Destination: &end,
			},
			&cli.StringFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Usage:       "Collection period (e.g., 24h, 7d, 30d)",
				Value:       "24h",
				Sources:     cli.EnvVars("HECATONCHEIRES_COMPILE_DURATION"),
				Destination: &duration,
			},
			&cli.StringSliceFlag{
				Name:        "source-id",
				Usage:       "Target Source IDs (can be specified multiple times, omit for all enabled sources)",
				Destination: &sourceIDs,
			},
			&cli.StringFlag{
				Name:        "firestore-project-id",
				Usage:       "Firestore Project ID (required)",
				Required:    true,
				Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_PROJECT_ID"),
				Destination: &projectID,
			},
			&cli.StringFlag{
				Name:        "firestore-database-id",
				Usage:       "Firestore Database ID",
				Value:       "(default)",
				Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_DATABASE_ID"),
				Destination: &databaseID,
			},
			&cli.StringFlag{
				Name:        "notion-api-token",
				Usage:       "Notion API token",
				Sources:     cli.EnvVars("HECATONCHEIRES_NOTION_API_TOKEN"),
				Destination: &notionToken,
			},
			&cli.StringFlag{
				Name:        "gemini-project",
				Usage:       "Gemini Project ID",
				Required:    true,
				Sources:     cli.EnvVars("HECATONCHEIRES_GEMINI_PROJECT"),
				Destination: &geminiProject,
			},
			&cli.StringFlag{
				Name:        "gemini-location",
				Usage:       "Gemini Location",
				Value:       "us-central1",
				Sources:     cli.EnvVars("HECATONCHEIRES_GEMINI_LOCATION"),
				Destination: &geminiLocation,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.Default()

			// Parse end time
			until := time.Now().UTC()
			if end != "" {
				parsed, err := time.Parse(time.RFC3339, end)
				if err != nil {
					return goerr.Wrap(err, "failed to parse end time", goerr.V("end", end))
				}
				until = parsed
			}

			// Parse duration
			dur, err := parseDuration(duration)
			if err != nil {
				return goerr.Wrap(err, "failed to parse duration", goerr.V("duration", duration))
			}
			since := until.Add(-dur)

			logger.Info("Compile configuration",
				"since", since,
				"until", until,
				"duration", duration,
				"sourceCount", len(sourceIDs))

			// Initialize Firestore repository
			repo, err := firestore.New(ctx, projectID, databaseID)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize firestore repository")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logger.Error("failed to close firestore repository", "error", err.Error())
				}
			}()

			// Initialize Notion service
			var notionSvc notion.Service
			if notionToken != "" {
				svc, err := notion.New(notionToken)
				if err != nil {
					return goerr.Wrap(err, "failed to initialize notion service")
				}
				notionSvc = svc
				logger.Info("Notion service enabled")
			} else {
				logger.Warn("Notion API token not configured")
			}

			// Initialize Gemini LLM client
			llmClient, err := gemini.New(ctx, geminiProject, geminiLocation)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Gemini client")
			}
			logger.Info("Gemini client initialized",
				"project", geminiProject,
				"location", geminiLocation)

			// Initialize Knowledge service
			knowledgeSvc, err := knowledge.New(llmClient)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize knowledge service")
			}

			// Initialize use cases
			ucOpts := []usecase.Option{
				usecase.WithNotion(notionSvc),
				usecase.WithKnowledgeService(knowledgeSvc),
			}
			uc := usecase.New(repo, ucOpts...)

			// Parse source IDs
			var parsedSourceIDs []model.SourceID
			for _, id := range sourceIDs {
				parsedSourceIDs = append(parsedSourceIDs, model.SourceID(id))
			}

			// Execute compile
			result, err := uc.Compile.Execute(ctx, usecase.CompileInput{
				SourceIDs: parsedSourceIDs,
				Since:     since,
				Until:     until,
			})
			if err != nil {
				return goerr.Wrap(err, "failed to execute compile")
			}

			// Log results
			logger.Info("Compile completed",
				"sourcesProcessed", len(result.Sources),
				"knowledgesCreated", len(result.Knowledges),
				"errorsCount", len(result.Errors))

			for _, k := range result.Knowledges {
				logger.Info("Knowledge created",
					"id", k.ID,
					"riskID", k.RiskID,
					"title", k.Title,
					"sourceURL", k.SourceURL)
			}

			for _, e := range result.Errors {
				logger.Error("Compile error",
					"sourceID", e.SourceID,
					"pageURL", e.PageURL,
					"error", e.Err.Error())
			}

			if len(result.Errors) > 0 {
				return goerr.New("compile completed with errors", goerr.V("errorCount", len(result.Errors)))
			}

			return nil
		},
	}
}

// parseDuration parses duration string with support for days (e.g., "7d")
func parseDuration(s string) (time.Duration, error) {
	// Check for day suffix
	if len(s) > 1 && s[len(s)-1] == 'd' {
		// Parse as days
		days, err := time.ParseDuration(s[:len(s)-1] + "h")
		if err != nil {
			return 0, err
		}
		// Convert hours to days (multiply by 24)
		return days * 24, nil
	}

	// Standard duration parsing
	return time.ParseDuration(s)
}
