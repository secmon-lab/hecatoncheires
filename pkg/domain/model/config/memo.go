package config

// MemoConfig holds a workspace's memo configuration: the "strong definition"
// of what the memo records (embedded into the agent system prompt) plus the
// custom field schema that drives memo forms and validation.
//
// FieldSchema reuses the same FieldDefinition machinery as Case fields, so memo
// fields are validated by the shared FieldValidator without any memo-specific
// validation logic.
type MemoConfig struct {
	// Description is the workspace's strong definition of the memo ("what is
	// this memo for"). It is shown in the WebUI and injected into the agent
	// system prompt. Empty when the workspace does not configure memos.
	Description string
	// FieldSchema is the memo custom field schema. Nil / empty Fields means the
	// workspace has not enabled memos.
	FieldSchema *FieldSchema
}

// Enabled reports whether the workspace has a usable memo configuration (at
// least one field defined). A workspace without memo fields does not expose the
// memo feature (the WebUI hides the Memos tab and agents get no memo tools).
func (c *MemoConfig) Enabled() bool {
	return c != nil && c.FieldSchema != nil && len(c.FieldSchema.Fields) > 0
}
