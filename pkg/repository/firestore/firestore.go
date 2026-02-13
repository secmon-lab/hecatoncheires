package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Firestore struct {
	client      *firestore.Client
	caseRepo    *caseRepository
	action      *actionRepository
	slack       *slackRepository
	slackUser   *slackUserRepository
	source      *sourceRepository
	knowledge   *knowledgeRepository
	caseMessage *caseMessageRepository
}

var _ interfaces.Repository = &Firestore{}

func New(ctx context.Context, projectID, databaseID string) (*Firestore, error) {
	var client *firestore.Client
	var err error
	if databaseID != "" {
		client, err = firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	} else {
		client, err = firestore.NewClient(ctx, projectID)
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client",
			goerr.V("projectID", projectID),
			goerr.V("databaseID", databaseID),
		)
	}

	f := &Firestore{
		client:      client,
		caseRepo:    newCaseRepository(client),
		action:      newActionRepository(client),
		slack:       newSlackRepository(client),
		slackUser:   newSlackUserRepository(client),
		source:      newSourceRepository(client),
		knowledge:   newKnowledgeRepository(client),
		caseMessage: newCaseMessageRepository(client),
	}

	return f, nil
}

func (f *Firestore) Case() interfaces.CaseRepository {
	return f.caseRepo
}

func (f *Firestore) Action() interfaces.ActionRepository {
	return f.action
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

func (f *Firestore) CaseMessage() interfaces.CaseMessageRepository {
	return f.caseMessage
}

func (f *Firestore) Close() error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}
