package firestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const reactionClaimsSubcollection = "reaction_claims"

// reactionClaim is the persisted claim record. It carries the source identity
// for operational inspection; the mere existence of the document is the claim.
// It is the canonical stored shape (not a mirror of a domain model), so it is
// exempt from the "no doc types" rule.
type reactionClaim struct {
	WorkspaceID     string
	SourceChannelID string
	SourceMessageTS string
}

type reactionClaimRepository struct {
	client *firestore.Client
}

var _ interfaces.ReactionClaimRepository = &reactionClaimRepository{}

func newReactionClaimRepository(client *firestore.Client) *reactionClaimRepository {
	return &reactionClaimRepository{client: client}
}

// reactionClaimDocID derives a slash-free, delimiter-safe document ID from the
// source message identity. Hashing avoids any ambiguity from IDs that contain
// separators and keeps lookups index-free (a plain document Get/Create by ID).
func reactionClaimDocID(sourceChannelID, sourceMessageTS string) string {
	sum := sha256.Sum256([]byte(sourceChannelID + ":" + sourceMessageTS))
	return hex.EncodeToString(sum[:])
}

func (r *reactionClaimRepository) docRef(workspaceID, sourceChannelID, sourceMessageTS string) *firestore.DocumentRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection(reactionClaimsSubcollection).Doc(reactionClaimDocID(sourceChannelID, sourceMessageTS))
}

func (r *reactionClaimRepository) Claim(ctx context.Context, workspaceID, sourceChannelID, sourceMessageTS string) (bool, error) {
	if workspaceID == "" || sourceChannelID == "" || sourceMessageTS == "" {
		return false, goerr.New("workspaceID, sourceChannelID and sourceMessageTS are required")
	}
	// Create fails with AlreadyExists when the document is present, giving an
	// atomic first-writer-wins claim across concurrent instances.
	_, err := r.docRef(workspaceID, sourceChannelID, sourceMessageTS).Create(ctx, reactionClaim{
		WorkspaceID:     workspaceID,
		SourceChannelID: sourceChannelID,
		SourceMessageTS: sourceMessageTS,
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return false, nil
		}
		return false, goerr.Wrap(err, "failed to claim reaction source",
			goerr.V("workspace_id", workspaceID),
			goerr.V("source_channel_id", sourceChannelID),
			goerr.V("source_message_ts", sourceMessageTS),
		)
	}
	return true, nil
}

func (r *reactionClaimRepository) Release(ctx context.Context, workspaceID, sourceChannelID, sourceMessageTS string) error {
	if workspaceID == "" || sourceChannelID == "" || sourceMessageTS == "" {
		return goerr.New("workspaceID, sourceChannelID and sourceMessageTS are required")
	}
	if _, err := r.docRef(workspaceID, sourceChannelID, sourceMessageTS).Delete(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return goerr.Wrap(err, "failed to release reaction claim",
			goerr.V("workspace_id", workspaceID),
			goerr.V("source_channel_id", sourceChannelID),
			goerr.V("source_message_ts", sourceMessageTS),
		)
	}
	return nil
}
