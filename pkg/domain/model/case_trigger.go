package model

// CaseTrigger selects what starts a Case in a thread-mode workspace.
//
//   - CaseTriggerInstant (default): every channel-root post in the monitored
//     channel starts a Case. This is the original thread-mode behaviour.
//   - CaseTriggerMention: a Case is started only when the bot is @mentioned —
//     either in a channel-root message or inside a thread that has no Case yet.
//     A plain post (no mention) never starts a Case.
//
// It is orthogonal to CaseMode and is only meaningful when CaseMode is thread;
// channel-mode workspaces ignore it.
type CaseTrigger string

const (
	// CaseTriggerInstant is the default: any channel-root post starts a Case.
	CaseTriggerInstant CaseTrigger = "instant"
	// CaseTriggerMention starts a Case only on an @mention of the bot.
	CaseTriggerMention CaseTrigger = "mention"
)

// IsValid reports whether the trigger is a recognised value. The empty string
// is not valid here; callers normalise empty to CaseTriggerInstant before use.
func (t CaseTrigger) IsValid() bool {
	switch t {
	case CaseTriggerInstant, CaseTriggerMention:
		return true
	default:
		return false
	}
}

// Normalize maps the empty string to the default CaseTriggerInstant.
func (t CaseTrigger) Normalize() CaseTrigger {
	if t == "" {
		return CaseTriggerInstant
	}
	return t
}

// IsMention reports whether Cases are started only on an @mention.
func (t CaseTrigger) IsMention() bool {
	return t.Normalize() == CaseTriggerMention
}
