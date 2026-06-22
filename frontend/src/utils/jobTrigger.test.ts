import { describe, it, expect } from 'vitest'

import { intervalLabel } from './jobTrigger'

describe('intervalLabel', () => {
  it('maps whole days', () => {
    expect(intervalLabel(86400)).toEqual({ key: 'caseAgentJobEveryDays', count: 1 })
    expect(intervalLabel(172800)).toEqual({ key: 'caseAgentJobEveryDays', count: 2 })
  })

  it('maps whole hours', () => {
    expect(intervalLabel(3600)).toEqual({ key: 'caseAgentJobEveryHours', count: 1 })
    expect(intervalLabel(7200)).toEqual({ key: 'caseAgentJobEveryHours', count: 2 })
  })

  it('maps whole minutes when not a whole hour', () => {
    expect(intervalLabel(5400)).toEqual({ key: 'caseAgentJobEveryMinutes', count: 90 })
    expect(intervalLabel(1800)).toEqual({ key: 'caseAgentJobEveryMinutes', count: 30 })
  })

  it('falls back to seconds for sub-minute or non-even intervals', () => {
    expect(intervalLabel(45)).toEqual({ key: 'caseAgentJobEverySeconds', count: 45 })
    expect(intervalLabel(95)).toEqual({ key: 'caseAgentJobEverySeconds', count: 95 })
  })

  it('clamps non-positive input to a zero-second label', () => {
    expect(intervalLabel(0)).toEqual({ key: 'caseAgentJobEverySeconds', count: 0 })
    expect(intervalLabel(-10)).toEqual({ key: 'caseAgentJobEverySeconds', count: 0 })
  })
})
