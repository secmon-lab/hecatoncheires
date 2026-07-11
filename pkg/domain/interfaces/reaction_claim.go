package interfaces

import "context"

// ReactionClaimRepository records, once per source message, that a reaction on
// that message has begun producing a cross-channel case. It is the idempotency
// gate for reaction-triggered case creation when the reacted message lives
// outside the workspace's monitored channel — there is no stable case-thread key
// to dedup on yet, so multiple users reacting (or a re-delivered event) would
// otherwise each spawn a case.
//
// Same-channel reactions do NOT use this: the reacted message's thread root is a
// stable key, so the existing turn lock plus Case().GetBySlackThread already
// dedup that path.
type ReactionClaimRepository interface {
	// Claim atomically records the (workspaceID, sourceChannelID,
	// sourceMessageTS) triple. It returns claimed=true for the first caller and
	// claimed=false when a claim already exists. Backed by an index-free
	// create-if-absent (a document keyed by a deterministic hash of the source
	// message), so it is safe across concurrent instances.
	Claim(ctx context.Context, workspaceID, sourceChannelID, sourceMessageTS string) (claimed bool, err error)

	// Release removes a claim so a future reaction on the same message can retry.
	// Called only when case creation failed after a successful Claim (e.g. the
	// seed root post failed, or the create turn fell back before asking a
	// question). Best-effort; a missing claim is not an error.
	Release(ctx context.Context, workspaceID, sourceChannelID, sourceMessageTS string) error
}
