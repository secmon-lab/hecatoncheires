package goast

# Under pkg/repository, ban the mirror-"doc" pattern: doc-converter functions
# (toXxxDoc / fromXxxDoc) and mirror doc types (XxxDoc). Both encode a second
# copy of the model's shape that silently drops a field when the model grows one
# — the exact Repository write-contract bug this codebase is guarding against.
# Persist *model.X directly (Set(ctx, x) / DataTo(&x)); no field-by-field copy.

# (a) doc-converter functions: func toCaseDoc(...) / func fromCaseDoc(...)
fail contains res if {
	input.Kind == "FuncDecl"
	contains(input.DirName, "pkg/repository")
	regex.match("^(to|from)[A-Z].*Doc$", input.Node.Name.Name)

	res := {
		"msg": sprintf("repository must not use doc-converter %q; persist *model.X directly (no field-by-field copy)", [input.Node.Name.Name]),
		"pos": input.Node.Name.NamePos,
		"sev": "ERROR",
	}
}

# (b) mirror doc types: type caseDoc struct { ... }
fail contains res if {
	input.Kind == "TypeSpec"
	contains(input.DirName, "pkg/repository")
	regex.match("Doc$", input.Node.Name.Name)

	res := {
		"msg": sprintf("repository must not define mirror doc type %q; the domain model is the wire format", [input.Node.Name.Name]),
		"pos": input.Node.Name.NamePos,
		"sev": "ERROR",
	}
}
