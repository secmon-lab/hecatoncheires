package authz

// WorkspaceQuery is the Rego query evaluated for a workspace access decision.
// It names the whole package rather than data.authz.allow: a package that
// exists always evaluates to an object, so a policy whose allow rules all fail
// reads as "allow missing" (deny) instead of opaq's "no evaluation result".
const WorkspaceQuery = "data.authz"

// WorkspaceInput is the document a workspace authorization policy receives as
// `input`.
type WorkspaceInput struct {
	Workspace WorkspaceRef  `json:"workspace"`
	User      WorkspaceUser `json:"user"`
}

// WorkspaceRef identifies the workspace being accessed.
type WorkspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkspaceUser is built from the record the existing Slack user sync stores.
// Every field except ID is "" when no record exists for the user.
type WorkspaceUser struct {
	ID string `json:"id"` // Slack user ID
	// Email is tagged secret because opaq logs the policy input at debug level
	// through the project logger.
	Email       string `json:"email" masq:"secret"`
	Name        string `json:"name"`         // Slack handle (model.SlackUser.Name)
	DisplayName string `json:"display_name"` // model.SlackUser.RealName: display name, falling back to the real name
}

// WorkspaceResult is decoded from the whole authz package document; any other
// rule in the package is ignored. A missing allow decodes as false.
type WorkspaceResult struct {
	Allow bool `json:"allow"`
}
