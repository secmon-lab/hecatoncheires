package sample

import "log/slog"

// Attribute constructors, handler/logger construction, and method calls on a
// logger instance are all allowed — none of them is a direct slog.<level> call.
func good(l *slog.Logger) {
	_ = slog.String("k", "v")
	_ = slog.Any("obj", 1)
	h := slog.NewJSONHandler(nil, nil)
	_ = slog.New(h)
	l.Info("logging via an injected instance is fine")
}
