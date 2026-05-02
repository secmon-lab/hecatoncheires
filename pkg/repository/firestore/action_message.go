package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"google.golang.org/api/iterator"
)

const actionSlackMessagesCollection = "slack_messages"

type actionMessageRepository struct {
	client *firestore.Client
}

var _ interfaces.ActionMessageRepository = &actionMessageRepository{}

func newActionMessageRepository(client *firestore.Client) *actionMessageRepository {
	return &actionMessageRepository{client: client}
}

func (r *actionMessageRepository) messagesCollection(workspaceID string, actionID int64) *firestore.CollectionRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("actions").Doc(fmt.Sprintf("%d", actionID)).
		Collection(actionSlackMessagesCollection)
}

func (r *actionMessageRepository) Put(ctx context.Context, workspaceID string, actionID int64, msg *slack.Message) error {
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

	ref := r.messagesCollection(workspaceID, actionID).Doc(msg.ID())
	if _, err := ref.Set(ctx, msgData); err != nil {
		return goerr.Wrap(err, "failed to save action message",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("message_id", msg.ID()))
	}
	return nil
}

func (r *actionMessageRepository) List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*slack.Message, string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.messagesCollection(workspaceID, actionID).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit + 1)

	if cursor != "" {
		cursorDoc := r.messagesCollection(workspaceID, actionID).Doc(cursor)
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
	hasMore := false

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to iterate action messages")
		}

		if len(messages) >= limit {
			hasMore = true
			break
		}

		var msgData slackMessage
		if err := doc.DataTo(&msgData); err != nil {
			return nil, "", goerr.Wrap(err, "failed to unmarshal action message",
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
