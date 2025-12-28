package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Firestore struct {
	client *firestore.Client
	risk   *riskRepository
}

var _ interfaces.Repository = &Firestore{}

type Option func(*Firestore)

func WithCollectionPrefix(prefix string) Option {
	return func(f *Firestore) {
		f.risk.collectionPrefix = prefix
	}
}

func New(ctx context.Context, projectID, databaseID string, opts ...Option) (*Firestore, error) {
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client", goerr.V("projectID", projectID), goerr.V("databaseID", databaseID))
	}

	f := &Firestore{
		client: client,
		risk:   newRiskRepository(client),
	}

	for _, opt := range opts {
		opt(f)
	}

	return f, nil
}

func (f *Firestore) Risk() interfaces.RiskRepository {
	return f.risk
}

func (f *Firestore) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
