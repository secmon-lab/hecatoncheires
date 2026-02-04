package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	slackUsersCollection   = "slack_users"
	slackMetadataCollection = "slack_metadata"
	refreshStatusDocument = "refresh_status"

	// Firestore batch operation limits
	// Reference: https://cloud.google.com/firestore/docs/query-data/get-data#go
	firestoreGetAllLimit = 30 // Maximum document references per GetAll
)

type slackUserRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

var _ interfaces.SlackUserRepository = &slackUserRepository{}

func newSlackUserRepository(client *firestore.Client) *slackUserRepository {
	return &slackUserRepository{
		client: client,
	}
}

// slackUserDoc is the Firestore persistence model
type slackUserDoc struct {
	ID        string    `firestore:"id"`
	Name      string    `firestore:"name"`
	RealName  string    `firestore:"real_name"`
	Email     string    `firestore:"email"`
	ImageURL  string    `firestore:"image_url"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

// slackUserMetadataDoc is the Firestore persistence model for metadata
type slackUserMetadataDoc struct {
	LastRefreshSuccess time.Time `firestore:"last_refresh_success"`
	LastRefreshAttempt time.Time `firestore:"last_refresh_attempt"`
	UserCount          int       `firestore:"user_count"`
}

func (r *slackUserRepository) collection() *firestore.CollectionRef {
	if r.collectionPrefix != "" {
		return r.client.Collection(r.collectionPrefix + "_" + slackUsersCollection)
	}
	return r.client.Collection(slackUsersCollection)
}

func (r *slackUserRepository) metadataCollection() *firestore.CollectionRef {
	if r.collectionPrefix != "" {
		return r.client.Collection(r.collectionPrefix + "_" + slackMetadataCollection)
	}
	return r.client.Collection(slackMetadataCollection)
}

func (r *slackUserRepository) toDoc(user *model.SlackUser) *slackUserDoc {
	return &slackUserDoc{
		ID:        string(user.ID),
		Name:      user.Name,
		RealName:  user.RealName,
		Email:     user.Email,
		ImageURL:  user.ImageURL,
		UpdatedAt: user.UpdatedAt,
	}
}

func (r *slackUserRepository) fromDoc(doc *slackUserDoc) *model.SlackUser {
	return &model.SlackUser{
		ID:        model.SlackUserID(doc.ID),
		Name:      doc.Name,
		RealName:  doc.RealName,
		Email:     doc.Email,
		ImageURL:  doc.ImageURL,
		UpdatedAt: doc.UpdatedAt,
	}
}

func (r *slackUserRepository) toMetadataDoc(metadata *model.SlackUserMetadata) *slackUserMetadataDoc {
	return &slackUserMetadataDoc{
		LastRefreshSuccess: metadata.LastRefreshSuccess,
		LastRefreshAttempt: metadata.LastRefreshAttempt,
		UserCount:          metadata.UserCount,
	}
}

func (r *slackUserRepository) fromMetadataDoc(doc *slackUserMetadataDoc) *model.SlackUserMetadata {
	return &model.SlackUserMetadata{
		LastRefreshSuccess: doc.LastRefreshSuccess,
		LastRefreshAttempt: doc.LastRefreshAttempt,
		UserCount:          doc.UserCount,
	}
}

// GetAll retrieves all Slack users from Firestore
func (r *slackUserRepository) GetAll(ctx context.Context) ([]*model.SlackUser, error) {
	iter := r.collection().Documents(ctx)
	defer iter.Stop()

	var users []*model.SlackUser
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate Slack users")
		}

		var userDoc slackUserDoc
		if err := doc.DataTo(&userDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal Slack user", goerr.V("docID", doc.Ref.ID))
		}

		users = append(users, r.fromDoc(&userDoc))
	}

	return users, nil
}

// GetByID retrieves a single Slack user by ID
func (r *slackUserRepository) GetByID(ctx context.Context, id model.SlackUserID) (*model.SlackUser, error) {
	doc, err := r.collection().Doc(string(id)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "slack user not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get Slack user", goerr.V("id", id))
	}

	var userDoc slackUserDoc
	if err := doc.DataTo(&userDoc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal Slack user", goerr.V("id", id))
	}

	return r.fromDoc(&userDoc), nil
}

// GetByIDs retrieves multiple Slack users by IDs (for DataLoader batching)
// Handles Firestore GetAll limit of 10 documents per batch by splitting into multiple requests
func (r *slackUserRepository) GetByIDs(ctx context.Context, ids []model.SlackUserID) (map[model.SlackUserID]*model.SlackUser, error) {
	if len(ids) == 0 {
		return make(map[model.SlackUserID]*model.SlackUser), nil
	}

	result := make(map[model.SlackUserID]*model.SlackUser, len(ids))

	// Split into batches of firestoreGetAllLimit (10 documents)
	for i := 0; i < len(ids); i += firestoreGetAllLimit {
		end := i + firestoreGetAllLimit
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		// Create document references
		refs := make([]*firestore.DocumentRef, len(batch))
		for j, id := range batch {
			refs[j] = r.collection().Doc(string(id))
		}

		// Batch get
		docs, err := r.client.GetAll(ctx, refs)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to batch get Slack users", goerr.V("count", len(batch)))
		}

		for idx, doc := range docs {
			if !doc.Exists() {
				// Missing users are not included in the result map (not an error)
				continue
			}

			var userDoc slackUserDoc
			if err := doc.DataTo(&userDoc); err != nil {
				return nil, goerr.Wrap(err, "failed to unmarshal Slack user", goerr.V("id", batch[idx]))
			}

			result[batch[idx]] = r.fromDoc(&userDoc)
		}
	}

	return result, nil
}

// SaveMany saves multiple Slack users (upsert operation)
// Handles Firestore batch write limit of 500 documents by splitting into multiple batches
func (r *slackUserRepository) SaveMany(ctx context.Context, users []*model.SlackUser) error {
	if len(users) == 0 {
		return nil
	}

	// Use BulkWriter which automatically handles batching
	bulkWriter := r.client.BulkWriter(ctx)
	defer bulkWriter.End()

	for _, user := range users {
		docRef := r.collection().Doc(string(user.ID))
		if _, err := bulkWriter.Set(docRef, r.toDoc(user)); err != nil {
			return goerr.Wrap(err, "failed to add Set operation to bulk writer", goerr.V("user_id", user.ID))
		}
	}

	// Flush and wait for all operations to complete
	bulkWriter.Flush()

	return nil
}

// DeleteAll deletes all Slack users from Firestore
// Handles Firestore batch delete limit of 500 documents by splitting into multiple batches
func (r *slackUserRepository) DeleteAll(ctx context.Context) error {
	// Retrieve all document references
	iter := r.collection().Documents(ctx)
	defer iter.Stop()

	var refs []*firestore.DocumentRef
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return goerr.Wrap(err, "failed to iterate Slack users for deletion")
		}
		refs = append(refs, doc.Ref)
	}

	if len(refs) == 0 {
		return nil
	}

	// Use BulkWriter which automatically handles batching
	bulkWriter := r.client.BulkWriter(ctx)
	defer bulkWriter.End()

	for _, ref := range refs {
		if _, err := bulkWriter.Delete(ref); err != nil {
			return goerr.Wrap(err, "failed to add Delete operation to bulk writer")
		}
	}

	// Flush and wait for all operations to complete
	bulkWriter.Flush()

	return nil
}

// GetMetadata retrieves refresh metadata
func (r *slackUserRepository) GetMetadata(ctx context.Context) (*model.SlackUserMetadata, error) {
	doc, err := r.metadataCollection().Doc(refreshStatusDocument).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Return zero value if metadata doesn't exist yet
			return &model.SlackUserMetadata{
				LastRefreshSuccess: time.Time{},
				LastRefreshAttempt: time.Time{},
				UserCount:          0,
			}, nil
		}
		return nil, goerr.Wrap(err, "failed to get Slack user metadata")
	}

	var metadataDoc slackUserMetadataDoc
	if err := doc.DataTo(&metadataDoc); err != nil {
		return nil, goerr.Wrap(err, "failed to unmarshal Slack user metadata")
	}

	return r.fromMetadataDoc(&metadataDoc), nil
}

// SaveMetadata saves refresh metadata
func (r *slackUserRepository) SaveMetadata(ctx context.Context, metadata *model.SlackUserMetadata) error {
	_, err := r.metadataCollection().Doc(refreshStatusDocument).Set(ctx, r.toMetadataDoc(metadata))
	if err != nil {
		return goerr.Wrap(err, "failed to save Slack user metadata")
	}
	return nil
}
