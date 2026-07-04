package goast

# Ban the fmt.Print* family (Print, Printf, Println) — selectors starting with
# "Print". These write straight to stdout and bypass the project's
# context-scoped logger (logging.From(ctx)). Fprint*/Sprint* are intentionally
# not matched: Fprint* targets an explicit io.Writer and Sprint* only returns a
# string, neither of which is an unscoped stdout write.
fail contains res if {
	input.Kind == "CallExpr"
	not is_eval_out_of_scope
	input.Node.Fun.X.Name == "fmt"
	startswith(input.Node.Fun.Sel.Name, "Print")

	res := {
		"msg": sprintf("do not use fmt.%s; use the context-scoped logger (logging.From(ctx)) instead", [input.Node.Fun.Sel.Name]),
		"pos": input.Node.Fun.X.NamePos,
		"sev": "ERROR",
	}
}

# Ban the Go builtin print/println. These write unbuffered to stderr, are
# explicitly documented as debug-only aids that may be removed from the
# language, and bypass the project's context-scoped logger. Their call target
# is a bare Ident (Fun.Name), unlike the qualified fmt.Print* above.
fail contains res if {
	input.Kind == "CallExpr"
	not is_eval_out_of_scope
	builtin_print[input.Node.Fun.Name]

	res := {
		"msg": sprintf("do not use the builtin %s(); use the context-scoped logger (logging.From(ctx)) instead", [input.Node.Fun.Name]),
		"pos": input.Node.Fun.NamePos,
		"sev": "ERROR",
	}
}

builtin_print := {"print", "println"}

# The eval subsystem (`hecatoncheires eval`) is temporarily out of scope for
# these policies: its harness lives under pkg/usecase/eval, and its CLI command
# (pkg/cli/eval.go) legitimately writes to stdout (e.g. --list-tools output).
is_eval_out_of_scope if {
	contains(input.DirName, "usecase/eval")
}

is_eval_out_of_scope if {
	endswith(input.DirName, "pkg/cli")
	input.FileName == "eval.go"
}
