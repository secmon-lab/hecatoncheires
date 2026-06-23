import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';

import { JobRunLogDetailPage } from '../pages/JobRunLogDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

const CASE_ID = 18;
const RUN_ID = '019ef1ab-6a51-7e45-8ec7-441aafa0f5f5';

const runLog = {
  __typename: 'JobRunLog',
  workspaceId: TEST_WORKSPACE_ID,
  caseId: CASE_ID,
  jobId: 'nightly-review',
  jobName: 'Nightly review',
  strategy: 'SIMPLE',
  runId: RUN_ID,
  traceId: 'trace-abc',
  stage: 'SUCCESS',
  startedAt: '2026-06-23T00:00:00.000Z',
  endedAt: '2026-06-23T00:01:00.000Z',
  durationMs: 60000,
  errorMessage: '',
  systemPrompt: 'You are a helpful agent.\nFollow the rules.',
  eventType: 'scheduled',
  eventTriggerAt: '2026-06-23T00:00:00.000Z',
};

// Two events, intentionally out of sequence order, plus an event whose
// payload is not valid JSON — exercising the sort and the parse-fallback.
const events = [
  {
    __typename: 'JobRunEvent',
    eventId: 'evt-2',
    runId: RUN_ID,
    sequence: 2,
    occurredAt: '2026-06-23T00:00:30.000Z',
    kind: 'TOOL_CALL',
    parentSequence: 1,
    phase: 'execute',
    agentLabel: '',
    payload: 'not-json{',
  },
  {
    __typename: 'JobRunEvent',
    eventId: 'evt-1',
    runId: RUN_ID,
    sequence: 1,
    occurredAt: '2026-06-23T00:00:10.000Z',
    kind: 'LLM_REQUEST',
    parentSequence: 0,
    phase: 'execute',
    agentLabel: '',
    payload: JSON.stringify({ Model: 'claude', Messages: [{ Role: 'user' }] }),
  },
];

test.describe('Job Run log detail — JSON download', () => {
  test('downloads the full execution record as JSON', async ({ page }) => {
    const runPage = new JobRunLogDetailPage(page);
    await runPage.mockRunLog(runLog, events);
    await runPage.navigate(TEST_WORKSPACE_ID, CASE_ID, RUN_ID);

    const download = await runPage.downloadJson();

    // Filename identifies the run uniquely.
    expect(download.suggestedFilename()).toBe(`jobrun-${CASE_ID}-${RUN_ID}.json`);

    const path = await download.path();
    const record = JSON.parse(readFileSync(path, 'utf-8'));

    // exportedAt is a client-stamped ISO timestamp.
    expect(typeof record.exportedAt).toBe('string');
    expect(Number.isNaN(Date.parse(record.exportedAt))).toBe(false);

    // runLog is carried verbatim, including the full system prompt.
    expect(record.runLog.runId).toBe(RUN_ID);
    expect(record.runLog.jobName).toBe('Nightly review');
    expect(record.runLog.stage).toBe('SUCCESS');
    expect(record.runLog.systemPrompt).toBe('You are a helpful agent.\nFollow the rules.');

    // Events are sorted by sequence and payloads decoded to nested JSON.
    expect(record.events.map((e: { sequence: number }) => e.sequence)).toEqual([1, 2]);
    expect(record.events[0].kind).toBe('LLM_REQUEST');
    expect(record.events[0].payload).toEqual({ Model: 'claude', Messages: [{ Role: 'user' }] });

    // The unparseable payload is preserved as the original string.
    expect(record.events[1].payload).toBe('not-json{');
  });
});
