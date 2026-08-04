package safe_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// capturingCtx returns a context carrying a logger that writes into the
// returned buffer, so a test can assert whether an error was reported.
func capturingCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

// errCloser is an io.Closer returning a fixed error.
type errCloser struct{ err error }

func (c *errCloser) Close() error { return c.err }

// errWriter is an io.Writer failing every write.
type errWriter struct{ err error }

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// errReader is an io.Reader failing every read.
type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestClose(t *testing.T) {
	t.Run("nil closer is a no-op", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, nil)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("successful close reports nothing", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, &errCloser{})
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("failing close is reported", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, &errCloser{err: errors.New("disk on fire")})
		gt.String(t, buf.String()).Contains("failed to close")
		gt.String(t, buf.String()).Contains("disk on fire")
	})

	t.Run("io.EOF is reported when no sentinel is given", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, &errCloser{err: io.EOF})
		gt.String(t, buf.String()).Contains("failed to close")
	})
}

func TestCloseExcept(t *testing.T) {
	t.Run("nil closer is a no-op", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.CloseExcept(ctx, nil, io.EOF)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("ignored sentinel is not reported", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.CloseExcept(ctx, &errCloser{err: io.EOF}, io.EOF)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("wrapped sentinel is matched by errors.Is", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		wrapped := goerr.Wrap(io.EOF, "stream finished")
		safe.CloseExcept(ctx, &errCloser{err: wrapped}, io.EOF)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("a different error is still reported", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.CloseExcept(ctx, &errCloser{err: errors.New("connection reset")}, io.EOF)
		gt.String(t, buf.String()).Contains("failed to close")
		gt.String(t, buf.String()).Contains("connection reset")
	})

	t.Run("any of several sentinels is ignored", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.CloseExcept(ctx, &errCloser{err: io.ErrClosedPipe}, io.EOF, io.ErrClosedPipe)
		gt.String(t, buf.String()).Equal("")
	})
}

func TestWrite(t *testing.T) {
	t.Run("nil writer is a no-op", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Write(ctx, nil, []byte("payload"))
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("successful write reports nothing and delivers the bytes", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		var out bytes.Buffer
		safe.Write(ctx, &out, []byte("payload"))
		gt.String(t, out.String()).Equal("payload")
		gt.String(t, logBuf.String()).Equal("")
	})

	t.Run("failing write is reported", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		safe.Write(ctx, &errWriter{err: errors.New("pipe closed")}, []byte("payload"))
		gt.String(t, logBuf.String()).Contains("failed to write")
		gt.String(t, logBuf.String()).Contains("pipe closed")
	})
}

func TestCopy(t *testing.T) {
	t.Run("successful copy reports nothing and delivers the bytes", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		var out bytes.Buffer
		safe.Copy(ctx, &out, strings.NewReader("payload"))
		gt.String(t, out.String()).Equal("payload")
		gt.String(t, logBuf.String()).Equal("")
	})

	t.Run("failing read is reported", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		var out bytes.Buffer
		safe.Copy(ctx, &out, &errReader{err: errors.New("source gone")})
		gt.String(t, logBuf.String()).Contains("failed to copy")
		gt.String(t, logBuf.String()).Contains("source gone")
	})
}
