package safe

import (
	"context"
	"io"
	"log/slog"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// Close safely closes an io.Closer and logs any errors.
// It handles nil closers gracefully.
func Close(ctx context.Context, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		logging.From(ctx).Error("Failed to close", slog.Any("error", err))
	}
}

// Write safely writes data to an io.Writer and logs any errors.
// It handles nil writers gracefully.
func Write(ctx context.Context, w io.Writer, data []byte) {
	if w == nil {
		return
	}
	if _, err := w.Write(data); err != nil {
		logging.From(ctx).Error("Failed to write", slog.Any("error", err))
	}
}

// Copy safely copies data from src to dst and logs any errors.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) {
	if _, err := io.Copy(dst, src); err != nil {
		logging.From(ctx).Error("Failed to copy", slog.Any("error", err))
	}
}
