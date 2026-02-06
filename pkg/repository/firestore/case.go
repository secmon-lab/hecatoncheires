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

type caseRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newCaseRepository(client *firestore.Client) *caseRepository {
	return &caseRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *caseRepository) casesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_cases"
	}
	return "cases"
}

func (r *caseRepository) counterCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_counters"
	}
	return "counters"
}

func (r *caseRepository) caseCounterDoc() string {
	return "case_counter"
}

func (r *caseRepository) getNextID(ctx context.Context) (int64, error) {
	counterRef := r.client.Collection(r.counterCollection()).Doc(r.caseCounterDoc())

	var nextID int64
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(counterRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				nextID = 1
				return tx.Set(counterRef, map[string]interface{}{
					"value": nextID,
				})
			}
			return goerr.Wrap(err, "failed to get counter")
		}

		currentValue, err := doc.DataAt("value")
		if err != nil {
			return goerr.Wrap(err, "failed to get counter value")
		}

		val, ok := currentValue.(int64)
		if !ok {
			return goerr.New("counter value is not of type int64", goerr.V("value", currentValue))
		}
		nextID = val + 1
		return tx.Update(counterRef, []firestore.Update{
			{Path: "value", Value: nextID},
		})
	})

	if err != nil {
		return 0, goerr.Wrap(err, "failed to get next ID")
	}

	return nextID, nil
}

func (r *caseRepository) Create(ctx context.Context, c *model.Case) (*model.Case, error) {
	nextID, err := r.getNextID(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}

	now := time.Now().UTC()
	created := &model.Case{
		ID:             nextID,
		Title:          c.Title,
		Description:    c.Description,
		AssigneeIDs:    c.AssigneeIDs,
		SlackChannelID: c.SlackChannelID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	docID := fmt.Sprintf("%d", created.ID)

	_, err = r.client.Collection(r.casesCollection()).Doc(docID).Set(ctx, created)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case", goerr.V("id", created.ID))
	}

	return created, nil
}

func (r *caseRepository) Get(ctx context.Context, id int64) (*model.Case, error) {
	docID := fmt.Sprintf("%d", id)
	docSnap, err := r.client.Collection(r.casesCollection()).Doc(docID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get case", goerr.V("id", id))
	}

	var c model.Case
	if err := docSnap.DataTo(&c); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case", goerr.V("id", id))
	}

	return &c, nil
}

func (r *caseRepository) List(ctx context.Context) ([]*model.Case, error) {
	iter := r.client.Collection(r.casesCollection()).Documents(ctx)
	defer iter.Stop()

	var cases []*model.Case
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate cases")
		}

		var c model.Case
		if err := docSnap.DataTo(&c); err != nil {
			return nil, goerr.Wrap(err, "failed to decode case", goerr.V("doc_id", docSnap.Ref.ID))
		}

		cases = append(cases, &c)
	}

	return cases, nil
}

func (r *caseRepository) Update(ctx context.Context, c *model.Case) (*model.Case, error) {
	docID := fmt.Sprintf("%d", c.ID)
	docRef := r.client.Collection(r.casesCollection()).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
		}
		return nil, goerr.Wrap(err, "failed to check case existence", goerr.V("id", c.ID))
	}

	// Update with new timestamp
	updated := &model.Case{
		ID:             c.ID,
		Title:          c.Title,
		Description:    c.Description,
		AssigneeIDs:    c.AssigneeIDs,
		SlackChannelID: c.SlackChannelID,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      time.Now().UTC(),
	}

	_, err = docRef.Set(ctx, updated)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V("id", c.ID))
	}

	return updated, nil
}

func (r *caseRepository) Delete(ctx context.Context, id int64) error {
	docID := fmt.Sprintf("%d", id)
	docRef := r.client.Collection(r.casesCollection()).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to check case existence", goerr.V("id", id))
	}

	_, err = docRef.Delete(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to delete case", goerr.V("id", id))
	}

	return nil
}
