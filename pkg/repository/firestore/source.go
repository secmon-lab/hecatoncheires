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

type sourceDocument struct {
	ID             string          `firestore:"id"`
	Name           string          `firestore:"name"`
	SourceType     string          `firestore:"source_type"`
	Description    string          `firestore:"description"`
	Enabled        bool            `firestore:"enabled"`
	NotionDBConfig *notionDBConfig `firestore:"notion_db_config,omitempty"`
	CreatedAt      time.Time       `firestore:"created_at"`
	UpdatedAt      time.Time       `firestore:"updated_at"`
}

type notionDBConfig struct {
	DatabaseID    string `firestore:"database_id"`
	DatabaseTitle string `firestore:"database_title"`
	DatabaseURL   string `firestore:"database_url"`
}

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

func sourceToDocument(source *model.Source) *sourceDocument {
	doc := &sourceDocument{
		ID:          string(source.ID),
		Name:        source.Name,
		SourceType:  string(source.SourceType),
		Description: source.Description,
		Enabled:     source.Enabled,
		CreatedAt:   source.CreatedAt,
		UpdatedAt:   source.UpdatedAt,
	}

	if source.NotionDBConfig != nil {
		doc.NotionDBConfig = &notionDBConfig{
			DatabaseID:    source.NotionDBConfig.DatabaseID,
			DatabaseTitle: source.NotionDBConfig.DatabaseTitle,
			DatabaseURL:   source.NotionDBConfig.DatabaseURL,
		}
	}

	return doc
}

func sourceToModel(doc *sourceDocument) *model.Source {
	source := &model.Source{
		ID:          model.SourceID(doc.ID),
		Name:        doc.Name,
		SourceType:  model.SourceType(doc.SourceType),
		Description: doc.Description,
		Enabled:     doc.Enabled,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
	}

	if doc.NotionDBConfig != nil {
		source.NotionDBConfig = &model.NotionDBConfig{
			DatabaseID:    doc.NotionDBConfig.DatabaseID,
			DatabaseTitle: doc.NotionDBConfig.DatabaseTitle,
			DatabaseURL:   doc.NotionDBConfig.DatabaseURL,
		}
	}

	return source
}

func (r *sourceRepository) Create(ctx context.Context, source *model.Source) (*model.Source, error) {
	now := time.Now().UTC()
	if source.ID == "" {
		source.ID = model.NewSourceID()
	}
	source.CreatedAt = now
	source.UpdatedAt = now

	doc := sourceToDocument(source)

	docRef := r.client.Collection(r.sourcesCollection()).Doc(string(source.ID))
	if _, err := docRef.Set(ctx, doc); err != nil {
		return nil, goerr.Wrap(err, "failed to create source")
	}

	return sourceToModel(doc), nil
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

	var srcDoc sourceDocument
	if err := doc.DataTo(&srcDoc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal source", goerr.V("id", id))
	}

	return sourceToModel(&srcDoc), nil
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

		var srcDoc sourceDocument
		if err := doc.DataTo(&srcDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal source")
		}

		sources = append(sources, sourceToModel(&srcDoc))
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

		var existing sourceDocument
		if err := doc.DataTo(&existing); err != nil {
			return goerr.Wrap(err, "failed to unmarshal source in transaction", goerr.V("id", source.ID))
		}

		source.CreatedAt = existing.CreatedAt
		source.UpdatedAt = now
		updatedDoc := sourceToDocument(source)

		if err := tx.Set(docRef, updatedDoc); err != nil {
			return goerr.Wrap(err, "failed to update source in transaction", goerr.V("id", source.ID))
		}
		updatedSource = sourceToModel(updatedDoc)
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
