package model

import (
	"regexp"
	"strings"

	"github.com/m-mizutani/goerr/v2"
)

// ActionStatusIDPattern restricts status IDs to alphanumerics with optional
// `_` / `-` separators. Uppercase is allowed so the legacy default IDs
// (`BACKLOG`, `TODO`, ...) remain valid as DB values.
var actionStatusIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$`)

// actionStatusIDMaxLen caps status IDs at 32 chars. Status IDs are embedded
// in Slack `static_select` option `value` fields together with a workspace ID
// and an action ID (`workspaceID:actionID:statusID`). Slack enforces a 75-char
// limit on `value`; capping IDs here keeps the composite well within bounds
// even with a long workspace slug and a 19-digit action ID.
const actionStatusIDMaxLen = 32

// ActionStatusColorPreset is a semantic preset name. The actual color value is
// resolved on the frontend; the backend only validates membership.
type ActionStatusColorPreset string

const (
	ActionStatusColorIdle        ActionStatusColorPreset = "idle"
	ActionStatusColorActive      ActionStatusColorPreset = "active"
	ActionStatusColorWaiting     ActionStatusColorPreset = "waiting"
	ActionStatusColorPaused      ActionStatusColorPreset = "paused"
	ActionStatusColorAttention   ActionStatusColorPreset = "attention"
	ActionStatusColorBlocked     ActionStatusColorPreset = "blocked"
	ActionStatusColorSuccess     ActionStatusColorPreset = "success"
	ActionStatusColorNeutralDone ActionStatusColorPreset = "neutral_done"
	ActionStatusColorFailure     ActionStatusColorPreset = "failure"
)

func allActionStatusColorPresets() []ActionStatusColorPreset {
	return []ActionStatusColorPreset{
		ActionStatusColorIdle,
		ActionStatusColorActive,
		ActionStatusColorWaiting,
		ActionStatusColorPaused,
		ActionStatusColorAttention,
		ActionStatusColorBlocked,
		ActionStatusColorSuccess,
		ActionStatusColorNeutralDone,
		ActionStatusColorFailure,
	}
}

var hexColorPattern = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)

// ActionStatusDefinition describes a single status definable per workspace.
type ActionStatusDefinition struct {
	ID          string
	Name        string
	Description string
	Color       string // either a preset name (lowercase) or "#RRGGBB" / "#RGB"
	Emoji       string
}

// ActionStatusSet is the resolved per-workspace status configuration.
// It is constructed once at startup and treated as immutable thereafter.
type ActionStatusSet struct {
	initialID string
	closedIDs map[string]struct{}
	statuses  []ActionStatusDefinition
	idIndex   map[string]int // statuses[idIndex[id]] == status
}

// NewActionStatusSet constructs an ActionStatusSet and validates the input.
func NewActionStatusSet(initialID string, closedIDs []string, statuses []ActionStatusDefinition) (*ActionStatusSet, error) {
	if len(statuses) == 0 {
		return nil, goerr.New("action status set must contain at least one status")
	}

	idx := make(map[string]int, len(statuses))
	for i, s := range statuses {
		if !actionStatusIDPattern.MatchString(s.ID) {
			return nil, goerr.New("invalid action status id",
				goerr.V("status_id", s.ID),
				goerr.V("pattern", actionStatusIDPattern.String()))
		}
		if len(s.ID) > actionStatusIDMaxLen {
			return nil, goerr.New("action status id exceeds maximum length",
				goerr.V("status_id", s.ID),
				goerr.V("max_len", actionStatusIDMaxLen))
		}
		if s.Name == "" {
			return nil, goerr.New("action status name is required",
				goerr.V("status_id", s.ID))
		}
		if _, dup := idx[s.ID]; dup {
			return nil, goerr.New("duplicate action status id", goerr.V("status_id", s.ID))
		}
		if err := validateActionStatusColor(s.Color); err != nil {
			return nil, goerr.Wrap(err, "invalid color for action status",
				goerr.V("status_id", s.ID))
		}
		idx[s.ID] = i
	}

	if initialID == "" {
		return nil, goerr.New("initial action status is required")
	}
	if _, ok := idx[initialID]; !ok {
		return nil, goerr.New("initial action status does not match any defined status",
			goerr.V("initial_id", initialID))
	}

	closedSet := make(map[string]struct{}, len(closedIDs))
	for _, id := range closedIDs {
		if _, ok := idx[id]; !ok {
			return nil, goerr.New("closed action status does not match any defined status",
				goerr.V("closed_id", id))
		}
		closedSet[id] = struct{}{}
	}

	clone := make([]ActionStatusDefinition, len(statuses))
	copy(clone, statuses)

	return &ActionStatusSet{
		initialID: initialID,
		closedIDs: closedSet,
		statuses:  clone,
		idIndex:   idx,
	}, nil
}

func validateActionStatusColor(color string) error {
	if color == "" {
		return nil
	}
	if hexColorPattern.MatchString(color) {
		return nil
	}
	for _, preset := range allActionStatusColorPresets() {
		if strings.EqualFold(string(preset), color) {
			return nil
		}
	}
	return goerr.New("color must be a preset name or #RRGGBB / #RGB hex code",
		goerr.V("color", color))
}

// Initial returns the initial status definition.
func (s *ActionStatusSet) Initial() ActionStatusDefinition {
	return s.statuses[s.idIndex[s.initialID]]
}

// InitialID returns the initial status ID.
func (s *ActionStatusSet) InitialID() string {
	return s.initialID
}

// ClosedIDs returns the IDs flagged as closed, in insertion order of `statuses`.
func (s *ActionStatusSet) ClosedIDs() []string {
	out := make([]string, 0, len(s.closedIDs))
	for _, def := range s.statuses {
		if _, ok := s.closedIDs[def.ID]; ok {
			out = append(out, def.ID)
		}
	}
	return out
}

// IsClosed reports whether the given status id is configured as closed.
func (s *ActionStatusSet) IsClosed(id string) bool {
	_, ok := s.closedIDs[id]
	return ok
}

// IsValid reports whether the given id matches a defined status.
func (s *ActionStatusSet) IsValid(id string) bool {
	_, ok := s.idIndex[id]
	return ok
}

// Get returns the definition for the given id.
func (s *ActionStatusSet) Get(id string) (ActionStatusDefinition, bool) {
	i, ok := s.idIndex[id]
	if !ok {
		return ActionStatusDefinition{}, false
	}
	return s.statuses[i], true
}

// Statuses returns a copy of all status definitions in declaration order.
func (s *ActionStatusSet) Statuses() []ActionStatusDefinition {
	out := make([]ActionStatusDefinition, len(s.statuses))
	copy(out, s.statuses)
	return out
}

// IDs returns all status ids in declaration order.
func (s *ActionStatusSet) IDs() []string {
	out := make([]string, len(s.statuses))
	for i, d := range s.statuses {
		out[i] = d.ID
	}
	return out
}

// Emoji returns the emoji for the given id, or a fallback if unknown / empty.
func (s *ActionStatusSet) Emoji(id string) string {
	if def, ok := s.Get(id); ok && def.Emoji != "" {
		return def.Emoji
	}
	return "❓" // ❓
}

// DefaultActionStatusSet returns the legacy 5-status set used when a workspace
// does not define its own statuses. Kept here so storage values from before
// configurable statuses (`"BACKLOG"`, `"TODO"`, ...) keep working unchanged.
func DefaultActionStatusSet() *ActionStatusSet {
	defs := []ActionStatusDefinition{
		{ID: "BACKLOG", Name: "Backlog", Color: "idle", Emoji: "\U0001F4CB"},
		{ID: "TODO", Name: "To Do", Color: "idle", Emoji: "\U0001F4CC"},
		{ID: "IN_PROGRESS", Name: "In Progress", Color: "active", Emoji: "▶️"},
		{ID: "BLOCKED", Name: "Blocked", Color: "blocked", Emoji: "\U0001F6D1"},
		{ID: "COMPLETED", Name: "Completed", Color: "success", Emoji: "✅"},
	}
	set, err := NewActionStatusSet("BACKLOG", []string{"COMPLETED"}, defs)
	if err != nil {
		// The default set is fully static. A construction failure indicates a
		// programming bug, not user input — fail loudly.
		panic("default action status set is invalid: " + err.Error())
	}
	return set
}
