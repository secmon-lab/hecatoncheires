package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"google.golang.org/api/iterator"
)

const caseSlackMessagesCollection = "slack_messages"

type caseMessageRepository struct {
	client *firestore.Client
}

var _ interfaces.CaseMessageRepository = &caseMessageRepository{}

func newCaseMessageRepository(client *firestore.Client) *caseMessageRepository {
	return &caseMessageRepository{client: client}
}

func (r *caseMessageRepository) messagesCollection(workspaceID string, caseID int64) *firestore.CollectionRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID)).
		Collection(caseSlackMessagesCollection)
}

func (r *caseMessageRepository) Put(ctx context.Context, workspaceID string, caseID int64, msg *slack.Message) error {
	if msg == nil {
		return goerr.New("message is nil")
	}

	var files []slackFile
	for _, f := range msg.Files() {
		files = append(files, slackFile{
			ID:         f.ID(),
			Name:       f.Name(),
			Mimetype:   f.Mimetype(),
			Filetype:   f.Filetype(),
			Size:       f.Size(),
			URLPrivate: f.URLPrivate(),
			Permalink:  f.Permalink(),
			ThumbURL:   f.ThumbURL(),
		})
	}
	msgData := &slackMessage{
		ID:        msg.ID(),
		ChannelID: msg.ChannelID(),
		ThreadTS:  msg.ThreadTS(),
		TeamID:    msg.TeamID(),
		UserID:    msg.UserID(),
		UserName:  msg.UserName(),
		Text:      msg.Text(),
		EventTS:   msg.EventTS(),
		Files:     files,
		CreatedAt: msg.CreatedAt(),
	}

	ref := r.messagesCollection(workspaceID, caseID).Doc(msg.ID())
	if _, err := ref.Set(ctx, msgData); err != nil {
		return goerr.Wrap(err, "failed to save case message",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("message_id", msg.ID()))
	}
	return nil
}

func (r *caseMessageRepository) List(ctx context.Context, workspaceID string, caseID int64, limit int, cursor string) ([]*slack.Message, string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.messagesCollection(workspaceID, caseID).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit + 1)

	if cursor != "" {
		cursorDoc := r.messagesCollection(workspaceID, caseID).Doc(cursor)
		docSnap, err := cursorDoc.Get(ctx)
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to get cursor document",
				goerr.V("cursor", cursor))
		}
		query = query.StartAfter(docSnap)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var messages []*slack.Message
	var hasMore bool

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to iterate case messages")
		}

		if len(messages) >= limit {
			hasMore = true
			break
		}

		var msgData slackMessage
		if err := doc.DataTo(&msgData); err != nil {
			return nil, "", goerr.Wrap(err, "failed to unmarshal case message",
				goerr.V("doc_id", doc.Ref.ID))
		}

		var files []slack.File
		for _, f := range msgData.Files {
			files = append(files, slack.NewFileFromData(
				f.ID, f.Name, f.Mimetype, f.Filetype,
				f.Size, f.URLPrivate, f.Permalink, f.ThumbURL,
			))
		}
		msg := slack.NewMessageFromData(
			msgData.ID,
			msgData.ChannelID,
			msgData.ThreadTS,
			msgData.TeamID,
			msgData.UserID,
			msgData.UserName,
			msgData.Text,
			msgData.EventTS,
			msgData.CreatedAt,
			files,
		)
		messages = append(messages, msg)
	}

	var nextCursor string
	if hasMore && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID()
	}

	return messages, nextCursor, nil
}

func (r *caseMessageRepository) Prune(ctx context.Context, workspaceID string, caseID int64, before time.Time) (int, error) {
	const batchSize = 500
	totalDeleted := 0

	for {
		query := r.messagesCollection(workspaceID, caseID).
			Where("CreatedAt", "<", before).
			Limit(batchSize)

		iter := query.Documents(ctx)
		bulkWriter := r.client.BulkWriter(ctx)
		count := 0

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				bulkWriter.End()
				return totalDeleted, goerr.Wrap(err, "failed to iterate messages for deletion")
			}

			if _, err := bulkWriter.Delete(doc.Ref); err != nil {
				iter.Stop()
				bulkWriter.End()
				return totalDeleted, goerr.Wrap(err, "failed to delete case message")
			}
			count++
		}
		iter.Stop()
		bulkWriter.End()

		if count == 0 {
			break
		}
		totalDeleted += count

		if count < batchSize {
			break
		}
	}

	return totalDeleted, nil
}
