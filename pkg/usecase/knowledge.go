package usecase

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// KnowledgeUseCase orchestrates workspace-wide shared knowledge operations.
// Embedding is optional: when no embed client is configured the use case
// degrades gracefully (no semantic vectors, substring search fallback).
type KnowledgeUseCase struct {
	repo        interfaces.Repository
	embedClient interfaces.EmbedClient
}

// NewKnowledgeUseCase constructs a KnowledgeUseCase. embedClient may be nil
// (fail-open: create/update still succeed and search falls back to substring).
func NewKnowledgeUseCase(repo interfaces.Repository, embedClient interfaces.EmbedClient) *KnowledgeUseCase {
	return &KnowledgeUseCase{repo: repo, embedClient: embedClient}
}

// ErrKnowledgeInput is returned when create/update input fails validation at the
// usecase entry point (before any persistence).
var ErrKnowledgeInput = goerr.New("invalid knowledge input")

// ErrUnknownTag is returned when a knowledge create/update references a tag id
// that does not exist in the workspace. The operation is rejected wholesale —
// no partial write occurs.
var ErrUnknownTag = goerr.New("unknown tag id")

// CreateKnowledgeInput is the domain-level input for creating a knowledge entry.
type CreateKnowledgeInput struct {
	Title  string
	Claim  string
	TagIDs []model.TagID
}

// Validate enforces input invariants at the entry point: a title and at least
// one tag id are required, and the claim must be within the length limit. Tag
// existence is verified separately against the repository in CreateKnowledge.
func (input CreateKnowledgeInput) Validate() error {
	if strings.TrimSpace(input.Title) == "" {
		return goerr.Wrap(ErrKnowledgeInput, "title is required")
	}
	if len(model.NormalizeTagIDs(input.TagIDs)) == 0 {
		return goerr.Wrap(ErrKnowledgeInput, "at least one tag is required")
	}
	if utf8.RuneCountInString(input.Claim) > model.MaxClaimLength {
		return goerr.Wrap(ErrKnowledgeInput, "claim is too long",
			goerr.V("length", utf8.RuneCountInString(input.Claim)), goerr.V("max", model.MaxClaimLength))
	}
	return nil
}

// UpdateKnowledgeInput is the domain-level input for updating a knowledge entry.
// Title / Claim / TagIDs are pointers: nil means "leave unchanged".
type UpdateKnowledgeInput struct {
	ID     model.KnowledgeID
	Title  *string
	Claim  *string
	TagIDs *[]model.TagID
}

// Validate enforces input invariants for the fields that are present.
func (input UpdateKnowledgeInput) Validate() error {
	if input.ID == "" {
		return goerr.Wrap(ErrKnowledgeInput, "knowledge ID is required")
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return goerr.Wrap(ErrKnowledgeInput, "title must not be empty")
	}
	if input.TagIDs != nil && len(model.NormalizeTagIDs(*input.TagIDs)) == 0 {
		return goerr.Wrap(ErrKnowledgeInput, "at least one tag is required")
	}
	if input.Claim != nil && utf8.RuneCountInString(*input.Claim) > model.MaxClaimLength {
		return goerr.Wrap(ErrKnowledgeInput, "claim is too long",
			goerr.V("length", utf8.RuneCountInString(*input.Claim)), goerr.V("max", model.MaxClaimLength))
	}
	return nil
}

// SearchKnowledgeInput controls a semantic search query.
type SearchKnowledgeInput struct {
	// Query is the natural-language search text. Empty returns the (optionally
	// tag-filtered) list ordered by CreatedAt.
	Query string
	// TagIDs applies an AND pre-filter before ranking.
	TagIDs []model.TagID
	// Limit caps the number of returned entries. Zero or negative means no cap;
	// the caller (resolver / tool) supplies the default.
	Limit int
}

