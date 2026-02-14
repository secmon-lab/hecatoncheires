package config

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// Repository holds CLI flags for repository backend configuration
type Repository struct {
	backend    string
	projectID  string
	databaseID string
}

// Flags returns CLI flags for repository configuration
func (r *Repository) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "repository-backend",
			Usage:       "Repository backend type (firestore or memory)",
			Value:       "firestore",
			Sources:     cli.EnvVars("HECATONCHEIRES_REPOSITORY_BACKEND"),
			Destination: &r.backend,
		},
		&cli.StringFlag{
			Name:        "firestore-project-id",
			Usage:       "Firestore Project ID (required when using firestore backend)",
			Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_PROJECT_ID"),
			Destination: &r.projectID,
		},
		&cli.StringFlag{
			Name:        "firestore-database-id",
			Usage:       "Firestore Database ID",
			Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_DATABASE_ID"),
			Destination: &r.databaseID,
		},
	}
}

// Backend returns the configured backend type
func (r *Repository) Backend() string {
	return r.backend
}

// ProjectID returns the Firestore project ID
func (r *Repository) ProjectID() string {
	return r.projectID
}

// DatabaseID returns the Firestore database ID
func (r *Repository) DatabaseID() string {
	return r.databaseID
}

// Configure initializes and returns a repository based on the configured backend.
// The caller is responsible for calling Close() on the returned repository.
func (r *Repository) Configure(ctx context.Context) (interfaces.Repository, error) {
	switch r.backend {
	case "firestore":
		if r.projectID == "" {
			return nil, goerr.New("firestore-project-id is required when using firestore backend")
		}
		repo, err := firestore.New(ctx, r.projectID, r.databaseID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to initialize firestore repository")
		}
		logging.Default().Info("Using Firestore repository",
			"project_id", r.projectID,
			"database_id", r.databaseID,
		)
		return repo, nil

	case "memory":
		logging.Default().Info("Using in-memory repository (development mode)")
		return memory.New(), nil

	default:
		return nil, goerr.New("invalid repository backend", goerr.V("backend", r.backend))
	}
}
