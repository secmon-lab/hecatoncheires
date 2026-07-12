import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('UI overhaul regressions', () => {
  test('root path always shows the workspace selector', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('workspace-selector')).toBeVisible();
  });

  test('CaseDetail header: Close is danger-styled, Delete is in kebab menu', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Header Layout Case',
      customFields: { category: 'task' },
    });

    await caseListPage.fillSearchFilter('Header Layout Case');
    await caseListPage.clickCaseByTitle('Header Layout Case');
    await caseDetailPage.waitForPageLoad();

    // Close button is rendered with the danger variant (red)
    const closeBtn = page.getByTestId('close-case-button');
    await expect(closeBtn).toBeVisible();
    const closeClass = await closeBtn.getAttribute('class');
    expect(closeClass || '').toContain('danger');

    // Delete is hidden behind the kebab menu, not directly visible
    await expect(page.getByTestId('case-delete-menu-item')).toBeHidden();
    await page.getByTestId('case-menu-button').click();
    await expect(page.getByTestId('case-delete-menu-item')).toBeVisible();
  });

  test('related actions list: clicking an item opens the action modal in-place', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const actionFormPage = new ActionFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Related Action Case',
      customFields: { category: 'task' },
    });

    await caseListPage.fillSearchFilter('Related Action Case');
    await caseListPage.clickCaseByTitle('Related Action Case');
    await caseDetailPage.waitForPageLoad();

    // Add an action via the case detail Add Action button
    await page.getByTestId('add-action-button').click();
    await actionFormPage.createAction({
      title: 'Inline Related Action',
      caseTitle: 'Related Action Case',
    });

    // Click the related action — modal should open and URL should reflect it
    const link = page.locator('a').filter({ hasText: 'Inline Related Action' }).first();
    await link.click();
    await expect(page).toHaveURL(/\/actions\/\d+$/);
    await expect(page.getByTestId('action-title')).toContainText('Inline Related Action');
  });

  test('Action kanban: drag and drop changes the column', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'DnD Parent Case',
      customFields: { category: 'task' },
    });

    await actionListPage.navigate(TEST_WORKSPACE_ID);
    await actionListPage.waitForBoardLoad();
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'DnD Action',
      caseTitle: 'DnD Parent Case',
    });
    await actionListPage.waitForBoardLoad();

    // Filter the kanban to just the action under test so unrelated seeded
    // actions don't perturb the column counts.
    await actionListPage.searchActions('DnD Action');
    expect(await actionListPage.getColumnCount('Backlog')).toBe(1);
    expect(await actionListPage.getColumnCount('In Progress')).toBe(0);

    await actionListPage.dragActionToColumn('DnD Action', 'In Progress');

    // Re-load to make sure the change was persisted
    await page.reload();
    await actionListPage.waitForBoardLoad();
    await actionListPage.searchActions('DnD Action');
    expect(await actionListPage.getColumnCount('Backlog')).toBe(0);
    expect(await actionListPage.getColumnCount('In Progress')).toBe(1);
  });

  test('react-select dropdowns are not clipped by the modal (use portal)', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.waitForFormVisible();

    // react-select hides <input id="category"> behind a styled control
    // wrapper; clicking the wrapper is the reliable open path. The portal
    // div is always mounted, so assert visibility on the actual menu list
    // and assert it lives outside the modal (i.e., portaled to body).
    await page.locator('.rs__control', { has: page.locator('#category') }).click();
    const menuList = page.locator('.rs__menu-list');
    await expect(menuList).toBeVisible();
    expect(await page.locator('[role="dialog"] .rs__menu-list').count()).toBe(0);
    expect(await page.locator('.rs__menu-portal .rs__menu-list').count()).toBeGreaterThan(0);
  });

  test('SELECT placeholder is no longer the ugly "-- Select --" string', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.waitForFormVisible();

    const placeholders = await page.locator('.rs__placeholder').allTextContents();
    for (const p of placeholders) {
      expect(p).not.toContain('-- Select --');
      expect(p).not.toContain('-- 選択 --');
    }
  });
});