// CreateKnowledge creates a new knowledge entry. The embedding is generated
// best-effort: a failure or a missing embed client never blocks creation.
func (uc *KnowledgeUseCase) CreateKnowledge(ctx context.Context, workspaceID string, input CreateKnowledgeInput) (*model.Knowledge, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	tagIDs := model.NormalizeTagIDs(input.TagIDs)
	if err := uc.verifyTagsExist(ctx, workspaceID, tagIDs); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	knowledge := &model.Knowledge{
		ID:          model.NewKnowledgeID(),
		WorkspaceID: workspaceID,
		Title:       strings.TrimSpace(input.Title),
		Claim:       input.Claim,
		TagIDs:      tagIDs,
		CreatorID:   creatorFromContext(ctx),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	knowledge.Embedding = uc.embedKnowledge(ctx, knowledge)

	if err := knowledge.Validate(); err != nil {
		return nil, err
	}

	created, err := uc.repo.Knowledge().Create(ctx, workspaceID, knowledge)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create knowledge", goerr.V("workspace_id", workspaceID))
	}
	return created, nil
}

// GetKnowledge retrieves a knowledge entry by ID.
func (uc *KnowledgeUseCase) GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	k, err := uc.repo.Knowledge().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get knowledge",
			goerr.V("workspace_id", workspaceID), goerr.V("knowledge_id", id))
	}
	return k, nil
}

// ListKnowledge lists knowledge entries with an optional tag AND filter.
func (uc *KnowledgeUseCase) ListKnowledge(ctx context.Context, workspaceID string, opts interfaces.KnowledgeListOptions) ([]*model.Knowledge, error) {
	items, err := uc.repo.Knowledge().List(ctx, workspaceID, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list knowledge", goerr.V("workspace_id", workspaceID))
	}
	return items, nil
}

// UpdateKnowledge applies a partial update. The embedding is regenerated when
// the title or claim changes (best-effort).
func (uc *KnowledgeUseCase) UpdateKnowledge(ctx context.Context, workspaceID string, input UpdateKnowledgeInput) (*model.Knowledge, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	knowledge, err := uc.repo.Knowledge().Get(ctx, workspaceID, input.ID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to load knowledge for update",
			goerr.V("workspace_id", workspaceID), goerr.V("knowledge_id", input.ID))
	}

	contentChanged := false
	if input.Title != nil {
		knowledge.Title = strings.TrimSpace(*input.Title)
		contentChanged = true
	}
	if input.Claim != nil {
		knowledge.Claim = *input.Claim
		contentChanged = true
	}
	if input.TagIDs != nil {
		tagIDs := model.NormalizeTagIDs(*input.TagIDs)
		if err := uc.verifyTagsExist(ctx, workspaceID, tagIDs); err != nil {
			return nil, err
		}
		knowledge.TagIDs = tagIDs
	}
	if contentChanged {
		knowledge.Embedding = uc.embedKnowledge(ctx, knowledge)
	}
	knowledge.UpdatedAt = time.Now().UTC()

	if err := knowledge.Validate(); err != nil {
		return nil, err
	}

	updated, err := uc.repo.Knowledge().Update(ctx, workspaceID, knowledge)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update knowledge",
			goerr.V("workspace_id", workspaceID), goerr.V("knowledge_id", input.ID))
	}
	return updated, nil
}

// DeleteKnowledge removes a knowledge entry.
func (uc *KnowledgeUseCase) DeleteKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	if err := uc.repo.Knowledge().Delete(ctx, workspaceID, id); err != nil {
		return goerr.Wrap(err, "failed to delete knowledge",
			goerr.V("workspace_id", workspaceID), goerr.V("knowledge_id", id))
	}
	return nil
}

// verifyTagsExist ensures every tag id exists in the workspace. It loads the
// workspace tag set once (the vocabulary is small) and checks membership; a
// single missing id fails the whole operation so no knowledge is ever persisted
// referencing a non-existent tag. This is the single authoritative guard for
// every knowledge write path (GraphQL, agent tools, reflection).
func (uc *KnowledgeUseCase) verifyTagsExist(ctx context.Context, workspaceID string, ids []model.TagID) error {
	if len(ids) == 0 {
		return goerr.Wrap(ErrKnowledgeInput, "at least one tag is required")
	}
	tags, err := uc.repo.Tag().List(ctx, workspaceID)
	if err != nil {
		return goerr.Wrap(err, "failed to list tags for verification", goerr.V("workspace_id", workspaceID))
	}
	known := make(map[model.TagID]struct{}, len(tags))
	for _, t := range tags {
		known[t.ID] = struct{}{}
	}
	missing := make([]model.TagID, 0)
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return goerr.Wrap(ErrUnknownTag, "knowledge references unknown tag(s); create the tag first",
			goerr.V("workspace_id", workspaceID), goerr.V("missing_tag_ids", missing))
	}
	return nil
}

