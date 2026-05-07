import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Action modal scroll behavior', () => {
  test('modal body scrolls when content overflows and the footer stays visible', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Constrain the viewport so the modal contents overflow vertically.
    await page.setViewportSize({ width: 1024, height: 480 });

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Modal Scroll Case',
      customFields: { category: 'task' },
    });

    await caseListPage.fillSearchFilter('Modal Scroll Case');
    await caseListPage.clickCaseByTitle('Modal Scroll Case');
    await caseDetailPage.waitForPageLoad();

    await page.getByTestId('add-action-button').click();
    await actionFormPage.createAction({
      title: 'Scroll Test Action',
      caseTitle: 'Modal Scroll Case',
    });

    // Open the action modal by clicking the related-action link.
    const link = page.locator('a').filter({ hasText: 'Scroll Test Action' }).first();
    await link.click();
    await expect(page.getByTestId('action-title')).toBeVisible();

    // The modal body must overflow vertically so it can scroll on its own.
    const body = page.locator('.modal-b');
    await expect(body).toBeVisible();
    const overflowsBeforeScroll = await body.evaluate(
      (el) => el.scrollHeight > el.clientHeight + 1,
    );
    expect(overflowsBeforeScroll).toBe(true);

    // Footer Archive button must already be visible — i.e. the card fits within
    // the viewport instead of being clipped past the bottom edge.
    const archive = page.getByTestId('action-archive-button');
    await expect(archive).toBeVisible();

    // Scrolling the body should actually move scrollTop (proves overflow is on
    // the body, not the page).
    await body.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    const scrolled = await body.evaluate((el) => el.scrollTop);
    expect(scrolled).toBeGreaterThan(0);

    // Footer is fixed within the card: still visible after scrolling and still
    // clickable. clickability is what the original bug took away.
    await expect(archive).toBeVisible();
    await archive.click();
    await expect(page.getByTestId('action-archive-confirm-button')).toBeVisible();
  });
});
