package sample

import "log/slog"

// Four direct slog logger calls — all flagged. slog.With returns a logger
// bound to the global default, so it bypasses logging.From(ctx) too.
func bad() {
	slog.Info("hello")
	slog.Error("boom")
	slog.InfoContext(nil, "ctx")
	_ = slog.With("k", "v")
}
