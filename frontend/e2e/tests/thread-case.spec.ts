import { test, expect } from '@playwright/test';
import { CaseKanbanPage } from '../pages/CaseKanbanPage';
import { ActionListPage } from '../pages/ActionListPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';

// The "review" workspace (frontend/e2e/fixtures/config.review.test.toml) is
// configured in thread mode with a [case.status] set, so its board renders
// Cases instead of Actions.
const THREAD_WS = 'review';
const CHANNEL_WS = 'test';

test.describe('Thread-mode Case board', () => {
  test('renders the Case Kanban for a thread-mode workspace', async ({ page }) => {
    const board = new CaseKanbanPage(page);
    await board.navigate(THREAD_WS);
    await board.waitForBoardLoad();

    // The thread-mode Case board is shown (not the Action board).
    expect(await board.isBoardVisible()).toBeTruthy();

    // Columns mirror the [case.status] configuration.
    expect(await board.columnExists('Triage')).toBeTruthy();
    expect(await board.columnExists('In Review')).toBeTruthy();
    expect(await board.columnExists('Done')).toBeTruthy();

    // Thread mode has no Actions: the "New Action" button must be absent.
    expect(await board.hasNewActionButton()).toBeFalsy();

    // The sidebar nav item is labelled "Board" (not "Actions") in thread mode.
    const sidebar = page.locator('.h-side');
    await expect(sidebar.getByText('Board', { exact: true })).toBeVisible();
    await expect(sidebar.getByText('Actions', { exact: true })).toHaveCount(0);
  });

  test('still renders the Action board for a channel-mode workspace', async ({ page }) => {
    const actions = new ActionListPage(page);
    await actions.navigate(CHANNEL_WS);
    await actions.waitForBoardLoad();

    // The channel-mode workspace keeps the Action board (action testid).
    expect(await actions.isPageLoaded()).toBeTruthy();

    // And the thread-mode Case board must NOT be present here.
    const caseBoard = new CaseKanbanPage(page);
    expect(await caseBoard.isBoardVisible()).toBeFalsy();

    // The sidebar keeps the "Actions" label for channel-mode workspaces.
    const sidebar = page.locator('.h-side');
    await expect(sidebar.getByText('Actions', { exact: true })).toBeVisible();
  });

  test('case detail in a thread-mode workspace: no Actions, editable board status, no Close button', async ({ page }) => {
    const caseList = new CaseListPage(page);
    const caseForm = new CaseFormPage(page);
    const detail = new CaseDetailPage(page);

    // Create a case in the thread-mode workspace, then open its detail.
    await caseList.navigate(THREAD_WS);
    await caseList.waitForTableLoad();
    await caseList.clickNewCaseButton();
    await caseForm.createCase({ title: 'Thread Detail Case', description: 'detail behaviour' });
    await caseList.waitForTableLoad();
    await caseList.fillSearchFilter('Thread Detail Case');
    await caseList.clickCaseByTitle('Thread Detail Case');
    expect(await detail.isPageLoaded()).toBeTruthy();

    // bug 1: the Related Actions section is gone (its "open in actions" action is absent).
    await expect(page.getByTestId('case-open-in-actions')).toHaveCount(0);

    // close button removed: thread mode closes via the board status, not a lifecycle button.
    await expect(page.getByTestId('close-case-button')).toHaveCount(0);

    // bug 2: status is an editable board-status selector mapped to [case.status],
    // and changing it persists (no longer the read-only lifecycle Open/Closed badge).
    const status = page.getByTestId('aside-board-status');
    await expect(status).toBeVisible();
    await status.selectOption('in_review');
    await expect(status).toHaveValue('in_review');
  });
});
