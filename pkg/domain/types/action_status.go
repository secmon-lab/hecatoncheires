package types

// ActionStatus is the persisted identifier of an Action status. The set of
// allowed values is no longer fixed at the type layer: it is defined per
// workspace via TOML configuration and resolved through
// `pkg/domain/model.ActionStatusSet`. The legacy constants below are kept
// only as the IDs of the default fallback set so that existing data written
// before configurable statuses (`"BACKLOG"`, `"TODO"`, ...) keeps working.
type ActionStatus string

const (
	ActionStatusBacklog    ActionStatus = "BACKLOG"
	ActionStatusTodo       ActionStatus = "TODO"
	ActionStatusInProgress ActionStatus = "IN_PROGRESS"
	ActionStatusBlocked    ActionStatus = "BLOCKED"
	ActionStatusCompleted  ActionStatus = "COMPLETED"
)

// String returns the underlying string value.
func (s ActionStatus) String() string {
	return string(s)
}
