import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for the case Memos tab (MemoTab), previously untested end to
// end. Drives the core journey: create → appears in the Active list → archive
// (with confirm dialog) → drops off Active → surfaces under the Archived
// filter. Memos are enabled for the `test` workspace via config.test.toml's
// [memo] section (one optional field, so creation is title-only).
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Case memos', () => {
  test('create a memo, then archive it and find it under the Archived filter', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const caseTitle = `Memo Case ${uniq()}`;
    const memoTitle = `Memo ${uniq()}`;

    // Create a case and open its detail page (the Memos section lives there).
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title: caseTitle, customFields: { category: 'task' } });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(caseTitle);
    await caseListPage.clickCaseByTitle(caseTitle);
    await caseDetailPage.waitForPageLoad();

    const memoRow = () => page.getByTestId('memo-row').filter({ hasText: memoTitle });

    // Create a memo (title only; the single memo field is optional).
    await page.getByTestId('new-memo-button').click();
    await page.locator('#memo-title').fill(memoTitle);
    await page.getByTestId('memo-form-submit').click();

    // It appears in the default Active list.
    await expect(memoRow()).toBeVisible();

    // Open it and archive it through the confirm dialog.
    await memoRow().click();
    await page.getByTestId('memo-detail-archive-button').click();
    await page.getByTestId('memo-archive-confirm-button').click();

    // It drops off the Active list…
    await page.getByTestId('memo-filter-active').click();
    await expect(memoRow()).toHaveCount(0);

    // …and shows up under the Archived filter.
    await page.getByTestId('memo-filter-archived').click();
    await expect(memoRow()).toBeVisible();
  });
});