// SearchKnowledge ranks entries by semantic similarity to the query. When no
// embedding is available (no embed client, embedding failure, or candidates
// without vectors) it falls back to a substring-match score over title + claim.
func (uc *KnowledgeUseCase) SearchKnowledge(ctx context.Context, workspaceID string, input SearchKnowledgeInput) ([]*model.Knowledge, error) {
	items, err := uc.repo.Knowledge().List(ctx, workspaceID, interfaces.KnowledgeListOptions{TagIDs: input.TagIDs})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list knowledge for search", goerr.V("workspace_id", workspaceID))
	}

	query := strings.TrimSpace(input.Query)
	if query == "" {
		return applyLimit(items, input.Limit), nil
	}

	queryVec, embedErr := uc.embedText(ctx, query)
	if embedErr != nil {
		// Non-fatal: report and fall back to substring search rather than failing
		// the whole query on a transient embedding outage.
		errutil.Handle(ctx, embedErr, "failed to embed knowledge search query")
		queryVec = nil
	}

	type scored struct {
		k     *model.Knowledge
		score float64
	}
	ranked := make([]scored, 0, len(items))
	useEmbedding := len(queryVec) > 0
	lowerQuery := strings.ToLower(query)
	for _, k := range items {
		var score float64
		if useEmbedding && len(k.Embedding) > 0 {
			score = cosineSimilarity(queryVec, k.Embedding)
		} else {
			score = substringScore(lowerQuery, k)
		}
		ranked = append(ranked, scored{k: k, score: score})
	}

	// Stable sort by score descending; tie-break on CreatedAt descending so the
	// most recent of equally-scored entries comes first.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].k.CreatedAt.After(ranked[j].k.CreatedAt)
		}
		return ranked[i].score > ranked[j].score
	})

	out := make([]*model.Knowledge, len(ranked))
	for i, r := range ranked {
		out[i] = r.k
	}
	return applyLimit(out, input.Limit), nil
}

// embedKnowledge generates the embedding for a knowledge entry best-effort,
// returning nil (and reporting via errutil) on failure so create/update never
// blocks on embedding.
func (uc *KnowledgeUseCase) embedKnowledge(ctx context.Context, k *model.Knowledge) []float64 {
	vec, err := uc.embedText(ctx, k.Title+"\n"+k.Claim)
	if err != nil {
		errutil.Handle(ctx, err, "failed to embed knowledge")
		return nil
	}
	return vec
}

// embedText returns the embedding vector for text. It returns (nil, nil) when no
// embed client is configured (fail-open). A configured client's failure is
// returned as an error for the caller to handle non-fatally.
func (uc *KnowledgeUseCase) embedText(ctx context.Context, text string) ([]float64, error) {
	if uc.embedClient == nil {
		return nil, nil
	}
	vecs, err := uc.embedClient.GenerateEmbedding(ctx, model.EmbeddingDimension, []string{text})
	if err != nil {
		return nil, goerr.Wrap(err, "embedding generation failed")
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	return vecs[0], nil
}

// applyLimit returns the first limit items, or all when limit <= 0.
func applyLimit(items []*model.Knowledge, limit int) []*model.Knowledge {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

// substringScore is the fallback relevance score when embeddings are
// unavailable. A title hit weighs more than a claim hit.
func substringScore(lowerQuery string, k *model.Knowledge) float64 {
	var score float64
	if strings.Contains(strings.ToLower(k.Title), lowerQuery) {
		score += 2
	}
	if strings.Contains(strings.ToLower(k.Claim), lowerQuery) {
		score++
	}
	return score
}

// cosineSimilarity returns the cosine similarity of two equal-length vectors.
// Mismatched lengths or a zero-norm vector yield 0.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
