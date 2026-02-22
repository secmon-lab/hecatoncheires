package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// memoryDoc is the Firestore document representation of model.Memory.
// Embedding is stored as firestore.Vector32 for FindNearest vector search.
type memoryDoc struct {
	ID        model.MemoryID     `firestore:"ID"`
	CaseID    int64              `firestore:"CaseID"`
	Claim     string             `firestore:"Claim"`
	Embedding firestore.Vector32 `firestore:"Embedding,omitempty"`
	CreatedAt time.Time          `firestore:"CreatedAt"`
}

func toMemoryDoc(m *model.Memory) *memoryDoc {
	doc := &memoryDoc{
		ID:        m.ID,
		CaseID:    m.CaseID,
		Claim:     m.Claim,
		CreatedAt: m.CreatedAt,
	}
	if len(m.Embedding) > 0 {
		doc.Embedding = firestore.Vector32(m.Embedding)
	}
	return doc
}

func fromMemoryDoc(d *memoryDoc) *model.Memory {
	m := &model.Memory{
		ID:        d.ID,
		CaseID:    d.CaseID,
		Claim:     d.Claim,
		CreatedAt: d.CreatedAt,
	}
	if len(d.Embedding) > 0 {
		m.Embedding = []float32(d.Embedding)
	}
	return m
}

type firestoreMemoryRepository struct {
	client *firestore.Client
}

func newFirestoreMemoryRepository(client *firestore.Client) *firestoreMemoryRepository {
	return &firestoreMemoryRepository{client: client}
}

// memoriesCollection returns the subcollection path:
// workspaces/{workspaceID}/cases/{caseID}/memories
func (r *firestoreMemoryRepository) memoriesCollection(workspaceID string, caseID int64) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID)).
		Collection("memories")
}

func (r *firestoreMemoryRepository) Create(ctx context.Context, workspaceID string, caseID int64, mem *model.Memory) (*model.Memory, error) {
	if mem.ID == "" {
		mem.ID = model.NewMemoryID()
	}
	mem.CaseID = caseID
	mem.CreatedAt = time.Now().UTC()

	docRef := r.memoriesCollection(workspaceID, caseID).Doc(string(mem.ID))
	if _, err := docRef.Set(ctx, toMemoryDoc(mem)); err != nil {
		return nil, goerr.Wrap(err, "failed to create memory")
	}

	return mem, nil
}

func (r *firestoreMemoryRepository) Get(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) (*model.Memory, error) {
	docRef := r.memoriesCollection(workspaceID, caseID).Doc(string(memoryID))
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
		}
		return nil, goerr.Wrap(err, "failed to get memory", goerr.V("memoryID", memoryID))
	}

	var d memoryDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal memory", goerr.V("memoryID", memoryID))
	}

	return fromMemoryDoc(&d), nil
}

func (r *firestoreMemoryRepository) Delete(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) error {
	docRef := r.memoriesCollection(workspaceID, caseID).Doc(string(memoryID))

	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
		}
		return goerr.Wrap(err, "failed to get memory", goerr.V("memoryID", memoryID))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete memory", goerr.V("memoryID", memoryID))
	}

	return nil
}

func (r *firestoreMemoryRepository) List(ctx context.Context, workspaceID string, caseID int64) ([]*model.Memory, error) {
	iter := r.memoriesCollection(workspaceID, caseID).
		OrderBy("CreatedAt", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	memories := make([]*model.Memory, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate memories")
		}

		var d memoryDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal memory")
		}

		memories = append(memories, fromMemoryDoc(&d))
	}

	return memories, nil
}

func (r *firestoreMemoryRepository) FindByEmbedding(ctx context.Context, workspaceID string, caseID int64, embedding []float32, limit int) ([]*model.Memory, error) {
	vq := r.memoriesCollection(workspaceID, caseID).
		FindNearest("Embedding", firestore.Vector32(embedding), limit, firestore.DistanceMeasureCosine, nil)

	iter := vq.Documents(ctx)
	defer iter.Stop()

	memories := make([]*model.Memory, 0, limit)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate memory vector search results")
		}

		var d memoryDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal memory from vector search")
		}

		memories = append(memories, fromMemoryDoc(&d))
	}

	return memories, nil
}
