import { test, expect } from '@playwright/test';
import { CaseKanbanPage } from '../pages/CaseKanbanPage';
import { ActionListPage } from '../pages/ActionListPage';

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
  });
});
