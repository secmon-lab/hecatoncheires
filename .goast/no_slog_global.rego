package goast

# Ban calling the slog package directly as a logger: slog.Debug/Info/Warn/
# Error/Log and their *Context / LogAttrs variants. These bypass the project's
# context-scoped logger; every log site must obtain a logger via
# logging.From(ctx) (pkg/utils/logging).
#
# Deliberately NOT matched (these are not logger calls):
#   - attribute constructors: slog.String / Any / Int64 / Group / ...
#   - logger/handler construction: slog.New / NewJSONHandler / Default
#   - method calls on an injected *slog.Logger (logger.Info(...)) — the call
#     target there is Fun.X.Name == "logger", not "slog".
# Only the method names in banned_slog_logger_method below trip the rule.
banned_slog_logger_method := {
	"Debug", "Info", "Warn", "Error", "Log",
	"DebugContext", "InfoContext", "WarnContext", "ErrorContext", "LogAttrs",
}

fail contains res if {
	input.Kind == "CallExpr"
	input.Node.Fun.X.Name == "slog"
	banned_slog_logger_method[input.Node.Fun.Sel.Name]

	res := {
		"msg": sprintf("do not call slog.%s directly; obtain a logger via logging.From(ctx)", [input.Node.Fun.Sel.Name]),
		"pos": input.Node.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}
