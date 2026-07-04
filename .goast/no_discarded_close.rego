package goast

# Require safe.Close(ctx, x) for closing io.Closer resources. Ban the three
# unsafe shapes: a discarded close (_ = x.Close()), a bare close statement
# (x.Close()), and a bare deferred close (defer x.Close()). safe.Close is
# nil-safe and routes the error through errutil.Handle; the raw forms drop the
# error and panic on a nil closer.
#
# Scope: production code only. _test.go is exempt — httptest.Server.Close() and
# similar test teardown are idiomatic and carry no resource-leak risk. The
# safe package itself (pkg/utils/safe) is exempt as it implements the wrapper.

# A close call is "safe" when its receiver is the safe package: safe.Close(...).
# Undefined (helper fails) for x.Close() and for two-level receivers like
# resp.Body.Close(), so `not close_call_is_safe(call)` correctly flags those.
close_call_is_safe(call) if {
	call.Fun.X.Name == "safe"
}

# (a) discarded: _ = x.Close()
fail contains res if {
	input.Kind == "AssignStmt"
	not endswith(input.FileName, "_test.go")
	not contains(input.DirName, "pkg/utils/safe")
	count(input.Node.Lhs) == 1
	input.Node.Lhs[0].Name == "_"
	call := input.Node.Rhs[0]
	call.Fun.Sel.Name == "Close"

	res := {
		"msg": "do not discard x.Close(); use safe.Close(ctx, x)",
		"pos": call.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}

# (b) bare statement: x.Close()
fail contains res if {
	input.Kind == "ExprStmt"
	not endswith(input.FileName, "_test.go")
	not contains(input.DirName, "pkg/utils/safe")
	call := input.Node.X
	call.Fun.Sel.Name == "Close"
	not close_call_is_safe(call)

	res := {
		"msg": "do not call x.Close() directly; use safe.Close(ctx, x)",
		"pos": call.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}

# (c) bare deferred: defer x.Close()
fail contains res if {
	input.Kind == "DeferStmt"
	not endswith(input.FileName, "_test.go")
	not contains(input.DirName, "pkg/utils/safe")
	call := input.Node.Call
	call.Fun.Sel.Name == "Close"
	not close_call_is_safe(call)

	res := {
		"msg": "do not defer x.Close() directly; use defer safe.Close(ctx, x)",
		"pos": call.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}
