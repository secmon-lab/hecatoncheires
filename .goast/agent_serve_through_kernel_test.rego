package goast

# Unit tests for agent_serve_through_kernel.rego (adjacent). Fixtures via
# `with input as`. Run with:  opa test .goast

# A call on a bare identifier receiver: <recv>.<sel>(...)
serve_ident_call(dir, file, recv, sel) := {
	"Path": sprintf("%s/%s", [dir, file]),
	"FileName": file,
	"DirName": dir,
	"Kind": "CallExpr",
	"Node": {"Fun": {
		"X": {"Name": recv, "NamePos": 100},
		"Sel": {"Name": sel, "NamePos": 105},
	}},
}

# A call on a field receiver: <recv>.<field>.<sel>(...)
serve_field_call(dir, file, field, sel) := {
	"Path": sprintf("%s/%s", [dir, file]),
	"FileName": file,
	"DirName": dir,
	"Kind": "CallExpr",
	"Node": {"Fun": {
		"X": {
			"X": {"Name": "uc", "NamePos": 90},
			"Sel": {"Name": field, "NamePos": 95},
		},
		"Sel": {"Name": sel, "NamePos": 105},
	}},
}

test_direct_kernel_serve_flagged if {
	some res in fail with input as serve_ident_call("pkg/cli", "serve.go", "agentKernel", "Serve")
	res.pos == 105
	res.sev == "ERROR"
	res.msg == "start the agent worker through kernel.Serve, which applies NoDuplicateSideEffects; do not call Kernel.Serve directly"
}

test_field_kernel_serve_flagged if {
	count(fail) == 1 with input as serve_field_call("pkg/usecase", "assist.go", "kernel", "Serve")
}

# The wrapper itself lives in pkg/agent/kernel and must be able to call Serve.
test_wrapper_allowed if {
	count(fail) == 0 with input as serve_ident_call("pkg/agent/kernel", "kernel.go", "k", "Serve")
}

# A test measuring the guard has to be able to bypass it.
test_test_file_allowed if {
	count(fail) == 0 with input as serve_ident_call("pkg/agent/kernel", "slot_test.go", "agentKernel", "Serve")
}

# An unrelated .Serve — the HTTP server — is not an agent worker.
test_http_server_serve_allowed if {
	count(fail) == 0 with input as serve_ident_call("pkg/controller/http", "server.go", "server", "ListenAndServe")
}

test_http_server_plain_serve_allowed if {
	count(fail) == 0 with input as serve_ident_call("pkg/controller/http", "server.go", "server", "Serve")
}

# Another method on the kernel is not the worker entry point.
test_other_kernel_method_allowed if {
	count(fail) == 0 with input as serve_ident_call("pkg/cli", "serve.go", "agentKernel", "GetProcess")
}
