package goast

# Unit tests for no_firestore_struct_tags.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

# A struct Field carrying the given raw tag literal (BasicLit, Kind 9 = STRING).
field_with_tag(tag) := {
	"Path": "pkg/domain/model/x.go",
	"FileName": "x.go",
	"DirName": "pkg/domain/model",
	"Kind": "Field",
	"Node": {"Tag": {"Value": tag, "ValuePos": 200, "Kind": 9}},
}

test_firestore_tag_flagged if {
	some res in fail with input as field_with_tag("`firestore:\"id\"`")
	res.pos == 200
	res.sev == "ERROR"
	res.msg == "do not add firestore:\"...\" struct tags; persist *model.X directly (Repository write contract)"
}

test_firestore_tag_among_others_flagged if {
	count(fail) == 1 with input as field_with_tag("`firestore:\"id\" json:\"id\"`")
}

# A json-only tag is allowed.
test_json_tag_allowed if {
	count(fail) == 0 with input as field_with_tag("`json:\"id\"`")
}

# A field with no tag at all — Tag is null — is allowed and must not error.
test_no_tag_allowed if {
	count(fail) == 0 with input as {
		"Path": "pkg/domain/model/x.go", "FileName": "x.go",
		"DirName": "pkg/domain/model", "Kind": "Field",
		"Node": {"Tag": null},
	}
}
