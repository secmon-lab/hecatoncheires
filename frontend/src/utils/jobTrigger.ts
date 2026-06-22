import type { MsgKey } from '../i18n'

export interface IntervalLabel {
  key: MsgKey
  count: number
}

// intervalLabel converts a fixed schedule interval (in seconds) into the
// largest whole time unit that divides it evenly, returning the i18n key
// and the count to interpolate. Examples: 3600 → hours/1, 5400 →
// minutes/90 (not a whole hour), 45 → seconds/45. Non-positive input is
// clamped to a zero-second label so the UI never renders a negative
// interval.
export function intervalLabel(seconds: number): IntervalLabel {
  if (seconds > 0 && seconds % 86400 === 0) {
    return { key: 'caseAgentJobEveryDays', count: seconds / 86400 }
  }
  if (seconds > 0 && seconds % 3600 === 0) {
    return { key: 'caseAgentJobEveryHours', count: seconds / 3600 }
  }
  if (seconds > 0 && seconds % 60 === 0) {
    return { key: 'caseAgentJobEveryMinutes', count: seconds / 60 }
  }
  return { key: 'caseAgentJobEverySeconds', count: Math.max(seconds, 0) }
}
