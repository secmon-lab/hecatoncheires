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

func (r *knowledgeRepository) Create(ctx context.Context, knowledge *model.Knowledge) (*model.Knowledge, error) {
	now := time.Now().UTC()
	if knowledge.ID == "" {
		knowledge.ID = model.NewKnowledgeID()
	}
	knowledge.CreatedAt = now
	knowledge.UpdatedAt = now

	docRef := r.client.Collection(r.knowledgesCollection()).Doc(string(knowledge.ID))
	if _, err := docRef.Set(ctx, knowledge); err != nil {
		return nil, goerr.Wrap(err, "failed to create knowledge")
	}

	return knowledge, nil
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

	var k model.Knowledge
	if err := doc.DataTo(&k); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal knowledge", goerr.V("id", id))
	}

	return &k, nil
}

func (r *knowledgeRepository) ListByCaseID(ctx context.Context, caseID int64) ([]*model.Knowledge, error) {
	// Note: Removed OrderBy("SourcedAt") to comply with Firestore index policy
	// Results are unordered, but avoid requiring new composite index
	iter := r.client.Collection(r.knowledgesCollection()).
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

		var k model.Knowledge
		if err := doc.DataTo(&k); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, &k)
	}

	return knowledges, nil
}

func (r *knowledgeRepository) ListByCaseIDs(ctx context.Context, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	if len(caseIDs) == 0 {
		return make(map[int64][]*model.Knowledge), nil
	}

	// Use parallel execution to avoid requiring new Firestore indexes
	// Each query uses existing index: case_id == X, ORDER BY sourced_at DESC
	type result struct {
		caseID     int64
		knowledges []*model.Knowledge
		err        error
	}

	resultCh := make(chan result, len(caseIDs))

	for _, caseID := range caseIDs {
		go func(id int64) {
			knowledges, err := r.ListByCaseID(ctx, id)
			resultCh <- result{
				caseID:     id,
				knowledges: knowledges,
				err:        err,
			}
		}(caseID)
	}

	// Collect results
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

func (r *knowledgeRepository) ListBySourceID(ctx context.Context, sourceID model.SourceID) ([]*model.Knowledge, error) {
	// Note: Removed OrderBy("SourcedAt") to comply with Firestore index policy
	// Results are unordered, but avoid requiring new composite index
	iter := r.client.Collection(r.knowledgesCollection()).
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

		var k model.Knowledge
		if err := doc.DataTo(&k); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, &k)
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

		var k model.Knowledge
		if err := doc.DataTo(&k); err != nil {
			return nil, 0, goerr.Wrap(err, "failed to unmarshal knowledge")
		}

		knowledges = append(knowledges, &k)
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
