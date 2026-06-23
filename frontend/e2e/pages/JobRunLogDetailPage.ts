import { Page, Locator, Download } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the agent Job Run log detail page
 * (`/ws/{workspaceId}/cases/{caseId}/agent/runs/{runId}`).
 *
 * Job run records are produced only by the agent runtime — there is no
 * GraphQL mutation to seed them — so tests drive this page by intercepting
 * the GetJobRunLog / GetJobRunEvents operations with canned responses. The
 * helpers here keep that wiring in one place.
 */
export class JobRunLogDetailPage extends BasePage {
  readonly downloadButton: Locator;

  constructor(page: Page) {
    super(page);
    this.downloadButton = page.getByTestId('job-run-download-json');
  }

  /**
   * Intercept the two run-log GraphQL operations and return the supplied
   * fixtures. Every other GraphQL operation (auth/me, workspace config, …)
   * is passed through to the real backend so the app shell still renders.
   */
  async mockRunLog(
    runLog: Record<string, unknown>,
    events: Array<Record<string, unknown>>,
  ): Promise<void> {
    await this.page.route('**/graphql', async (route) => {
      const body = route.request().postDataJSON() as
        | { operationName?: string; query?: string }
        | undefined;
      const op = body?.operationName ?? ''
      const query = body?.query ?? ''
      const isLog = op === 'GetJobRunLog' || query.includes('jobRunLog(')
      const isEvents = op === 'GetJobRunEvents' || query.includes('jobRunEvents(')
      if (isLog) {
        await route.fulfill({
          contentType: 'application/json',
          body: JSON.stringify({ data: { jobRunLog: runLog } }),
        });
        return;
      }
      if (isEvents) {
        await route.fulfill({
          contentType: 'application/json',
          body: JSON.stringify({ data: { jobRunEvents: events } }),
        });
        return;
      }
      await route.continue();
    });
  }

  async navigate(workspaceId: string, caseId: number, runId: string): Promise<void> {
    await this.goto(`/ws/${workspaceId}/cases/${caseId}/agent/runs/${runId}`);
    await this.waitForVisible(this.downloadButton);
  }

  /**
   * Click the download button and return the captured Download so the test
   * can read and assert on the exported file's contents.
   */
  async downloadJson(): Promise<Download> {
    const [download] = await Promise.all([
      this.page.waitForEvent('download'),
      this.downloadButton.click(),
    ]);
    return download;
  }
}
