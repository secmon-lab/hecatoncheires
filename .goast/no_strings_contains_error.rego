package goast

# Ban discriminating errors by their text: strings.Contains / HasPrefix /
# HasSuffix applied to <expr>.Error(). Error discrimination must use
# errors.Is / errors.As against typed sentinels, not substring matching on the
# rendered message (which silently breaks when wording changes).
#
# Scope: production code only. _test.go is exempt on purpose — a test asserting
# on a third-party error's message (Slack "missing_scope", an HTTP "non-2xx",
# ...) has no typed sentinel to match and legitimately inspects the surfaced
# text. The rule targets production error-handling logic, not test assertions.
str_text_predicate := {"Contains", "HasPrefix", "HasSuffix"}

fail contains res if {
	input.Kind == "CallExpr"
	not endswith(input.FileName, "_test.go")
	input.Node.Fun.X.Name == "strings"
	str_text_predicate[input.Node.Fun.Sel.Name]

	# some argument is a call whose selector is Error() — i.e. <expr>.Error().
	# (We match the selector name only; the arg count of Error() is left
	# unchecked because a nil arg slice marshals as JSON null in live eval,
	# and count(null) would make the whole rule undefined.)
	some arg in input.Node.Args
	arg.Fun.Sel.Name == "Error"

	res := {
		"msg": sprintf("do not discriminate errors with strings.%s(err.Error()); use errors.Is/As against a typed sentinel", [input.Node.Fun.Sel.Name]),
		"pos": input.Node.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}
