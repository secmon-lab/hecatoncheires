import { describe, it, expect } from 'vitest'

import { RUN_POLL_INTERVAL_MS, RUN_POLL_MAX_MS, shouldPollRunLogs } from './runPolling'

const NOW = 1_700_000_000_000

describe('shouldPollRunLogs', () => {
  it('polls while a RUNNING row is visible', () => {
    expect(
      shouldPollRunLogs({
        rows: [{ stage: 'SUCCESS' }, { stage: 'RUNNING' }],
        page: 0,
        deadline: 0,
        now: NOW,
      }),
    ).toBe(true)
  })

  it('polls while a just-issued trigger has not produced a RUNNING row yet', () => {
    expect(
      shouldPollRunLogs({
        rows: [{ stage: 'SUCCESS' }],
        page: 0,
        deadline: NOW + 10_000,
        now: NOW,
      }),
    ).toBe(true)
  })

  it('stops once the trigger deadline passes with no RUNNING row', () => {
    expect(
      shouldPollRunLogs({
        rows: [{ stage: 'SUCCESS' }, { stage: 'FAILED' }],
        page: 0,
        deadline: NOW - 1,
        now: NOW,
      }),
    ).toBe(false)
  })

  it('does not poll when nothing is pending', () => {
    expect(
      shouldPollRunLogs({ rows: [{ stage: 'SUCCESS' }], page: 0, deadline: 0, now: NOW }),
    ).toBe(false)
    expect(shouldPollRunLogs({ rows: [], page: 0, deadline: 0, now: NOW })).toBe(false)
  })

  it('treats AWAITING_INPUT as terminal for polling purposes', () => {
    // A suspended run waits on a human and can stay in that stage for hours;
    // polling for it would never end.
    expect(
      shouldPollRunLogs({
        rows: [{ stage: 'AWAITING_INPUT' }],
        page: 0,
        deadline: 0,
        now: NOW,
      }),
    ).toBe(false)
  })

  it('never polls away from the head page', () => {
    expect(
      shouldPollRunLogs({ rows: [{ stage: 'RUNNING' }], page: 1, deadline: NOW + 10_000, now: NOW }),
    ).toBe(false)
  })

  it('exposes a poll interval shorter than the give-up bound', () => {
    expect(RUN_POLL_INTERVAL_MS).toBeGreaterThan(0)
    expect(RUN_POLL_MAX_MS).toBeGreaterThan(RUN_POLL_INTERVAL_MS)
  })
})
