package goast

# Unit tests for no_fmt_print.rego (adjacent). Self-contained: fixtures are
# supplied via `with input as`.
# Run with:  opa test .goast

# --- fixtures ---------------------------------------------------------------

# A qualified call fmt.<sel>(...) — Fun is a SelectorExpr {X: Ident, Sel: Ident}.
fmt_call(sel) := {
	"Path": "pkg/x/x.go",
	"FileName": "x.go",
	"DirName": "pkg/x",
	"Kind": "CallExpr",
	"Node": {"Fun": {
		"X": {"Name": "fmt", "NamePos": 100},
		"Sel": {"Name": sel, "NamePos": 104},
	}},
}

# A bare identifier call <name>(...) — Fun is an Ident {Name}.
ident_call(name) := {
	"Path": "pkg/x/x.go",
	"FileName": "x.go",
	"DirName": "pkg/x",
	"Kind": "CallExpr",
	"Node": {"Fun": {"Name": name, "NamePos": 42}},
}

# --- fmt.Print* -------------------------------------------------------------

test_fmt_println_flagged if {
	some res in fail with input as fmt_call("Println")
	res.pos == 100
	res.sev == "ERROR"
	res.msg == "do not use fmt.Println; use the context-scoped logger (logging.From(ctx)) instead"
}

test_fmt_print_flagged if {
	count(fail) == 1 with input as fmt_call("Print")
}

test_fmt_printf_flagged if {
	count(fail) == 1 with input as fmt_call("Printf")
}

# Sprint* only returns a string — allowed.
test_fmt_sprintf_allowed if {
	count(fail) == 0 with input as fmt_call("Sprintf")
}

# Fprint* targets an explicit io.Writer — allowed.
test_fmt_fprintf_allowed if {
	count(fail) == 0 with input as fmt_call("Fprintf")
}

# Some other fmt helper — allowed.
test_fmt_errorf_allowed if {
	count(fail) == 0 with input as fmt_call("Errorf")
}

# --- builtin print / println ------------------------------------------------

test_builtin_println_flagged if {
	some res in fail with input as ident_call("println")
	res.pos == 42
	res.sev == "ERROR"
	res.msg == "do not use the builtin println(); use the context-scoped logger (logging.From(ctx)) instead"
}

test_builtin_print_flagged if {
	count(fail) == 1 with input as ident_call("print")
}

# An ordinary function call named neither print nor println — allowed.
test_ordinary_ident_call_allowed if {
	count(fail) == 0 with input as ident_call("doWork")
}

# --- eval subsystem is out of scope -----------------------------------------

# fmt.Println inside the eval harness (pkg/usecase/eval/**) — ignored.
test_fmt_print_in_eval_harness_ignored if {
	count(fail) == 0 with input as json.patch(
		fmt_call("Println"),
		[{"op": "replace", "path": "/DirName", "value": "pkg/usecase/eval/report"}],
	)
}

# fmt.Println inside the eval CLI command (pkg/cli/eval.go) — ignored.
test_fmt_print_in_eval_cli_ignored if {
	count(fail) == 0 with input as json.patch(
		fmt_call("Println"),
		[
			{"op": "replace", "path": "/DirName", "value": "pkg/cli"},
			{"op": "replace", "path": "/FileName", "value": "eval.go"},
		],
	)
}

# A different file under pkg/cli is still subject to the rule.
test_fmt_print_in_other_cli_flagged if {
	count(fail) == 1 with input as json.patch(
		fmt_call("Println"),
		[
			{"op": "replace", "path": "/DirName", "value": "pkg/cli"},
			{"op": "replace", "path": "/FileName", "value": "serve.go"},
		],
	)
}
