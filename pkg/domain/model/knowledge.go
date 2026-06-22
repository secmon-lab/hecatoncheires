package model

import (
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
	// MaxTags is the maximum number of tag references on a single Knowledge entry.
	MaxTags = 50
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
	// TagIDs reference first-class Tag entities for classification and filtering.
	// At least one tag is required. Tags must be created (create_tag / GraphQL
	// createTag) before they can be referenced here; the usecase verifies every
	// ID exists in the workspace before persistence. The slice is de-duplicated
	// (order preserved) by the usecase.
	TagIDs []TagID
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
// TagIDs must already be normalized by the caller; Validate does not mutate.
// Validate only enforces structural invariants — the existence of each TagID is
// verified by the usecase (which has repository access).
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
	if len(k.TagIDs) == 0 {
		return goerr.Wrap(ErrKnowledgeValidation, "at least one tag is required")
	}
	if len(k.TagIDs) > MaxTags {
		return goerr.Wrap(ErrKnowledgeValidation, "too many tags",
			goerr.V("count", len(k.TagIDs)), goerr.V("max", MaxTags))
	}
	for _, id := range k.TagIDs {
		if id == "" {
			return goerr.Wrap(ErrKnowledgeValidation, "tag id must not be empty")
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

// NormalizeTagIDs drops empty entries and de-duplicates while preserving
// first-seen order. The usecase applies this before existence verification /
// Validate / persistence.
func NormalizeTagIDs(ids []TagID) []TagID {
	seen := make(map[TagID]struct{}, len(ids))
	out := make([]TagID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
