package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Firestore struct {
	client *firestore.Client
	// Future: Add data storage fields here
}

var _ interfaces.Repository = &Firestore{}

func New(ctx context.Context, projectID string) (*Firestore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client")
	}

	return &Firestore{
		client: client,
	}, nil
}

func (f *Firestore) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
