package auth.mcp

default allow := false

allow if {
	input.tool.name == "hecaton_list_workspaces"
}
