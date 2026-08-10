package config

import (
	"context"
	"log/slog"

	"cloud.google.com/go/storage"
	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/urfave/cli/v3"
)

// Storage holds CLI flags for the Cloud Storage backend used by the agent
// session archive (gollem History + Trace persistence).
type Storage struct {
	bucket string
	prefix string
}

// Flags returns the CLI flags for Cloud Storage configuration.
func (s *Storage) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "cloud-storage-bucket",
			Usage:       "Cloud Storage bucket for agent session History/Trace (required)",
			Sources:     cli.EnvVars("HECATONCHEIRES_CLOUD_STORAGE_BUCKET"),
			Destination: &s.bucket,
		},
		&cli.StringFlag{
			Name:        "cloud-storage-prefix",
			Usage:       "Object key prefix within the Cloud Storage bucket",
			Sources:     cli.EnvVars("HECATONCHEIRES_CLOUD_STORAGE_PREFIX"),
			Destination: &s.prefix,
		},
	}
}

// Bucket returns the configured bucket name.
func (s *Storage) Bucket() string { return s.bucket }

// Prefix returns the configured object key prefix.
func (s *Storage) Prefix() string { return s.prefix }

// LogAttrs returns log attributes describing the configuration.
func (s *Storage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("bucket", s.bucket),
		slog.String("prefix", s.prefix),
	}
}

// Archive bundles the Cloud Storage-backed stores the agent runtime writes to.
// They share one client, which Close releases.
type Archive struct {
	// History is the per-session gollem history store used by the pre-agentkit
	// agent runtime.
	History gollem.HistoryRepository
	// Trace is where each agent run's archive is written.
	Trace trace.Repository
	// ProcessHistory is the agentkit HistoryStore: one immutable version per
	// committed transition, so a Process's conversation rolls back with its
	// state.
	ProcessHistory agentkit.HistoryStore
	// Close releases the shared storage client and must be called on shutdown.
	Close func()
}

// Configure builds the Cloud Storage-backed archive. An error is returned when
// the bucket flag is empty.
func (s *Storage) Configure(ctx context.Context) (*Archive, error) {
	if s.bucket == "" {
		return nil, goerr.New("--cloud-storage-bucket is required")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create Cloud Storage client",
			goerr.V("bucket", s.bucket),
		)
	}

	return &Archive{
		History:        agentarchive.NewCloudStorageHistoryRepository(client, s.bucket, s.prefix),
		Trace:          agentarchive.NewCloudStorageTraceRepository(client, s.bucket, s.prefix),
		ProcessHistory: agentarchive.NewCloudStorageHistoryStore(client, s.bucket, s.prefix),
		Close: func() {
			if err := client.Close(); err != nil {
				errutil.Handle(context.Background(), goerr.Wrap(err, "failed to close Cloud Storage client"), "failed to close Cloud Storage client")
			}
		},
	}, nil
}
