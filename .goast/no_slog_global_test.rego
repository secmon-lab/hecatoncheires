package goast

# Unit tests for no_slog_global.rego (adjacent). Fixtures via `with input as`.
# Run with:  opa test .goast

# A qualified call slog.<sel>(...) — Fun is a SelectorExpr {X: Ident, Sel: Ident}.
slog_call(sel) := {
	"Path": "pkg/x/x.go",
	"FileName": "x.go",
	"DirName": "pkg/x",
	"Kind": "CallExpr",
	"Node": {"Fun": {
		"X": {"Name": "slog", "NamePos": 100},
		"Sel": {"Name": sel, "NamePos": 105},
	}},
}

test_slog_info_flagged if {
	some res in fail with input as slog_call("Info")
	res.pos == 105
	res.sev == "ERROR"
	res.msg == "do not call slog.Info directly; obtain a logger via logging.From(ctx)"
}

test_slog_error_flagged if {
	count(fail) == 1 with input as slog_call("Error")
}

test_slog_warn_flagged if {
	count(fail) == 1 with input as slog_call("Warn")
}

test_slog_debug_flagged if {
	count(fail) == 1 with input as slog_call("Debug")
}

test_slog_infocontext_flagged if {
	count(fail) == 1 with input as slog_call("InfoContext")
}

test_slog_logattrs_flagged if {
	count(fail) == 1 with input as slog_call("LogAttrs")
}

# slog.With returns a logger bound to the global default — banned.
test_slog_with_flagged if {
	some res in fail with input as slog_call("With")
	res.pos == 105
	res.msg == "do not call slog.With directly; obtain a logger via logging.From(ctx)"
}

# Attribute constructors are allowed.
test_slog_string_allowed if {
	count(fail) == 0 with input as slog_call("String")
}

test_slog_any_allowed if {
	count(fail) == 0 with input as slog_call("Any")
}

# Logger / handler construction is allowed.
test_slog_new_allowed if {
	count(fail) == 0 with input as slog_call("New")
}

test_slog_newjsonhandler_allowed if {
	count(fail) == 0 with input as slog_call("NewJSONHandler")
}

# A method call on an injected logger instance (logger.Info) is not slog.Info.
test_logger_instance_info_allowed if {
	count(fail) == 0 with input as json.patch(
		slog_call("Info"),
		[{"op": "replace", "path": "/Node/Fun/X/Name", "value": "logger"}],
	)
}
