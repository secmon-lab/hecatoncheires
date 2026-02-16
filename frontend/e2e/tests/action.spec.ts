import { test, expect } from '@playwright/test';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { ActionDetailPage } from '../pages/ActionDetailPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Action Management', () => {
  test.beforeEach(async ({ page }) => {
    // Create a test case first (Actions require a parent case)
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Parent Case for Actions',
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

    // Click "New Action" button
    await actionListPage.clickNewActionButton();

    // Fill in the form
    await actionFormPage.createAction({
      title: 'E2E Test Action',
      description: 'This is a test action created by Playwright',
      caseTitle: 'Parent Case for Actions',
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Verify the action appears on the kanban board
    const exists = await actionListPage.actionExists('E2E Test Action');
    expect(exists).toBeTruthy();
  });

  test('should open action modal when clicking a card', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    // Create a test action first
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Action for Modal View',
      description: 'Testing action modal view',
      caseTitle: 'Parent Case for Actions',
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Click on the action card to open modal
    await actionListPage.clickActionByTitle('Action for Modal View');

    // Verify modal is loaded
    const isLoaded = await actionDetailPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();

    // Verify the title matches
    const title = await actionDetailPage.getTitle();
    expect(title).toContain('Action for Modal View');
  });

  test('should list multiple actions on kanban', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Create multiple actions
    const actionCount = 3;
    for (let i = 1; i <= actionCount; i++) {
      await actionListPage.clickNewActionButton();
      await actionFormPage.createAction({
        title: `Action ${i}`,
        description: `Description for action ${i}`,
        caseTitle: 'Parent Case for Actions',
      });
    }

    // Reload to get fresh data
    await page.reload();
    await actionListPage.waitForBoardLoad();

    // Verify all actions are listed
    for (let i = 1; i <= actionCount; i++) {
      const exists = await actionListPage.actionExists(`Action ${i}`);
      expect(exists).toBeTruthy();
    }
  });

  test('should create an action without description', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Click "New Action" button
    await actionListPage.clickNewActionButton();

    // Create action with title only (no description)
    await actionFormPage.createAction({
      title: 'Action Without Description',
      caseTitle: 'Parent Case for Actions',
    });

    // Wait for board to update
    await actionListPage.waitForBoardLoad();

    // Verify the action appears on the kanban board
    const exists = await actionListPage.actionExists('Action Without Description');
    expect(exists).toBeTruthy();
  });

  test('should change action status from modal', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    // Create an action
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Action for Status Change',
      description: 'Testing inline status change',
      caseTitle: 'Parent Case for Actions',
    });

    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle('Action for Status Change');
    await actionDetailPage.waitForPageLoad();

    // Verify initial status is TODO
    const initialStatus = await actionDetailPage.getStatus();
    expect(initialStatus).toBe('TODO');

    // Change status to IN_PROGRESS
    await actionDetailPage.changeStatus('IN_PROGRESS');

    // Verify status updated
    const newStatus = await actionDetailPage.getStatus();
    expect(newStatus).toBe('IN_PROGRESS');

    // Reload and verify persistence
    await page.reload();
    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle('Action for Status Change');
    await actionDetailPage.waitForPageLoad();
    const persistedStatus = await actionDetailPage.getStatus();
    expect(persistedStatus).toBe('IN_PROGRESS');
  });

  test('should change action status to completed', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    // Create an action
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Action to Complete',
      description: 'Testing completion',
      caseTitle: 'Parent Case for Actions',
    });

    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle('Action to Complete');
    await actionDetailPage.waitForPageLoad();

    // Change status to COMPLETED
    await actionDetailPage.changeStatus('COMPLETED');

    // Verify status updated
    const status = await actionDetailPage.getStatus();
    expect(status).toBe('COMPLETED');

    // Reload and verify persistence
    await page.reload();
    await actionListPage.waitForBoardLoad();
    await actionListPage.clickActionByTitle('Action to Complete');
    await actionDetailPage.waitForPageLoad();
    const persistedStatus = await actionDetailPage.getStatus();
    expect(persistedStatus).toBe('COMPLETED');
  });

  test('should filter actions by search text', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Create actions with distinct names
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Alpha Task',
      caseTitle: 'Parent Case for Actions',
    });

    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Beta Task',
      caseTitle: 'Parent Case for Actions',
    });

    await actionListPage.waitForBoardLoad();

    // Search for "Alpha"
    await actionListPage.searchActions('Alpha');
    // Wait for filter to apply
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));

    expect(await actionListPage.actionExists('Alpha Task')).toBeTruthy();
    expect(await actionListPage.actionExists('Beta Task')).toBeFalsy();

    // Clear filter and verify both appear
    await actionListPage.clearFilters();
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));

    expect(await actionListPage.actionExists('Alpha Task')).toBeTruthy();
    expect(await actionListPage.actionExists('Beta Task')).toBeTruthy();
  });
});
