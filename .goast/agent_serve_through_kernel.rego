package goast

# Require every agent worker to start through kernel.Serve rather than calling
# agentkit's Kernel.Serve directly.
#
# kernel.Serve prepends NoDuplicateSideEffects (WithMaxUncleanReclaims(0)). That
# option is not tuning: an agentkit transition performs its effect and is
# checkpointed AFTERWARDS, so a claim that dies in between leaves a Process whose
# last checkpoint still asks for the call that already happened. agentkit's
# default lets three such takeovers re-run it, and none of this application's
# side-effecting tools is idempotent — core__create_action,
# slack__post_to_case_channel and the memo / knowledge creators all take effect
# on the first call. A forgotten option therefore means a second Action or a
# second Slack post, with nothing to indicate it happened.
#
# Matching is on the RECEIVER NAME, which is what makes this checkable from the
# syntax alone: the kernel is held as `agentKernel` (pkg/cli) or `uc.kernel`
# (pkg/usecase), and `k.Serve` inside pkg/agent/kernel is the one legitimate
# call. Anything else named *ernel calling .Serve is the mistake this catches.
#
# Deliberately NOT matched:
#   - kernel.Serve(ctx, k, ...) — the wrapper itself; its Fun is a package
#     selector (Fun.X.Name == "kernel"), not a kernel-valued receiver.
#   - http server.ListenAndServe and any other .Serve whose receiver is not a
#     kernel.
#   - _test.go files: a test that measures the guard has to be able to bypass it.
agent_kernel_receiver := {"agentKernel", "kernel"}

fail contains res if {
	input.Kind == "CallExpr"
	not endswith(input.FileName, "_test.go")
	not contains(input.DirName, "pkg/agent/kernel")

	input.Node.Fun.Sel.Name == "Serve"
	is_agent_kernel_receiver(input.Node.Fun.X)

	res := {
		"msg": "start the agent worker through kernel.Serve, which applies NoDuplicateSideEffects; do not call Kernel.Serve directly",
		"pos": input.Node.Fun.Sel.NamePos,
		"sev": "ERROR",
	}
}

# A bare identifier receiver: agentKernel.Serve(...)
is_agent_kernel_receiver(x) if {
	agent_kernel_receiver[x.Name]
}

# A field receiver: uc.kernel.Serve(...)
is_agent_kernel_receiver(x) if {
	agent_kernel_receiver[x.Sel.Name]
}
