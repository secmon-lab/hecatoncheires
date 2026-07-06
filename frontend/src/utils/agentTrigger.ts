import type { MsgKey } from '../i18n'

// runTriggerLabelKey maps a JobRunLog.eventType provenance value to the i18n
// key for its human-readable label. The backend writes "case" / "scheduled"
// for Job runs and "mention" for mention-triggered agent runs. Returns null
// for an unrecognized value so the caller falls back to the raw string rather
// than showing a blank chip.
export function runTriggerLabelKey(eventType: string): MsgKey | null {
  switch (eventType) {
    case 'case':
      return 'caseAgentRunTriggerCase'
    case 'scheduled':
      return 'caseAgentRunTriggerScheduled'
    case 'mention':
      return 'caseAgentRunTriggerMention'
    default:
      return null
  }
}
