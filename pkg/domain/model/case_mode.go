package model

// CaseMode selects how Cases bind to Slack within a workspace.
//
//   - CaseModeChannel (default): one Case maps to a dedicated Slack channel
//     created at case activation. This is the original behaviour.
//   - CaseModeThread: the workspace monitors a single channel; each top-level
//     human message in that channel becomes a Case bound to its thread. No
//     dedicated channel is created, Actions and Drafts are not used, and the
//     configurable status set ([case.status]) attaches to the Case itself.
type CaseMode string

const (
	// CaseModeChannel is the default channel-per-case mode.
	CaseModeChannel CaseMode = "channel"
	// CaseModeThread is the thread-per-case mode.
	CaseModeThread CaseMode = "thread"
)

// IsValid reports whether the mode is a recognised value. The empty string is
// not valid here; callers normalise empty to CaseModeChannel before use.
func (m CaseMode) IsValid() bool {
	switch m {
	case CaseModeChannel, CaseModeThread:
		return true
	default:
		return false
	}
}

// Normalize maps the empty string to the default CaseModeChannel.
func (m CaseMode) Normalize() CaseMode {
	if m == "" {
		return CaseModeChannel
	}
	return m
}

// IsThread reports whether this is the thread-per-case mode.
func (m CaseMode) IsThread() bool {
	return m.Normalize() == CaseModeThread
}
