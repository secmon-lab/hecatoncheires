package goast

# Unit tests for repo_no_doc_converter.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

doc_func_decl(dir, name) := {
	"Path": "x.go", "FileName": "x.go", "DirName": dir,
	"Kind": "FuncDecl",
	"Node": {"Name": {"Name": name, "NamePos": 300}},
}

doc_type_spec(dir, name) := {
	"Path": "x.go", "FileName": "x.go", "DirName": dir,
	"Kind": "TypeSpec",
	"Node": {"Name": {"Name": name, "NamePos": 400}},
}

# --- converter functions ----------------------------------------------------

test_to_doc_flagged if {
	some res in fail with input as doc_func_decl("pkg/repository/firestore", "toCaseDoc")
	res.pos == 300
	res.sev == "ERROR"
	res.msg == "repository must not use doc-converter \"toCaseDoc\"; persist *model.X directly (no field-by-field copy)"
}

test_from_doc_flagged if {
	count(fail) == 1 with input as doc_func_decl("pkg/repository/memory", "fromCaseDoc")
}

# A normal function is fine.
test_ordinary_func_allowed if {
	count(fail) == 0 with input as doc_func_decl("pkg/repository/firestore", "loadCase")
}

# toCaseDoc OUTSIDE pkg/repository is out of scope.
test_converter_outside_repository_allowed if {
	count(fail) == 0 with input as doc_func_decl("pkg/usecase", "toCaseDoc")
}

# --- mirror doc types -------------------------------------------------------

test_doc_type_flagged if {
	some res in fail with input as doc_type_spec("pkg/repository/firestore", "caseDoc")
	res.pos == 400
	res.msg == "repository must not define mirror doc type \"caseDoc\"; the domain model is the wire format"
}

test_ordinary_type_allowed if {
	count(fail) == 0 with input as doc_type_spec("pkg/repository/firestore", "record")
}

test_doc_type_outside_repository_allowed if {
	count(fail) == 0 with input as doc_type_spec("pkg/domain/model", "caseDoc")
}
