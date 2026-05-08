package model

import (
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// CaseDraftID is a UUID-based identifier for CaseDraft
type CaseDraftID string

// NewCaseDraftID generates a new UUID v4 CaseDraftID
func NewCaseDraftID() CaseDraftID {
	return CaseDraftID(uuid.New().String())
}

// String returns the string representation
func (id CaseDraftID) String() string {
	return string(id)
}

// CaseDraftTTL is the lifetime of a draft after creation.
const CaseDraftTTL = 24 * time.Hour

// CaseDraft holds the workspace-agnostic source material plus the current
// (single) AI-materialized Case payload for the currently-selected workspace.
// Switching workspace re-runs the Materializer and overwrites Materialization;
// no per-workspace cache is kept (intentional simplification).
type CaseDraft struct {
	ID        CaseDraftID
	CreatedBy string // Slack user ID who triggered the mention
	CreatedAt time.Time
	ExpiresAt time.Time

	// (a) Source — workspace-agnostic, immutable after creation
	RawMessages []DraftMessage
	MentionText string
	Source      DraftSource

	// (b) Current AI materialization (overwritten on workspace switch)
	SelectedWorkspaceID string
	Materialization     *WorkspaceMaterialization

	// InferenceInProgress is the server-side guard against concurrent
	// Submit/Edit/WS-switch interactions while a Materializer call is running.
	InferenceInProgress bool

	// EphemeralChannelID and EphemeralMessageTS identify the originally posted
	// ephemeral message. Used by interaction handlers that lack a response_url
	// (e.g., view_submission flows) to update the ephemeral via chat APIs.
	EphemeralChannelID string
	EphemeralMessageTS string
}

// DraftMessage is a single Slack message captured for context.
type DraftMessage struct {
	UserID    string
	Text      string
	TS        string
	Permalink string
}

// DraftSource describes where the mention happened.
type DraftSource struct {
	TeamID    string
	ChannelID string
	ThreadTS  string // empty when the mention was not in a thread
	MentionTS string
}

// WorkspaceMaterialization is the AI-generated Case payload for a specific
// workspace's FieldSchema. Generated lazily and overwritten on workspace switch.
type WorkspaceMaterialization struct {
	GeneratedAt       time.Time
	Title             string
	Description       string
	CustomFieldValues map[string]FieldValue // key = FieldID defined in workspace's FieldSchema
}

// NewCaseDraft constructs a fresh draft with a new ID, current timestamps,
// and a default 24h expiry. Caller must populate Source / RawMessages /
// MentionText / SelectedWorkspaceID before saving.
func NewCaseDraft(now time.Time, createdBy string) *CaseDraft {
	return &CaseDraft{
		ID:        NewCaseDraftID(),
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: now.Add(CaseDraftTTL),
	}
}

// IsExpired reports whether the draft is past its ExpiresAt.
func (d *CaseDraft) IsExpired(now time.Time) bool {
	return !now.Before(d.ExpiresAt)
}

// MaterializationValidationIssue describes a single problem found by
// WorkspaceMaterialization.Validate. Issues are partitioned into "fatal"
// (the materialization is unusable as-is — must be re-generated or rejected)
// and "fixable" (the user can correct it through the Edit modal).
type MaterializationValidationIssue struct {
	FieldID types.FieldID // empty for issues on Title/Description
	Code    string        // machine-readable category, e.g. "missing_required"
	Message string
	Fatal   bool
}

// Error implements the error interface for individual issues so callers may
// surface them via goerr.Wrap.
func (i MaterializationValidationIssue) Error() string {
	if i.FieldID == "" {
		return i.Code + ": " + i.Message
	}
	return string(i.FieldID) + ": " + i.Code + ": " + i.Message
}

// Validate checks the materialization against the workspace's FieldSchema.
// It does NOT mutate the receiver — it only inspects.
//
// Returned issues describe per-field / per-attribute problems; the boolean
// `fatal` is true iff at least one issue is marked fatal (currently: missing
// required Title/Description, or a fundamentally unusable structure).
//
// Callers typically:
//   - reject (regenerate) on fatal == true
//   - drop or surface non-fatal field issues so the human fills them via Edit
func (m *WorkspaceMaterialization) Validate(schema *config.FieldSchema) (issues []MaterializationValidationIssue, fatal bool) {
	if m == nil {
		return []MaterializationValidationIssue{{Code: "nil_materialization", Message: "materialization is nil", Fatal: true}}, true
	}
	if schema == nil {
		return []MaterializationValidationIssue{{Code: "nil_schema", Message: "FieldSchema is nil", Fatal: true}}, true
	}

	if strings.TrimSpace(m.Title) == "" {
		issues = append(issues, MaterializationValidationIssue{Code: "missing_title", Message: "title is empty", Fatal: true})
	}

	defByID := make(map[string]config.FieldDefinition, len(schema.Fields))
	for _, fd := range schema.Fields {
		defByID[fd.ID] = fd
	}

	// Per-field validation against schema.
	for _, fd := range schema.Fields {
		fv, present := m.CustomFieldValues[fd.ID]
		if !present {
			if fd.Required {
				issues = append(issues, MaterializationValidationIssue{
					FieldID: types.FieldID(fd.ID),
					Code:    "missing_required",
					Message: "required field is not filled",
				})
			}
			continue
		}
		issues = append(issues, validateFieldValue(fd, fv)...)
	}

	// Detect hallucinated keys (present in materialization but not in schema).
	for fieldID := range m.CustomFieldValues {
		if _, ok := defByID[fieldID]; !ok {
			issues = append(issues, MaterializationValidationIssue{
				FieldID: types.FieldID(fieldID),
				Code:    "unknown_field",
				Message: "field is not defined in workspace schema",
			})
		}
	}

	for _, iss := range issues {
		if iss.Fatal {
			fatal = true
			break
		}
	}
	return issues, fatal
}

func validateFieldValue(fd config.FieldDefinition, fv FieldValue) []MaterializationValidationIssue {
	if fv.Type != fd.Type {
		return []MaterializationValidationIssue{{
			FieldID: types.FieldID(fd.ID),
			Code:    "type_mismatch",
			Message: "value type does not match schema type",
		}}
	}

	switch fd.Type {
	case types.FieldTypeText, types.FieldTypeUser:
		if _, ok := fv.Value.(string); !ok {
			return wrongShape(fd, "expected string")
		}
	case types.FieldTypeURL:
		s, ok := fv.Value.(string)
		if !ok {
			return wrongShape(fd, "expected string")
		}
		if u, err := url.ParseRequestURI(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return []MaterializationValidationIssue{{
				FieldID: types.FieldID(fd.ID),
				Code:    "bad_url",
				Message: "value is not a valid http(s) URL",
			}}
		}
	case types.FieldTypeDate:
		s, ok := fv.Value.(string)
		if !ok {
			return wrongShape(fd, "expected string")
		}
		if !isISO8601Date(s) {
			return []MaterializationValidationIssue{{
				FieldID: types.FieldID(fd.ID),
				Code:    "bad_date",
				Message: "value is not a valid ISO-8601 date or datetime",
			}}
		}
	case types.FieldTypeNumber:
		if _, ok := fv.Value.(float64); !ok {
			return wrongShape(fd, "expected number")
		}
	case types.FieldTypeSelect:
		s, ok := fv.Value.(string)
		if !ok {
			return wrongShape(fd, "expected string")
		}
		if !optionAllowed(fd, s) {
			return []MaterializationValidationIssue{{
				FieldID: types.FieldID(fd.ID),
				Code:    "bad_enum",
				Message: "value is not in the allowed options",
			}}
		}
	case types.FieldTypeMultiSelect:
		arr, ok := fv.Value.([]string)
		if !ok {
			return wrongShape(fd, "expected array of strings")
		}
		for _, s := range arr {
			if !optionAllowed(fd, s) {
				return []MaterializationValidationIssue{{
					FieldID: types.FieldID(fd.ID),
					Code:    "bad_enum",
					Message: "one or more values are not in the allowed options",
				}}
			}
		}
	case types.FieldTypeMultiUser:
		if _, ok := fv.Value.([]string); !ok {
			return wrongShape(fd, "expected array of strings")
		}
	}
	return nil
}

func wrongShape(fd config.FieldDefinition, want string) []MaterializationValidationIssue {
	return []MaterializationValidationIssue{{
		FieldID: types.FieldID(fd.ID),
		Code:    "wrong_shape",
		Message: want,
	}}
}

func optionAllowed(fd config.FieldDefinition, s string) bool {
	for _, opt := range fd.Options {
		if opt.ID == s {
			return true
		}
	}
	return false
}

// isISO8601Date accepts either a calendar date (YYYY-MM-DD) or RFC3339 datetime.
func isISO8601Date(s string) bool {
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return true
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return true
	}
	return false
}

// FilterToValid returns a copy of the materialization with all fields that
// failed validation stripped out (so the human can fill them via Edit). The
// returned bool indicates whether the *Title* survived; callers should
// regenerate the materialization when title is missing.
func (m *WorkspaceMaterialization) FilterToValid(schema *config.FieldSchema) (*WorkspaceMaterialization, error) {
	issues, fatal := m.Validate(schema)
	if fatal {
		return nil, goerr.New("materialization has fatal validation issues",
			goerr.V("issues", issues))
	}
	bad := make(map[string]bool, len(issues))
	for _, iss := range issues {
		if iss.FieldID != "" {
			bad[string(iss.FieldID)] = true
		}
	}
	out := &WorkspaceMaterialization{
		GeneratedAt:       m.GeneratedAt,
		Title:             m.Title,
		Description:       m.Description,
		CustomFieldValues: make(map[string]FieldValue, len(m.CustomFieldValues)),
	}
	for k, v := range m.CustomFieldValues {
		if bad[k] {
			continue
		}
		out.CustomFieldValues[k] = v
	}
	return out, nil
}
