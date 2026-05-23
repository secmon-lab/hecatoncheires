package model

// SystemActorID is the sentinel ActorUserID used when a Job's tool invokes
// a mutation on behalf of the system (no human user). The '@' prefix ensures
// the value never collides with Slack user IDs (which always start with 'U'
// or 'W').
const SystemActorID = "@system"

// IsSystemActor reports whether the given actor user ID is the system
// sentinel. Permission checks that key off a Slack user identity must skip
// for system actor — there is no human to authenticate as.
func IsSystemActor(actorUserID string) bool {
	return actorUserID == SystemActorID
}
