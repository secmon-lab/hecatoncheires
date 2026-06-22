package model

import (
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// TagID is the immutable identifier of a Tag. A random UUIDv4 is used rather
// than a sequential counter: tags are created by agents (the create_tag tool /
// the reflection agent) as well as by humans, so a per-workspace counter
// document would add transaction contention for no benefit. Tag ordering is
// driven by the explicit CreatedAt field, so the time-ordering of UUIDv7 buys
// nothing here.
type TagID string

// NewTagID generates a fresh random TagID.
func NewTagID() TagID {
	return TagID(uuid.NewString())
}

// String returns the raw string form of the TagID.
func (id TagID) String() string { return string(id) }

// ErrTagValidation is returned when a Tag fails its structural invariants.
var ErrTagValidation = goerr.New("tag validation failed")

// MaxTagNameLength is the maximum rune length of a Tag.Name.
const MaxTagNameLength = 64

// Tag is a workspace-scoped, first-class classification label referenced by
// Knowledge entries via its immutable ID. Unlike the previous free-form string
// tags, a Tag must be created explicitly (the create_tag tool / GraphQL
// createTag) before any Knowledge can reference it.
//
// All identifiers are flat top-level fields even though the Firestore path
// already encodes WorkspaceID, mirroring the Knowledge / Memo convention so a
// document inspected in isolation answers "which workspace is this?".
type Tag struct {
	ID          TagID
	WorkspaceID string
	// Name is an optional, human-facing label. It is mutable (update_tag) and
	// carries no uniqueness constraint — the ID is the stable key.
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate enforces the structural invariants required before any persistence
// write. The caller (usecase) assigns the ID via NewTagID and stamps the
// timestamps before the repository writes.
func (t *Tag) Validate() error {
	if t == nil {
		return goerr.Wrap(ErrTagValidation, "tag is nil")
	}
	if t.ID == "" {
		return goerr.Wrap(ErrTagValidation, "tag ID is required")
	}
	if t.WorkspaceID == "" {
		return goerr.Wrap(ErrTagValidation, "workspace ID is required")
	}
	if utf8.RuneCountInString(t.Name) > MaxTagNameLength {
		return goerr.Wrap(ErrTagValidation, "tag name is too long",
			goerr.V("length", utf8.RuneCountInString(t.Name)), goerr.V("max", MaxTagNameLength))
	}
	return nil
}
