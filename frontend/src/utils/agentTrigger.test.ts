import { describe, it, expect } from 'vitest'

import { runTriggerLabelKey } from './agentTrigger'

describe('runTriggerLabelKey', () => {
  it('maps the known backend event types to their i18n keys', () => {
    expect(runTriggerLabelKey('case')).toBe('caseAgentRunTriggerCase')
    expect(runTriggerLabelKey('scheduled')).toBe('caseAgentRunTriggerScheduled')
    expect(runTriggerLabelKey('mention')).toBe('caseAgentRunTriggerMention')
  })

  it('returns null for unknown or empty values so the caller can fall back', () => {
    expect(runTriggerLabelKey('')).toBeNull()
    expect(runTriggerLabelKey('something-new')).toBeNull()
  })
})
