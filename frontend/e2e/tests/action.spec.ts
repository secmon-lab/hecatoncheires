import { test, expect } from '@playwright/test';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { ActionDetailPage } from '../pages/ActionDetailPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// Unique suffix per created entity. The e2e backend is a single in-memory
// process shared across all workers with no per-test reset, and the suite
// runs fullyParallel — so generic reused titles (the old 'Parent Case for
// Actions', 'Action 1'…) collided across concurrent tests and produced flaky
// board/card lookups. Every title below is made unique so each test only ever
// matches its own data.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Action Management', () => {
  // Each test gets its own parent case; set in beforeEach and read by the test
  // that immediately follows it (Playwright runs beforeEach + its test back to
  // back on one worker, so this module-scoped handoff is race-free).
  let parentCaseTitle: string;

  test.beforeEach(async ({ page }) => {
    // Create a test case first (Actions require a parent case)
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    parentCaseTitle = `Parent Case ${uniq()}`;

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: parentCaseTitle,
      description: 'Case to hold test actions',
      customFields: {
        category: 'task',
      },
    });

    // Navigate to action list page
    const actionListPage = new ActionListPage(page);
    await actionListPage.navigate(TEST_WORKSPACE_ID);
    await actionListPage.waitForBoardLoad();
  });

  test('should display kanban board with 5 columns', async ({ page }) => {
    const actionListPage = new ActionListPage(page);

    // Verify the page is loaded
    const isLoaded = await actionListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();

    // Verify all 5 kanban columns exist
    expect(await actionListPage.columnExists('Backlog')).toBeTruthy();
    expect(await actionListPage.columnExists('To Do')).toBeTruthy();
    expect(await actionListPage.columnExists('In Progress')).toBeTruthy();
    expect(await actionListPage.columnExists('Blocked')).toBeTruthy();
    expect(await actionListPage.columnExists('Completed')).toBeTruthy();
  });

  test('should create a new action and show it on kanban', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const title = `E2E Test Action ${uniq()}`;

    // Click "New Action" button
    await actionListPage.clickNewActionButton();

    // Fill in the form
    await actionFormPage.createAction({
      title,
      description: 'This is a test action created by Playwright',
      caseTitle: parentCaseTitle,
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Verify the action appears on the kanban board
    const exists = await actionListPage.actionExists(title);
    expect(exists).toBeTruthy();
  });

  test('should open action modal when clicking a card', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);
    const title = `Action for Modal View ${uniq()}`;

    // Create a test action first
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title,
      description: 'Testing action modal view',
      caseTitle: parentCaseTitle,
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Click on the action card to open modal
    await actionListPage.clickActionByTitle(title);

    // Verify modal is loaded
    const isLoaded = await actionDetailPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();

    // Verify the title matches
    const modalTitle = await actionDetailPage.getTitle();
    expect(modalTitle).toContain(title);
  });

  test('should list multiple actions on kanban', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const run = uniq();

    // Create multiple actions
    const actionCount = 3;
    const titles: string[] = [];
    for (let i = 1; i <= actionCount; i++) {
      const title = `Action ${i} ${run}`;
      titles.push(title);
      await actionListPage.clickNewActionButton();
      await actionFormPage.createAction({
        title,
        description: `Description for action ${i}`,
        caseTitle: parentCaseTitle,
      });
    }

    // Reload to get fresh data
    await page.reload();
    await actionListPage.waitForBoardLoad();

    // Verify all actions are listed
    for (const title of titles) {
      const exists = await actionListPage.actionExists(title);
      expect(exists).toBeTruthy();
    }
  });

  test('should create an action without description', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const title = `Action Without Description ${uniq()}`;

    // Click "New Action" button
    await actionListPage.clickNewActionButton();

    // Create action with title only (no description)
    await actionFormPage.createAction({
      title,
      caseTitle: parentCaseTitle,
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Verify the action appears on the kanban board
    const exists = await actionListPage.actionExists(title);
    expect(exists).toBeTruthy();
  });

  test('should change action status from modal', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);
    const title = `Action for Status Change ${uniq()}`;

    // Create an action
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title,
      description: 'Testing inline status change',
      caseTitle: parentCaseTitle,
    });

    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle(title);
    await actionDetailPage.waitForPageLoad();

    // Verify initial status is BACKLOG
    const initialStatus = await actionDetailPage.getStatus();
    expect(initialStatus).toBe('BACKLOG');

    // Change status to IN_PROGRESS
    await actionDetailPage.changeStatus('IN_PROGRESS');

    // Verify status updated
    const newStatus = await actionDetailPage.getStatus();
    expect(newStatus).toBe('IN_PROGRESS');

    // Reload and verify persistence (URL still has action ID, so modal re-opens)
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    const persistedStatus = await actionDetailPage.getStatus();
    expect(persistedStatus).toBe('IN_PROGRESS');
  });

  test('should change action status to completed', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);
    const title = `Action to Complete ${uniq()}`;

    // Create an action
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title,
      description: 'Testing completion',
      caseTitle: parentCaseTitle,
    });

    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle(title);
    await actionDetailPage.waitForPageLoad();

    // Change status to COMPLETED
    await actionDetailPage.changeStatus('COMPLETED');

    // Verify status updated
    const status = await actionDetailPage.getStatus();
    expect(status).toBe('COMPLETED');

    // Reload and verify persistence (URL still has action ID, so modal re-opens)
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    const persistedStatus = await actionDetailPage.getStatus();
    expect(persistedStatus).toBe('COMPLETED');
  });

  test('should bulk archive every action in the completed column', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);
    const run = uniq();
    const alpha = `Done Alpha ${run}`;
    const beta = `Done Beta ${run}`;

    // Create two actions and move both into the Completed column.
    for (const title of [alpha, beta]) {
      await actionListPage.clickNewActionButton();
      await actionFormPage.createAction({
        title,
        caseTitle: parentCaseTitle,
      });
      await actionListPage.waitForBoardLoad();
      await actionListPage.clickActionByTitle(title);
      await actionDetailPage.waitForPageLoad();
      await actionDetailPage.changeStatus('COMPLETED');
      // Return to the board (closing the modal) before the next iteration.
      await actionListPage.navigate(TEST_WORKSPACE_ID);
      await actionListPage.waitForBoardLoad();
    }

    // Both completed actions are on the board.
    await expect(actionListPage.getActionCardByTitle(alpha)).toBeVisible();
    await expect(actionListPage.getActionCardByTitle(beta)).toBeVisible();

    // The bulk-archive menu is exposed only on the completed (closed) column.
    expect(await actionListPage.isColumnMenuVisible('Completed')).toBeTruthy();
    expect(await actionListPage.isColumnMenuVisible('To Do')).toBeFalsy();
    expect(await actionListPage.isColumnMenuVisible('In Progress')).toBeFalsy();

    // Archive all actions in the Completed column.
    await actionListPage.archiveAllInColumn('Completed');

    // Both completed actions disappear from the board.
    await expect(actionListPage.getActionCardByTitle(alpha)).toBeHidden();
    await expect(actionListPage.getActionCardByTitle(beta)).toBeHidden();

    // The archive round-trips to the backend: still gone after a reload.
    await page.reload();
    await actionListPage.waitForBoardLoad();
    await expect(actionListPage.getActionCardByTitle(alpha)).toBeHidden();
    await expect(actionListPage.getActionCardByTitle(beta)).toBeHidden();
  });

  test('bulk archive menu appears only on the Completed column', async ({ page }) => {
    const actionListPage = new ActionListPage(page);

    // The bulk-archive control is a property of the closed/Completed column
    // alone; the open columns never carry it. We deliberately do NOT assert
    // "the archive item is disabled because Completed is empty" — the board
    // shows every action in the workspace, so on the shared in-memory backend
    // another test's completed action could legitimately populate the column.
    // The enabled path is covered by the bulk-archive test above, which owns
    // the completed cards it reasons about.
    expect(await actionListPage.isColumnMenuVisible('Completed')).toBeTruthy();
    expect(await actionListPage.isColumnMenuVisible('Backlog')).toBeFalsy();
    expect(await actionListPage.isColumnMenuVisible('To Do')).toBeFalsy();
    expect(await actionListPage.isColumnMenuVisible('In Progress')).toBeFalsy();
    expect(await actionListPage.isColumnMenuVisible('Blocked')).toBeFalsy();
  });

  test('should filter actions by search text', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const run = uniq();
    const alpha = `Alpha Task ${run}`;
    const beta = `Beta Task ${run}`;

    // Create actions with distinct names
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: alpha,
      caseTitle: parentCaseTitle,
    });

    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: beta,
      caseTitle: parentCaseTitle,
    });

    await actionListPage.waitForBoardLoad();

    // Search for the alpha action — wait for beta to disappear as filter confirmation
    await actionListPage.searchActions(alpha);
    await expect(actionListPage.getActionCardByTitle(beta)).toBeHidden();
    await expect(actionListPage.getActionCardByTitle(alpha)).toBeVisible();

    // Clear filter and verify both appear
    await actionListPage.clearFilters();
    await expect(actionListPage.getActionCardByTitle(alpha)).toBeVisible();
    await expect(actionListPage.getActionCardByTitle(beta)).toBeVisible();
  });
});
