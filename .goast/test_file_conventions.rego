package goast

# Two File-level test conventions.

# (a) External test package. A _test.go file must declare package <pkg>_test so
# tests exercise only the exported contract. The single exception is
# export_test.go — the sanctioned internal seam that re-exports identifiers
# under a *ForTest alias (it intentionally uses the internal package).
fail contains res if {
	input.Kind == "File"
	endswith(input.FileName, "_test.go")
	input.FileName != "export_test.go"
	not endswith(input.Node.Name.Name, "_test")

	res := {
		"msg": sprintf("test file must use an external %s_test package (found internal package %s); move internal-access seams into export_test.go", [input.Node.Name.Name, input.Node.Name.Name]),
		"pos": input.Node.Name.NamePos,
		"sev": "ERROR",
	}
}

# (b) Standard test filename. All tests for xyz.go live in xyz_test.go; suffixed
# variants like _e2e_test.go / _integration_test.go fragment the suite and are
# banned.
fail contains res if {
	input.Kind == "File"
	regex.match("_(e2e|integration)_test\\.go$", input.FileName)

	res := {
		"msg": sprintf("non-standard test filename %q; name test files xyz_test.go (one test file per source file)", [input.FileName]),
		"pos": input.Node.Package,
		"sev": "ERROR",
	}
}
