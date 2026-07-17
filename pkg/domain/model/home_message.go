package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// HomeMessageID identifies a stored home message. It is a UUID v7 so that
// document IDs and CreatedAt order agree (lexicographically time-ordered).
type HomeMessageID string

// NewHomeMessageID mints a time-ordered home message ID.
func NewHomeMessageID() HomeMessageID {
	return HomeMessageID(uuid.Must(uuid.NewV7()).String())
}

func (id HomeMessageID) String() string { return string(id) }

// ErrHomeMessageValidation is returned when a HomeMessage fails validation.
var ErrHomeMessageValidation = goerr.New("home message validation failed")

// HomeMessage is one LLM-generated home greeting for a user. Messages are
// append-only (never updated or TTL-deleted); freshness is judged in the
// usecase by comparing CreatedAt against a window. Keeping the history lets the
// generator avoid repeating recent phrasings.
type HomeMessage struct {
	ID        HomeMessageID
	UserID    string // Slack User ID (auth token Sub). Required.
	Message   string // Generated one-liner. Required.
	Lang      string // Language at generation ("en"/"ja"); used for reuse matching.
	CreatedAt time.Time
}

// Validate enforces the invariants the repository relies on before every write.
func (m *HomeMessage) Validate() error {
	if m == nil {
		return goerr.Wrap(ErrHomeMessageValidation, "home message is nil")
	}
	if m.ID == "" {
		return goerr.Wrap(ErrHomeMessageValidation, "id is required")
	}
	if m.UserID == "" {
		return goerr.Wrap(ErrHomeMessageValidation, "user ID is required")
	}
	if m.Message == "" {
		return goerr.Wrap(ErrHomeMessageValidation, "message is required")
	}
	return nil
}
