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

  test('create a memo with a markdown body, preview it, and see it rendered in the detail modal', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const caseTitle = `Markdown Case ${uniq()}`;
    const memoTitle = `Markdown Memo ${uniq()}`;

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title: caseTitle, customFields: { category: 'task' } });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(caseTitle);
    await caseListPage.clickCaseByTitle(caseTitle);
    await caseDetailPage.waitForPageLoad();

    const memoRow = () => page.getByTestId('memo-row').filter({ hasText: memoTitle });

    // Open the memo form and fill the markdown 'body' field. The body includes a
    // GFM table, which react-markdown only renders when remark-gfm is wired in.
    await page.getByTestId('new-memo-button').click();
    await page.locator('#memo-title').fill(memoTitle);
    const markdownSource = [
      '# Heading',
      '',
      '- alpha',
      '- **bold item**',
      '',
      '| Col A | Col B |',
      '| --- | --- |',
      '| cell one | cell two |',
    ].join('\n');
    await page.getByTestId('body-textarea').fill(markdownSource);

    // The Preview tab renders the markdown (heading + list + bold + GFM table),
    // not raw markers.
    await page.getByTestId('body-tab-preview').click();
    const preview = page.getByTestId('body-preview');
    await expect(preview.locator('h1')).toHaveText('Heading');
    await expect(preview.locator('li')).toHaveCount(2);
    await expect(preview.locator('strong')).toHaveText('bold item');
    await expect(preview).not.toContainText('**');
    // GFM table renders as a real <table>; the pipe source is not shown literally.
    await expect(preview.locator('table')).toBeVisible();
    await expect(preview.locator('th')).toHaveCount(2);
    await expect(preview.locator('tbody td')).toHaveText(['cell one', 'cell two']);
    await expect(preview).not.toContainText('| Col A |');

    // Back to Write tab: the raw source is preserved.
    await page.getByTestId('body-tab-write').click();
    await expect(page.getByTestId('body-textarea')).toHaveValue(markdownSource);

    await page.getByTestId('memo-form-submit').click();
    await expect(memoRow()).toBeVisible();

    // Open the detail modal: the markdown body is rendered, including the GFM table.
    await memoRow().click();
    const modalHeading = page.getByRole('heading', { level: 1, name: 'Heading' });
    await expect(modalHeading).toBeVisible();
    await expect(page.getByText('bold item')).toBeVisible();
    await expect(page.getByRole('table')).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell one' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell two' })).toBeVisible();

    // Reload the whole page and re-open to confirm the value round-tripped to the backend.
    await page.reload();
    await caseDetailPage.waitForPageLoad();
    await memoRow().click();
    await expect(page.getByRole('heading', { level: 1, name: 'Heading' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell one' })).toBeVisible();
  });
});
