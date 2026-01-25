package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Firestore struct {
	client       *firestore.Client
	risk         *riskRepository
	response     *responseRepository
	riskResponse *riskResponseRepository
	slack        *slackRepository
	source       *sourceRepository
	knowledge    *knowledgeRepository
}

var _ interfaces.Repository = &Firestore{}

type Option func(*Firestore)

func WithCollectionPrefix(prefix string) Option {
	return func(f *Firestore) {
		f.risk.collectionPrefix = prefix
		f.response.collectionPrefix = prefix
		f.riskResponse.collectionPrefix = prefix
		f.slack.collectionPrefix = prefix
		f.source.collectionPrefix = prefix
		f.knowledge.collectionPrefix = prefix
	}
}

func New(ctx context.Context, projectID, databaseID string, opts ...Option) (*Firestore, error) {
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client", goerr.V("projectID", projectID), goerr.V("databaseID", databaseID))
	}

	riskRepo := newRiskRepository(client)
	responseRepo := newResponseRepository(client)
	riskResponseRepo := newRiskResponseRepository(client, responseRepo, riskRepo)
	slackRepo := newSlackRepository(client)
	sourceRepo := newSourceRepository(client)
	knowledgeRepo := newKnowledgeRepository(client)

	f := &Firestore{
		client:       client,
		risk:         riskRepo,
		response:     responseRepo,
		riskResponse: riskResponseRepo,
		slack:        slackRepo,
		source:       sourceRepo,
		knowledge:    knowledgeRepo,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f, nil
}

func (f *Firestore) Risk() interfaces.RiskRepository {
	return f.risk
}

func (f *Firestore) Response() interfaces.ResponseRepository {
	return f.response
}

func (f *Firestore) RiskResponse() interfaces.RiskResponseRepository {
	return f.riskResponse
}

func (f *Firestore) Slack() interfaces.SlackRepository {
	return f.slack
}

func (f *Firestore) Source() interfaces.SourceRepository {
	return f.source
}

func (f *Firestore) Knowledge() interfaces.KnowledgeRepository {
	return f.knowledge
}

func (f *Firestore) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
