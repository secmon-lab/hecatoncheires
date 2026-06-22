package auth.mcp

# Default deny: a request is authorized only when a rule below explicitly
# allows it. This is the shape an operator writes for the MCP endpoint.
default allow := false

# Allow when the Authorization header carries the shared MCP token supplied
# via the env allow-list (--mcp-env MCP_TOKEN).
allow if {
	input.req.header.Authorization[0] == sprintf("Bearer %s", [input.env.MCP_TOKEN])
}

# Act as a fixed Slack user so private-case access control can resolve the
# caller's identity downstream.
user := "U0TESTUSER" if {
	input.req.header.Authorization[0] == sprintf("Bearer %s", [input.env.MCP_TOKEN])
}
