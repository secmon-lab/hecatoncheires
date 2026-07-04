package sample

import "log/slog"

// Three direct slog logger calls — all flagged.
func bad() {
	slog.Info("hello")
	slog.Error("boom")
	slog.InfoContext(nil, "ctx")
}
