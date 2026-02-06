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

type sourceRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newSourceRepository(client *firestore.Client) *sourceRepository {
	return &sourceRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *sourceRepository) sourcesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_sources"
	}
	return "sources"
}

func (r *sourceRepository) Create(ctx context.Context, source *model.Source) (*model.Source, error) {
	now := time.Now().UTC()
	if source.ID == "" {
		source.ID = model.NewSourceID()
	}
	source.CreatedAt = now
	source.UpdatedAt = now

	docRef := r.client.Collection(r.sourcesCollection()).Doc(string(source.ID))
	if _, err := docRef.Set(ctx, source); err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return source, nil
}

func (r *sourceRepository) Get(ctx context.Context, id model.SourceID) (*model.Source, error) {
	docRef := r.client.Collection(r.sourcesCollection()).Doc(string(id))
	doc, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get source", goerr.V("id", id))
	}

	var source model.Source
	if err := doc.DataTo(&source); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal source", goerr.V("id", id))
	}

	return &source, nil
}

func (r *sourceRepository) List(ctx context.Context) ([]*model.Source, error) {
	iter := r.client.Collection(r.sourcesCollection()).Documents(ctx)
	defer iter.Stop()

	var sources []*model.Source
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate sources")
		}

		var source model.Source
		if err := doc.DataTo(&source); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal source")
		}

		sources = append(sources, &source)
	}

	return sources, nil
}

func (r *sourceRepository) Update(ctx context.Context, source *model.Source) (*model.Source, error) {
	docRef := r.client.Collection(r.sourcesCollection()).Doc(string(source.ID))
	now := time.Now().UTC()

	var updatedSource *model.Source
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", source.ID))
			}
			return goerr.Wrap(err, "failed to get source in transaction", goerr.V("id", source.ID))
		}

		var existing model.Source
		if err := doc.DataTo(&existing); err != nil {
			return goerr.Wrap(err, "failed to unmarshal source in transaction", goerr.V("id", source.ID))
		}

		source.CreatedAt = existing.CreatedAt
		source.UpdatedAt = now

		if err := tx.Set(docRef, source); err != nil {
			return goerr.Wrap(err, "failed to update source in transaction", goerr.V("id", source.ID))
		}
		updatedSource = source
		return nil
	})

	if err != nil {
		return nil, err
	}

	return updatedSource, nil
}

func (r *sourceRepository) Delete(ctx context.Context, id model.SourceID) error {
	docRef := r.client.Collection(r.sourcesCollection()).Doc(string(id))

	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to get source", goerr.V("id", id))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete source", goerr.V("id", id))
	}

	return nil
}
