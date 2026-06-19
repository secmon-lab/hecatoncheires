package model

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// KnowledgeID is a UUID-based identifier for a Knowledge entry.
//
// As with MemoID, a string UUID (v7) is used rather than an int64 counter:
// knowledge entries are created by agents during case processing as well as by
// humans, so a counter document would add transaction contention. UUID v7 is
// generated in-process (multi-instance safe) and is time-ordered, keeping
// CreatedAt-order listing naturally sortable.
type KnowledgeID string

// NewKnowledgeID generates a new time-ordered UUID v7 KnowledgeID.
func NewKnowledgeID() KnowledgeID {
	return KnowledgeID(uuid.Must(uuid.NewV7()).String())
}

// String returns the raw string form of the KnowledgeID.
func (id KnowledgeID) String() string { return string(id) }

// ErrKnowledgeValidation is returned when a Knowledge fails its structural
// invariants.
var ErrKnowledgeValidation = goerr.New("knowledge validation failed")

// Knowledge invariants. These are domain constants (not deployment-tunable
// configuration): they bound a single shared-knowledge entry so a runaway agent
// cannot write unbounded documents. The defaults live here as exported
// constants rather than hidden inside Validate so callers and tests reference
// the same source of truth.
const (
	// MaxClaimLength is the maximum rune length of the Markdown claim body.
	MaxClaimLength = 8000
	// MaxTags is the maximum number of tags on a single Knowledge entry.
	MaxTags = 50
	// MaxTagLength is the maximum rune length of a single tag.
	MaxTagLength = 64
)

// Knowledge is a workspace-wide shared knowledge entry: organization-specific
// information that does not exist in the LLM's general knowledge (operating
// rules, internal proper nouns, past judgements, threat intel, ...) accumulated
// so it can be reused on future case processing. Unlike Memo it is NOT scoped to
// a case and carries no custom fields — only a Markdown claim body and tags.
//
// All identifiers are flat top-level fields even though the Firestore path
// already encodes WorkspaceID, mirroring the Memo convention so a document
// inspected in isolation answers "which workspace is this?".
type Knowledge struct {
	ID          KnowledgeID
	WorkspaceID string
	Title       string
	// Claim is a single Markdown text body (not an array). It is stored verbatim
	// and rendered as Markdown by the consumer (Web UI). The agent write tools
	// produce it directly.
	Claim string
	// Tags are free-form string tags used for classification and filtering. At
	// least one tag is required. They are normalized (trimmed / de-duplicated /
	// empty-dropped) by the usecase before persistence.
	Tags []string
	// Embedding is the vector embedding of Title + Claim used for in-memory
	// cosine semantic search. It is empty when no embedding client is configured
	// (fail-open). It is never exposed through the GraphQL surface.
	Embedding []float64
	// CreatorID is the Slack user id of the human author. It is empty when the
	// entry was authored by an agent (system actor).
	CreatorID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate enforces the structural invariants required before any persistence
// write. Repositories MUST call it before every write so a usecase / handler bug
// that forgot to inject an identity field fails loudly at the first write.
//
// Tags must already be normalized by the caller; Validate does not mutate.
func (k *Knowledge) Validate() error {
	if k == nil {
		return goerr.Wrap(ErrKnowledgeValidation, "knowledge is nil")
	}
	if k.ID == "" {
		return goerr.Wrap(ErrKnowledgeValidation, "knowledge ID is required")
	}
	if k.WorkspaceID == "" {
		return goerr.Wrap(ErrKnowledgeValidation, "workspace ID is required")
	}
	if k.Title == "" {
		return goerr.Wrap(ErrKnowledgeValidation, "title is required")
	}
	if len(k.Tags) == 0 {
		return goerr.Wrap(ErrKnowledgeValidation, "at least one tag is required")
	}
	if len(k.Tags) > MaxTags {
		return goerr.Wrap(ErrKnowledgeValidation, "too many tags",
			goerr.V("count", len(k.Tags)), goerr.V("max", MaxTags))
	}
	for _, t := range k.Tags {
		if t == "" {
			return goerr.Wrap(ErrKnowledgeValidation, "tag must not be empty")
		}
		if utf8.RuneCountInString(t) > MaxTagLength {
			return goerr.Wrap(ErrKnowledgeValidation, "tag is too long",
				goerr.V("tag", t), goerr.V("max", MaxTagLength))
		}
	}
	if utf8.RuneCountInString(k.Claim) > MaxClaimLength {
		return goerr.Wrap(ErrKnowledgeValidation, "claim is too long",
			goerr.V("length", utf8.RuneCountInString(k.Claim)), goerr.V("max", MaxClaimLength))
	}
	// Guard against accidental wrong-dimension embedding writes. An empty
	// embedding is legitimate (fail-open when no embedder is configured).
	if len(k.Embedding) > 0 && len(k.Embedding) != EmbeddingDimension {
		return goerr.Wrap(ErrKnowledgeValidation, "embedding dimension mismatch",
			goerr.V("got", len(k.Embedding)), goerr.V("want", EmbeddingDimension))
	}
	return nil
}

// NormalizeTags trims whitespace, drops empty entries, and de-duplicates while
// preserving first-seen order. Tag matching is case-sensitive, so casing is
// preserved as entered. The usecase applies this before Validate / persistence.
func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
