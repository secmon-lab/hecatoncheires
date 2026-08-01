// Polling policy for the Case Agent run-log table.
//
// A manually triggered Job run appears in the table a moment after the
// mutation returns (the backend writes the RUNNING log before it calls the
// LLM), so the page polls to surface it and to follow it to its terminal
// stage. The policy lives here, apart from the component, so its two edge
// cases stay testable.

/** How often the run-log query refetches while polling is active. */
export const RUN_POLL_INTERVAL_MS = 5000

/**
 * How long a just-issued trigger keeps the page polling before giving up on
 * seeing its RUNNING row. Without this bound, a run that starts and finishes
 * between two polls would never be observed as RUNNING and the page would
 * poll forever.
 */
export const RUN_POLL_MAX_MS = 60000

export interface RunPollInput {
  /** The run-log rows currently rendered. */
  rows: { stage: string }[]
  /** Cursor page index; only the head page shows the newest runs. */
  page: number
  /** Epoch ms set when a trigger was issued; 0 when none is pending. */
  deadline: number
  /** Current epoch ms. */
  now: number
}

/**
 * shouldPollRunLogs reports whether the run-log query should keep polling.
 * Polling continues while a RUNNING row is visible, or while a just-issued
 * trigger has not yet produced one and its deadline has not passed.
 *
 * Paging away from the head stops polling: older pages are immutable
 * history, and refetching them would fight the cursor the user navigated to.
 */
export function shouldPollRunLogs({ rows, page, deadline, now }: RunPollInput): boolean {
  if (page !== 0) return false
  if (rows.some((r) => r.stage === 'RUNNING')) return true
  return now < deadline
}
