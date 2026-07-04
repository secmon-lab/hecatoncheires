package goast

# Unit tests for test_file_conventions.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

# A File node with the given filename and package name.
file_node(fname, pkg) := {
	"Path": sprintf("pkg/x/%s", [fname]),
	"FileName": fname,
	"DirName": "pkg/x",
	"Kind": "File",
	"Node": {"Name": {"Name": pkg, "NamePos": 9}, "Package": 1},
}

# --- (a) external test package ----------------------------------------------

test_internal_test_package_flagged if {
	some res in fail with input as file_node("serve_test.go", "cli")
	res.pos == 9
	res.sev == "ERROR"
	res.msg == "test file must use an external cli_test package (found internal package cli); move internal-access seams into export_test.go"
}

test_external_test_package_allowed if {
	count(fail) == 0 with input as file_node("serve_test.go", "cli_test")
}

# export_test.go is exempt even though it uses the internal package.
test_export_test_go_exempt if {
	count(fail) == 0 with input as file_node("export_test.go", "cli")
}

# A non-test .go file is not subject to the rule at all.
test_non_test_file_ignored if {
	count(fail) == 0 with input as file_node("serve.go", "cli")
}

# --- (b) standard test filename ---------------------------------------------

test_e2e_filename_flagged if {
	some res in fail with input as file_node("widget_e2e_test.go", "cli_test")
	res.pos == 1
	res.msg == "non-standard test filename \"widget_e2e_test.go\"; name test files xyz_test.go (one test file per source file)"
}

test_integration_filename_flagged if {
	count(fail) == 1 with input as file_node("widget_integration_test.go", "cli_test")
}

test_standard_test_filename_allowed if {
	count(fail) == 0 with input as file_node("widget_test.go", "cli_test")
}
