package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type knowledgeDocument struct {
	ID        string             `firestore:"id"`
	RiskID    int64              `firestore:"risk_id"`
	SourceID  string             `firestore:"source_id"`
	SourceURL string             `firestore:"source_url"`
	Title     string             `firestore:"title"`
	Summary   string             `firestore:"summary"`
	Embedding firestore.Vector32 `firestore:"embedding"`
	SourcedAt time.Time          `firestore:"sourced_at"`
	CreatedAt time.Time          `firestore:"created_at"`
	UpdatedAt time.Time          `firestore:"updated_at"`
}

type knowledgeRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newKnowledgeRepository(client *firestore.Client) *knowledgeRepository {
	return &knowledgeRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *knowledgeRepository) knowledgesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_knowledges"
	}
	return "knowledges"
}

func knowledgeToDocument(k *model.Knowledge) *knowledgeDocument {
	doc := &knowledgeDocument{
		ID:        string(k.ID),
		RiskID:    k.RiskID,
		SourceID:  string(k.SourceID),
		SourceURL: k.SourceURL,
		Title:     k.Title,
		Summary:   k.Summary,
		SourcedAt: k.SourcedAt,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
	}

	if k.Embedding != nil {
		doc.Embedding = firestore.Vector32(k.Embedding)
	}

	return doc
}

func knowledgeToModel(doc *knowledgeDocument) *model.Knowledge {
	k := &model.Knowledge{
		ID:        model.KnowledgeID(doc.ID),
		RiskID:    doc.RiskID,
		SourceID:  model.SourceID(doc.SourceID),
		SourceURL: doc.SourceURL,
		Title:     doc.Title,
		Summary:   doc.Summary,
		SourcedAt: doc.SourcedAt,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}

	if doc.Embedding != nil {
		k.Embedding = []float32(doc.Embedding)
	}

	return k
}

func (r *knowledgeRepository) Create(ctx context.Context, knowledge *model.Knowledge) (*model.Knowledge, error) {
	now := time.Now().UTC()
	if knowledge.ID == "" {
		knowledge.ID = model.NewKnowledgeID()
	}
	knowledge.CreatedAt = now
	knowledge.UpdatedAt = now

	doc := knowledgeToDocument(knowledge)

	docRef := r.client.Collection(r.knowledgesCollection()).Doc(string(knowledge.ID))
	if _, err := docRef.Set(ctx, doc); err != nil {
		return nil, goerr.Wrap(err, "failed to create knowledge")
	}

	return knowledgeToModel(doc), nil
}

func (r *knowledgeRepository) Get(ctx context.Context, id model.KnowledgeID) (*model.Knowledge, error) {
	docRef := r.client.Collection(r.knowledgesCollection()).Doc(string(id))
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get knowledge", goerr.V("id", id))
	}

	var kDoc knowledgeDocument
	if err := doc.DataTo(&kDoc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal knowledge", goerr.V("id", id))
	}

	return knowledgeToModel(&kDoc), nil
}

func (r *knowledgeRepository) ListByRiskID(ctx context.Context, riskID int64) ([]*model.Knowledge, error) {
	iter := r.client.Collection(r.knowledgesCollection()).
		Where("risk_id", "==", riskID).
		OrderBy("sourced_at", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var knowledges []*model.Knowledge
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate knowledges", goerr.V("riskID", riskID))
		}

		var kDoc knowledgeDocument
		if err := doc.DataTo(&kDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, knowledgeToModel(&kDoc))
	}

	return knowledges, nil
}

func (r *knowledgeRepository) ListByRiskIDs(ctx context.Context, riskIDs []int64) (map[int64][]*model.Knowledge, error) {
	if len(riskIDs) == 0 {
		return make(map[int64][]*model.Knowledge), nil
	}

	// Use parallel execution to avoid requiring new Firestore indexes
	// Each query uses existing index: risk_id == X, ORDER BY sourced_at DESC
	type result struct {
		riskID     int64
		knowledges []*model.Knowledge
		err        error
	}

	resultCh := make(chan result, len(riskIDs))

	for _, riskID := range riskIDs {
		go func(id int64) {
			knowledges, err := r.ListByRiskID(ctx, id)
			resultCh <- result{
				riskID:     id,
				knowledges: knowledges,
				err:        err,
			}
		}(riskID)
	}

	// Collect results
	resultMap := make(map[int64][]*model.Knowledge, len(riskIDs))
	for i := 0; i < len(riskIDs); i++ {
		res := <-resultCh
		if res.err != nil {
			return nil, goerr.Wrap(res.err, "failed to get knowledges for risk", goerr.V("riskID", res.riskID))
		}
		resultMap[res.riskID] = res.knowledges
	}

	return resultMap, nil
}

func (r *knowledgeRepository) ListBySourceID(ctx context.Context, sourceID model.SourceID) ([]*model.Knowledge, error) {
	iter := r.client.Collection(r.knowledgesCollection()).
		Where("source_id", "==", string(sourceID)).
		OrderBy("sourced_at", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var knowledges []*model.Knowledge
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate knowledges", goerr.V("sourceID", sourceID))
		}

		var kDoc knowledgeDocument
		if err := doc.DataTo(&kDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, knowledgeToModel(&kDoc))
	}

	return knowledges, nil
}

func (r *knowledgeRepository) ListWithPagination(ctx context.Context, limit, offset int) ([]*model.Knowledge, int, error) {
	// Get total count first
	allDocs, err := r.client.Collection(r.knowledgesCollection()).Documents(ctx).GetAll()
	if err != nil {
		return nil, 0, goerr.Wrap(err, "failed to count knowledges")
	}
	totalCount := len(allDocs)

	// Get paginated results
	query := r.client.Collection(r.knowledgesCollection()).
		OrderBy("created_at", firestore.Desc).
		Offset(offset).
		Limit(limit)

	iter := query.Documents(ctx)
	defer iter.Stop()

	var knowledges []*model.Knowledge
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, 0, goerr.Wrap(err, "failed to iterate knowledges")
		}

		var kDoc knowledgeDocument
		if err := doc.DataTo(&kDoc); err != nil {
			return nil, 0, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, knowledgeToModel(&kDoc))
	}

	return knowledges, totalCount, nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, id model.KnowledgeID) error {
	docRef := r.client.Collection(r.knowledgesCollection()).Doc(string(id))

	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to get knowledge", goerr.V("id", id))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete knowledge", goerr.V("id", id))
	}

	return nil
}
