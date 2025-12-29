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

type responseDocument struct {
	ID           int64     `firestore:"id"`
	Title        string    `firestore:"title"`
	Description  string    `firestore:"description"`
	ResponderIDs []string  `firestore:"responder_ids"`
	URL          string    `firestore:"url"`
	Status       string    `firestore:"status"`
	CreatedAt    time.Time `firestore:"created_at"`
	UpdatedAt    time.Time `firestore:"updated_at"`
}

type responseRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newResponseRepository(client *firestore.Client) *responseRepository {
	return &responseRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *responseRepository) responsesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_responses"
	}
	return "responses"
}

func (r *responseRepository) counterCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_counters"
	}
	return "counters"
}

func (r *responseRepository) responseCounterDoc() string {
	return "response_counter"
}

// toResponseDocument converts model.Response to responseDocument
func toResponseDocument(response *model.Response) *responseDocument {
	return &responseDocument{
		ID:           response.ID,
		Title:        response.Title,
		Description:  response.Description,
		ResponderIDs: response.ResponderIDs,
		URL:          response.URL,
		Status:       string(response.Status),
		CreatedAt:    response.CreatedAt,
		UpdatedAt:    response.UpdatedAt,
	}
}

// toResponseModel converts responseDocument to model.Response
func toResponseModel(doc *responseDocument) *model.Response {
	return &model.Response{
		ID:           doc.ID,
		Title:        doc.Title,
		Description:  doc.Description,
		ResponderIDs: doc.ResponderIDs,
		URL:          doc.URL,
		Status:       types.ResponseStatus(doc.Status),
		CreatedAt:    doc.CreatedAt,
		UpdatedAt:    doc.UpdatedAt,
	}
}

func (r *responseRepository) getNextID(ctx context.Context) (int64, error) {
	counterRef := r.client.Collection(r.counterCollection()).Doc(r.responseCounterDoc())

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

func (r *responseRepository) Create(ctx context.Context, response *model.Response) (*model.Response, error) {
	nextID, err := r.getNextID(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}

	now := time.Now().UTC()
	newResponse := &model.Response{
		ID:           nextID,
		Title:        response.Title,
		Description:  response.Description,
		ResponderIDs: response.ResponderIDs,
		URL:          response.URL,
		Status:       response.Status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	doc := toResponseDocument(newResponse)
	docRef := r.client.Collection(r.responsesCollection()).Doc(fmt.Sprintf("%d", nextID))

	_, err = docRef.Set(ctx, doc)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create response", goerr.V("id", nextID))
	}

	return newResponse, nil
}

func (r *responseRepository) Get(ctx context.Context, id int64) (*model.Response, error) {
	docRef := r.client.Collection(r.responsesCollection()).Doc(fmt.Sprintf("%d", id))
	docSnap, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get response", goerr.V("id", id))
	}

	var doc responseDocument
	if err := docSnap.DataTo(&doc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal response", goerr.V("id", id))
	}

	return toResponseModel(&doc), nil
}

func (r *responseRepository) List(ctx context.Context) ([]*model.Response, error) {
	iter := r.client.Collection(r.responsesCollection()).Documents(ctx)
	defer iter.Stop()

	var responses []*model.Response
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate responses")
		}

		var responseDoc responseDocument
		if err := doc.DataTo(&responseDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal response", goerr.V("id", doc.Ref.ID))
		}

		responses = append(responses, toResponseModel(&responseDoc))
	}

	return responses, nil
}

func (r *responseRepository) Update(ctx context.Context, response *model.Response) (*model.Response, error) {
	docRef := r.client.Collection(r.responsesCollection()).Doc(fmt.Sprintf("%d", response.ID))

	// Check if the document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", response.ID))
		}
		return nil, goerr.Wrap(err, "failed to get response for update", goerr.V("id", response.ID))
	}

	now := time.Now().UTC()
	updated := &model.Response{
		ID:           response.ID,
		Title:        response.Title,
		Description:  response.Description,
		ResponderIDs: response.ResponderIDs,
		URL:          response.URL,
		Status:       response.Status,
		CreatedAt:    response.CreatedAt,
		UpdatedAt:    now,
	}

	doc := toResponseDocument(updated)
	_, err = docRef.Set(ctx, doc)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update response", goerr.V("id", response.ID))
	}

	return updated, nil
}

func (r *responseRepository) Delete(ctx context.Context, id int64) error {
	docRef := r.client.Collection(r.responsesCollection()).Doc(fmt.Sprintf("%d", id))

	// Check if the document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to get response for deletion", goerr.V("id", id))
	}

	_, err = docRef.Delete(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to delete response", goerr.V("id", id))
	}

	return nil
}
