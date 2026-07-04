package goast

# Unit tests for no_discarded_close.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

# _ = <recv>.Close()
discarded_close(recv) := {
	"Path": "pkg/x/x.go", "FileName": "x.go", "DirName": "pkg/x",
	"Kind": "AssignStmt",
	"Node": {
		"Lhs": [{"Name": "_"}],
		"Rhs": [{"Fun": {"X": {"Name": recv}, "Sel": {"Name": "Close", "NamePos": 50}}}],
	},
}

# bare statement: <recv>.Close()
bare_close(recv) := {
	"Path": "pkg/x/x.go", "FileName": "x.go", "DirName": "pkg/x",
	"Kind": "ExprStmt",
	"Node": {"X": {"Fun": {"X": {"Name": recv}, "Sel": {"Name": "Close", "NamePos": 60}}}},
}

# defer <recv>.Close()
defer_close(recv) := {
	"Path": "pkg/x/x.go", "FileName": "x.go", "DirName": "pkg/x",
	"Kind": "DeferStmt",
	"Node": {"Call": {"Fun": {"X": {"Name": recv}, "Sel": {"Name": "Close", "NamePos": 70}}}},
}

# --- discarded --------------------------------------------------------------

test_discarded_close_flagged if {
	some res in fail with input as discarded_close("c")
	res.pos == 50
	res.sev == "ERROR"
	res.msg == "do not discard x.Close(); use safe.Close(ctx, x)"
}

# _ = a, b := ... style (two LHS) is not a discarded single close.
test_multi_lhs_not_flagged if {
	count(fail) == 0 with input as json.patch(
		discarded_close("c"),
		[{"op": "add", "path": "/Node/Lhs/-", "value": {"Name": "err"}}],
	)
}

# --- bare -------------------------------------------------------------------

test_bare_close_flagged if {
	some res in fail with input as bare_close("c")
	res.pos == 60
	res.msg == "do not call x.Close() directly; use safe.Close(ctx, x)"
}

# safe.Close(...) as a bare statement is the sanctioned form — allowed.
test_bare_safe_close_allowed if {
	count(fail) == 0 with input as bare_close("safe")
}

# --- defer ------------------------------------------------------------------

test_defer_close_flagged if {
	some res in fail with input as defer_close("c")
	res.pos == 70
	res.msg == "do not defer x.Close() directly; use defer safe.Close(ctx, x)"
}

test_defer_safe_close_allowed if {
	count(fail) == 0 with input as defer_close("safe")
}

# --- scope exemptions -------------------------------------------------------

test_test_file_exempt if {
	count(fail) == 0 with input as json.patch(
		discarded_close("c"),
		[{"op": "replace", "path": "/FileName", "value": "x_test.go"}],
	)
}

test_safe_package_exempt if {
	count(fail) == 0 with input as json.patch(
		bare_close("closer"),
		[{"op": "replace", "path": "/DirName", "value": "pkg/utils/safe"}],
	)
}

# A non-Close bare method call — allowed.
test_non_close_bare_allowed if {
	count(fail) == 0 with input as json.patch(
		bare_close("c"),
		[{"op": "replace", "path": "/Node/X/Fun/Sel/Name", "value": "Flush"}],
	)
}
