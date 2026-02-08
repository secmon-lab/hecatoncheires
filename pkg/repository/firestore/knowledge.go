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

// knowledgeDoc is the Firestore document representation of model.Knowledge.
// Embedding is stored as firestore.Vector32 so that FindNearest vector search works.
type knowledgeDoc struct {
	ID        model.KnowledgeID  `firestore:"ID"`
	CaseID    int64              `firestore:"CaseID"`
	SourceID  model.SourceID     `firestore:"SourceID"`
	SourceURL string             `firestore:"SourceURL"`
	Title     string             `firestore:"Title"`
	Summary   string             `firestore:"Summary"`
	Embedding firestore.Vector32 `firestore:"Embedding,omitempty"`
	SourcedAt time.Time          `firestore:"SourcedAt"`
	CreatedAt time.Time          `firestore:"CreatedAt"`
	UpdatedAt time.Time          `firestore:"UpdatedAt"`
}

func toKnowledgeDoc(k *model.Knowledge) *knowledgeDoc {
	doc := &knowledgeDoc{
		ID:        k.ID,
		CaseID:    k.CaseID,
		SourceID:  k.SourceID,
		SourceURL: k.SourceURL,
		Title:     k.Title,
		Summary:   k.Summary,
		SourcedAt: k.SourcedAt,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
	}
	if len(k.Embedding) > 0 {
		doc.Embedding = firestore.Vector32(k.Embedding)
	}
	return doc
}

func fromKnowledgeDoc(d *knowledgeDoc) *model.Knowledge {
	k := &model.Knowledge{
		ID:        d.ID,
		CaseID:    d.CaseID,
		SourceID:  d.SourceID,
		SourceURL: d.SourceURL,
		Title:     d.Title,
		Summary:   d.Summary,
		SourcedAt: d.SourcedAt,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
	if len(d.Embedding) > 0 {
		k.Embedding = []float32(d.Embedding)
	}
	return k
}

func docToKnowledge(doc *firestore.DocumentSnapshot) (*model.Knowledge, error) {
	var d knowledgeDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}
	return fromKnowledgeDoc(&d), nil
}

type knowledgeRepository struct {
	client *firestore.Client
}

func newKnowledgeRepository(client *firestore.Client) *knowledgeRepository {
	return &knowledgeRepository{
		client: client,
	}
}

func (r *knowledgeRepository) knowledgesCollection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("knowledges")
}

func (r *knowledgeRepository) Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
	now := time.Now().UTC()
	if knowledge.ID == "" {
		knowledge.ID = model.NewKnowledgeID()
	}
	knowledge.CreatedAt = now
	knowledge.UpdatedAt = now

	docRef := r.knowledgesCollection(workspaceID).Doc(string(knowledge.ID))
	if _, err := docRef.Set(ctx, toKnowledgeDoc(knowledge)); err != nil {
		return nil, goerr.Wrap(err, "failed to create knowledge")
	}

	return knowledge, nil
}

func (r *knowledgeRepository) Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	docRef := r.knowledgesCollection(workspaceID).Doc(string(id))
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get knowledge", goerr.V("id", id))
	}

	k, err := docToKnowledge(doc)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal knowledge", goerr.V("id", id))
	}

	return k, nil
}

func (r *knowledgeRepository) ListByCaseID(ctx context.Context, workspaceID string, caseID int64) ([]*model.Knowledge, error) {
	iter := r.knowledgesCollection(workspaceID).
		Where("CaseID", "==", caseID).
		Documents(ctx)
	defer iter.Stop()

	knowledges := make([]*model.Knowledge, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate knowledges", goerr.V("caseID", caseID))
		}

		k, err := docToKnowledge(doc)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, k)
	}

	return knowledges, nil
}

func (r *knowledgeRepository) ListByCaseIDs(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	if len(caseIDs) == 0 {
		return make(map[int64][]*model.Knowledge), nil
	}

	type result struct {
		caseID     int64
		knowledges []*model.Knowledge
		err        error
	}

	resultCh := make(chan result, len(caseIDs))

	for _, caseID := range caseIDs {
		go func(id int64) {
			knowledges, err := r.ListByCaseID(ctx, workspaceID, id)
			resultCh <- result{
				caseID:     id,
				knowledges: knowledges,
				err:        err,
			}
		}(caseID)
	}

	resultMap := make(map[int64][]*model.Knowledge, len(caseIDs))
	for i := 0; i < len(caseIDs); i++ {
		res := <-resultCh
		if res.err != nil {
			return nil, goerr.Wrap(res.err, "failed to get knowledges for case", goerr.V("caseID", res.caseID))
		}
		resultMap[res.caseID] = res.knowledges
	}

	return resultMap, nil
}

func (r *knowledgeRepository) ListBySourceID(ctx context.Context, workspaceID string, sourceID model.SourceID) ([]*model.Knowledge, error) {
	iter := r.knowledgesCollection(workspaceID).
		Where("SourceID", "==", string(sourceID)).
		Documents(ctx)
	defer iter.Stop()

	knowledges := make([]*model.Knowledge, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate knowledges", goerr.V("sourceID", sourceID))
		}

		k, err := docToKnowledge(doc)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, k)
	}

	return knowledges, nil
}

func (r *knowledgeRepository) ListWithPagination(ctx context.Context, workspaceID string, limit, offset int) ([]*model.Knowledge, int, error) {
	// Get total count first
	allDocs, err := r.knowledgesCollection(workspaceID).Documents(ctx).GetAll()
	if err != nil {
		return nil, 0, goerr.Wrap(err, "failed to count knowledges")
	}
	totalCount := len(allDocs)

	// Get paginated results
	query := r.knowledgesCollection(workspaceID).
		OrderBy("CreatedAt", firestore.Desc).
		Offset(offset).
		Limit(limit)

	iter := query.Documents(ctx)
	defer iter.Stop()

	knowledges := make([]*model.Knowledge, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, 0, goerr.Wrap(err, "failed to iterate knowledges")
		}

		k, err := docToKnowledge(doc)
		if err != nil {
			return nil, 0, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, k)
	}

	return knowledges, totalCount, nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	docRef := r.knowledgesCollection(workspaceID).Doc(string(id))

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

func (r *knowledgeRepository) FindByEmbedding(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error) {
	vq := r.knowledgesCollection(workspaceID).
		FindNearest("Embedding", firestore.Vector32(embedding), limit, firestore.DistanceMeasureCosine, nil)

	iter := vq.Documents(ctx)
	defer iter.Stop()

	knowledges := make([]*model.Knowledge, 0, limit)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate vector search results")
		}

		k, err := docToKnowledge(doc)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge from vector search")
		}

		knowledges = append(knowledges, k)
	}

	return knowledges, nil
}
