package goast

# Unit tests for no_strings_contains_error.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

# strings.<pred>(err.Error(), ...) — Fun is strings.<pred>, first arg is the
# call err.Error() (a CallExpr whose Sel is Error).
strings_err_call(pred) := {
	"Path": "pkg/x/x.go",
	"FileName": "x.go",
	"DirName": "pkg/x",
	"Kind": "CallExpr",
	"Node": {
		"Fun": {
			"X": {"Name": "strings", "NamePos": 100},
			"Sel": {"Name": pred, "NamePos": 108},
		},
		"Args": [
			{"Fun": {"X": {"Name": "err"}, "Sel": {"Name": "Error"}}},
			{"Value": "\"x\"", "Kind": 9},
		],
	},
}

test_contains_error_flagged if {
	some res in fail with input as strings_err_call("Contains")
	res.pos == 108
	res.sev == "ERROR"
	res.msg == "do not discriminate errors with strings.Contains(err.Error()); use errors.Is/As against a typed sentinel"
}

test_hasprefix_error_flagged if {
	count(fail) == 1 with input as strings_err_call("HasPrefix")
}

test_hassuffix_error_flagged if {
	count(fail) == 1 with input as strings_err_call("HasSuffix")
}

# strings.Contains on a plain string argument (no .Error() call) — allowed.
test_contains_plain_string_allowed if {
	count(fail) == 0 with input as json.patch(
		strings_err_call("Contains"),
		[{"op": "replace", "path": "/Node/Args/0", "value": {"Name": "s"}}],
	)
}

# strings.Split etc. (not a text predicate) — allowed even with .Error().
test_non_predicate_allowed if {
	count(fail) == 0 with input as json.patch(
		strings_err_call("Contains"),
		[{"op": "replace", "path": "/Node/Fun/Sel/Name", "value": "Split"}],
	)
}

# The same violation inside a _test.go file is exempt.
test_test_file_exempt if {
	count(fail) == 0 with input as json.patch(
		strings_err_call("Contains"),
		[{"op": "replace", "path": "/FileName", "value": "x_test.go"}],
	)
}
