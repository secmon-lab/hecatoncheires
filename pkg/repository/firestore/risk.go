package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type riskDocument struct {
	ID                  int64     `firestore:"id"`
	Name                string    `firestore:"name"`
	Description         string    `firestore:"description"`
	CategoryIDs         []string  `firestore:"category_ids"`
	SpecificImpact      string    `firestore:"specific_impact"`
	LikelihoodID        string    `firestore:"likelihood_id"`
	ImpactID            string    `firestore:"impact_id"`
	ResponseTeamIDs     []string  `firestore:"response_team_ids"`
	AssigneeIDs         []string  `firestore:"assignee_ids"`
	DetectionIndicators string    `firestore:"detection_indicators"`
	CreatedAt           time.Time `firestore:"created_at"`
	UpdatedAt           time.Time `firestore:"updated_at"`
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

// toDocument converts model.Risk to riskDocument
func toDocument(risk *model.Risk) *riskDocument {
	categoryIDs := make([]string, len(risk.CategoryIDs))
	for i, id := range risk.CategoryIDs {
		categoryIDs[i] = string(id)
	}

	teamIDs := make([]string, len(risk.ResponseTeamIDs))
	for i, id := range risk.ResponseTeamIDs {
		teamIDs[i] = string(id)
	}

	return &riskDocument{
		ID:                  risk.ID,
		Name:                risk.Name,
		Description:         risk.Description,
		CategoryIDs:         categoryIDs,
		SpecificImpact:      risk.SpecificImpact,
		LikelihoodID:        string(risk.LikelihoodID),
		ImpactID:            string(risk.ImpactID),
		ResponseTeamIDs:     teamIDs,
		AssigneeIDs:         risk.AssigneeIDs,
		DetectionIndicators: risk.DetectionIndicators,
		CreatedAt:           risk.CreatedAt,
		UpdatedAt:           risk.UpdatedAt,
	}
}

// toModel converts riskDocument to model.Risk
func toModel(doc *riskDocument) *model.Risk {
	categoryIDs := make([]types.CategoryID, len(doc.CategoryIDs))
	for i, id := range doc.CategoryIDs {
		categoryIDs[i] = types.CategoryID(id)
	}

	teamIDs := make([]types.TeamID, len(doc.ResponseTeamIDs))
	for i, id := range doc.ResponseTeamIDs {
		teamIDs[i] = types.TeamID(id)
	}

	return &model.Risk{
		ID:                  doc.ID,
		Name:                doc.Name,
		Description:         doc.Description,
		CategoryIDs:         categoryIDs,
		SpecificImpact:      doc.SpecificImpact,
		LikelihoodID:        types.LikelihoodID(doc.LikelihoodID),
		ImpactID:            types.ImpactID(doc.ImpactID),
		ResponseTeamIDs:     teamIDs,
		AssigneeIDs:         doc.AssigneeIDs,
		DetectionIndicators: doc.DetectionIndicators,
		CreatedAt:           doc.CreatedAt,
		UpdatedAt:           doc.UpdatedAt,
	}
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

func (r *riskRepository) Create(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	id, err := r.getNextID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	risk.ID = id
	risk.CreatedAt = now
	risk.UpdatedAt = now

	doc := toDocument(risk)

	docRef := r.client.Collection(r.risksCollection()).Doc(fmt.Sprintf("%d", id))
	if _, err := docRef.Set(ctx, doc); err != nil {
		return nil, goerr.Wrap(err, "failed to create risk")
	}

	return toModel(doc), nil
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

	return toModel(&riskDoc), nil
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

		risks = append(risks, toModel(&riskDoc))
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
	risk.CreatedAt = existing.CreatedAt
	risk.UpdatedAt = now

	updated := toDocument(risk)

	if _, err := docRef.Set(ctx, updated); err != nil {
		return nil, goerr.Wrap(err, "failed to update risk", goerr.V("id", risk.ID))
	}

	return toModel(updated), nil
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
