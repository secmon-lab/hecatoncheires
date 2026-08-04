package safe

import (
	"context"
	"errors"
	"io"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Close safely closes an io.Closer and reports any error via errutil.Handle.
// It handles nil closers gracefully.
func Close(ctx context.Context, closer io.Closer) {
	CloseExcept(ctx, closer)
}

// CloseExcept closes an io.Closer and reports the error via errutil.Handle
// unless it matches one of the given sentinels. It exists because some Close
// implementations use a sentinel to signal a normal shutdown — BigQuery's
// managedwriter returns io.EOF from Close on a healthy stream — and reporting
// that as an error buries real failures under per-call noise. Pass no
// sentinels to get the plain Close behaviour. It handles nil closers
// gracefully.
func CloseExcept(ctx context.Context, closer io.Closer, ignore ...error) {
	if closer == nil {
		return
	}
	err := closer.Close()
	if err == nil {
		return
	}
	for _, ig := range ignore {
		if errors.Is(err, ig) {
			return
		}
	}
	errutil.Handle(ctx, goerr.Wrap(err, "failed to close"), "failed to close")
}

// Write safely writes data to an io.Writer and reports any error via errutil.Handle.
// It handles nil writers gracefully.
func Write(ctx context.Context, w io.Writer, data []byte) {
	if w == nil {
		return
	}
	if _, err := w.Write(data); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to write"), "failed to write")
	}
}

// Copy safely copies data from src to dst and reports any error via errutil.Handle.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) {
	if _, err := io.Copy(dst, src); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to copy"), "failed to copy")
	}
}
