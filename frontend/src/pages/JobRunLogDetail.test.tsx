import { describe, it, expect } from 'vitest'

import { buildRunExport } from './JobRunLogDetail'

const log = {
  workspaceId: 'risk-review',
  caseId: 18,
  jobId: 'job-1',
  jobName: 'Nightly review',
  runId: '019ef1ab-6a51-7e45-8ec7-441aafa0f5f5',
  traceId: 'trace-1',
  stage: 'SUCCESS' as const,
  startedAt: '2026-06-23T00:00:00.000Z',
  endedAt: '2026-06-23T00:01:00.000Z',
  durationMs: 60000,
  errorMessage: '',
  systemPrompt: 'You are a helpful agent.\nFollow the rules.',
  eventType: 'scheduled',
  eventTriggerAt: '2026-06-23T00:00:00.000Z',
}

const exportedAt = '2026-06-23T09:00:00.000Z'

describe('buildRunExport', () => {
  it('preserves every runLog field verbatim', () => {
    const result = buildRunExport(log, [], exportedAt)
    expect(result.exportedAt).toBe(exportedAt)
    expect(result.runLog).toEqual(log)
    // The long-form systemPrompt must be included in full, not truncated.
    expect(result.runLog.systemPrompt).toBe('You are a helpful agent.\nFollow the rules.')
  })

  it('orders events by sequence and decodes JSON payloads to objects', () => {
    const events = [
      {
        eventId: 'e2',
        runId: log.runId,
        sequence: 2,
        occurredAt: '2026-06-23T00:00:30.000Z',
        kind: 'LLM_RESPONSE' as const,
        parentSequence: 1,
        phase: 'execute',
        agentLabel: 'planner',
        payload: JSON.stringify({ InputTokens: 10, OutputTokens: 20 }),
      },
      {
        eventId: 'e1',
        runId: log.runId,
        sequence: 1,
        occurredAt: '2026-06-23T00:00:10.000Z',
        kind: 'LLM_REQUEST' as const,
        parentSequence: 0,
        phase: 'execute',
        agentLabel: 'planner',
        payload: JSON.stringify({ Model: 'claude', Messages: [{ Role: 'user' }] }),
      },
    ]
    const result = buildRunExport(log, events, exportedAt)
    expect(result.events.map((e) => e.sequence)).toEqual([1, 2])
    expect(result.events[0].payload).toEqual({ Model: 'claude', Messages: [{ Role: 'user' }] })
    expect(result.events[1].payload).toEqual({ InputTokens: 10, OutputTokens: 20 })
    // Non-payload event fields survive the transform.
    expect(result.events[0].eventId).toBe('e1')
    expect(result.events[0].kind).toBe('LLM_REQUEST')
    expect(result.events[0].agentLabel).toBe('planner')
  })

  it('keeps an unparseable payload as the original string', () => {
    const events = [
      {
        eventId: 'e1',
        runId: log.runId,
        sequence: 1,
        occurredAt: '2026-06-23T00:00:10.000Z',
        kind: 'RUN_ERROR' as const,
        parentSequence: 0,
        phase: 'execute',
        agentLabel: '',
        payload: 'not-json{',
      },
    ]
    const result = buildRunExport(log, events, exportedAt)
    expect(result.events[0].payload).toBe('not-json{')
  })

  it('returns an empty events array when there are no events', () => {
    const result = buildRunExport(log, [], exportedAt)
    expect(result.events).toEqual([])
  })

  it('does not mutate the input events array', () => {
    const events = [
      {
        eventId: 'e2',
        runId: log.runId,
        sequence: 2,
        occurredAt: '2026-06-23T00:00:30.000Z',
        kind: 'TOOL_CALL' as const,
        parentSequence: 1,
        phase: 'execute',
        agentLabel: 'worker',
        payload: '{}',
      },
      {
        eventId: 'e1',
        runId: log.runId,
        sequence: 1,
        occurredAt: '2026-06-23T00:00:10.000Z',
        kind: 'LLM_REQUEST' as const,
        parentSequence: 0,
        phase: 'execute',
        agentLabel: 'planner',
        payload: '{}',
      },
    ]
    buildRunExport(log, events, exportedAt)
    expect(events.map((e) => e.eventId)).toEqual(['e2', 'e1'])
  })
})
