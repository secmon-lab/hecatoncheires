package config

import (
	"context"
	"log/slog"

	"cloud.google.com/go/storage"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
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

// Configure builds the gollem HistoryRepository and trace.Repository backed by
// Cloud Storage. The returned cleanup function closes the underlying storage
// client and must be called on shutdown. An error is returned when the bucket
// flag is empty.
func (s *Storage) Configure(ctx context.Context) (gollem.HistoryRepository, trace.Repository, func(), error) {
	if s.bucket == "" {
		return nil, nil, nil, goerr.New("--cloud-storage-bucket is required")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, nil, goerr.Wrap(err, "failed to create Cloud Storage client",
			goerr.V("bucket", s.bucket),
		)
	}

	historyRepo := agentarchive.NewCloudStorageHistoryRepository(client, s.bucket, s.prefix)
	traceRepo := agentarchive.NewCloudStorageTraceRepository(client, s.bucket, s.prefix)

	cleanup := func() {
		if err := client.Close(); err != nil {
			// Cleanup is called from main; log via the package-level logger.
			slog.Default().Error("failed to close Cloud Storage client", "error", err.Error())
		}
	}
	return historyRepo, traceRepo, cleanup, nil
}
