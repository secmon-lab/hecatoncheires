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

type riskDocument struct {
	ID          int64     `firestore:"id"`
	Name        string    `firestore:"name"`
	Description string    `firestore:"description"`
	CreatedAt   time.Time `firestore:"created_at"`
	UpdatedAt   time.Time `firestore:"updated_at"`
}

type riskRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newRiskRepository(client *firestore.Client) *riskRepository {
	return &riskRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *riskRepository) risksCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_risks"
	}
	return "risks"
}

func (r *riskRepository) counterCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_counters"
	}
	return "counters"
}

func (r *riskRepository) riskCounterDoc() string {
	return "risk_counter"
}

func (r *riskRepository) getNextID(ctx context.Context) (int64, error) {
	counterRef := r.client.Collection(r.counterCollection()).Doc(r.riskCounterDoc())

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

		nextID = currentValue.(int64) + 1
		return tx.Update(counterRef, []firestore.Update{
			{Path: "value", Value: nextID},
		})
	})

	if err != nil {
		return 0, goerr.Wrap(err, "failed to get next ID")
	}

	return nextID, nil
}

func (r *riskRepository) Create(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	id, err := r.getNextID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	doc := &riskDocument{
		ID:          id,
		Name:        risk.Name,
		Description: risk.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	docRef := r.client.Collection(r.risksCollection()).Doc(fmt.Sprintf("%d", id))
	if _, err := docRef.Set(ctx, doc); err != nil {
		return nil, goerr.Wrap(err, "failed to create risk")
	}

	return &model.Risk{
		ID:          doc.ID,
		Name:        doc.Name,
		Description: doc.Description,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
	}, nil
}

func (r *riskRepository) Get(ctx context.Context, id int64) (*model.Risk, error) {
	docRef := r.client.Collection(r.risksCollection()).Doc(fmt.Sprintf("%d", id))
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get risk", goerr.V("id", id))
	}

	var riskDoc riskDocument
	if err := doc.DataTo(&riskDoc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal risk", goerr.V("id", id))
	}

	return &model.Risk{
		ID:          riskDoc.ID,
		Name:        riskDoc.Name,
		Description: riskDoc.Description,
		CreatedAt:   riskDoc.CreatedAt,
		UpdatedAt:   riskDoc.UpdatedAt,
	}, nil
}

func (r *riskRepository) List(ctx context.Context) ([]*model.Risk, error) {
	iter := r.client.Collection(r.risksCollection()).Documents(ctx)
	defer iter.Stop()

	var risks []*model.Risk
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate risks")
		}

		var riskDoc riskDocument
		if err := doc.DataTo(&riskDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal risk")
		}

		risks = append(risks, &model.Risk{
			ID:          riskDoc.ID,
			Name:        riskDoc.Name,
			Description: riskDoc.Description,
			CreatedAt:   riskDoc.CreatedAt,
			UpdatedAt:   riskDoc.UpdatedAt,
		})
	}

	return risks, nil
}

func (r *riskRepository) Update(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	docRef := r.client.Collection(r.risksCollection()).Doc(fmt.Sprintf("%d", risk.ID))

	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", risk.ID))
		}
		return nil, goerr.Wrap(err, "failed to get risk", goerr.V("id", risk.ID))
	}

	var existing riskDocument
	if err := doc.DataTo(&existing); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal risk", goerr.V("id", risk.ID))
	}

	now := time.Now().UTC()
	updated := &riskDocument{
		ID:          existing.ID,
		Name:        risk.Name,
		Description: risk.Description,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   now,
	}

	if _, err := docRef.Set(ctx, updated); err != nil {
		return nil, goerr.Wrap(err, "failed to update risk", goerr.V("id", risk.ID))
	}

	return &model.Risk{
		ID:          updated.ID,
		Name:        updated.Name,
		Description: updated.Description,
		CreatedAt:   updated.CreatedAt,
		UpdatedAt:   updated.UpdatedAt,
	}, nil
}

func (r *riskRepository) Delete(ctx context.Context, id int64) error {
	docRef := r.client.Collection(r.risksCollection()).Doc(fmt.Sprintf("%d", id))

	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to get risk", goerr.V("id", id))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete risk", goerr.V("id", id))
	}

	return nil
}
