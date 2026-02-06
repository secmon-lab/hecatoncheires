package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Firestore struct {
	client    *firestore.Client
	caseRepo  *caseRepository
	action    *actionRepository
	caseField *fieldValueRepository
	slack     *slackRepository
	slackUser *slackUserRepository
	source    *sourceRepository
	knowledge *knowledgeRepository
}

var _ interfaces.Repository = &Firestore{}

type Option func(*Firestore)

func WithCollectionPrefix(prefix string) Option {
	return func(f *Firestore) {
		f.caseRepo.collectionPrefix = prefix
		f.action.collectionPrefix = prefix
		f.caseField.collectionPrefix = prefix
		f.slack.collectionPrefix = prefix
		f.slackUser.collectionPrefix = prefix
		f.source.collectionPrefix = prefix
		f.knowledge.collectionPrefix = prefix
	}
}

func New(ctx context.Context, projectID string, opts ...Option) (*Firestore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client", goerr.V("projectID", projectID))
	}

	caseRepo := newCaseRepository(client)
	actionRepo := newActionRepository(client)
	fieldValueRepo := newFieldValueRepository(client)
	slackRepo := newSlackRepository(client)
	slackUserRepo := newSlackUserRepository(client)
	sourceRepo := newSourceRepository(client)
	knowledgeRepo := newKnowledgeRepository(client)

	f := &Firestore{
		client:    client,
		caseRepo:  caseRepo,
		action:    actionRepo,
		caseField: fieldValueRepo,
		slack:     slackRepo,
		slackUser: slackUserRepo,
		source:    sourceRepo,
		knowledge: knowledgeRepo,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f, nil
}

func (f *Firestore) Case() interfaces.CaseRepository {
	return f.caseRepo
}

func (f *Firestore) Action() interfaces.ActionRepository {
	return f.action
}

func (f *Firestore) CaseField() interfaces.FieldValueRepository {
	return f.caseField
}

func (f *Firestore) Slack() interfaces.SlackRepository {
	return f.slack
}

func (f *Firestore) SlackUser() interfaces.SlackUserRepository {
	return f.slackUser
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
