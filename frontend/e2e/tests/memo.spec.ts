import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { MemoDetailPage } from '../pages/MemoDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for case memos. A memo row now opens a dedicated detail page
// with its own URL (`/ws/{ws}/cases/{id}/memos/{memoId}`) instead of a modal.
// Journeys: create → open page → archive → unarchive round trip, markdown
// rendering surviving a direct-URL reload, and editing from the page.
// Memos are enabled for the `test` workspace via config.test.toml's [memo]
// section (one optional field, so creation is title-only).
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

// Create a fresh case (the Memos section lives on its detail page) and a
// memo inside it, then leave the browser on the case detail page.
async function setupCaseWithMemo(
  page: import('@playwright/test').Page,
  caseTitle: string,
  memoTitle: string,
): Promise<void> {
  const caseListPage = new CaseListPage(page);
  const caseFormPage = new CaseFormPage(page);
  const caseDetailPage = new CaseDetailPage(page);

  await caseListPage.navigate(TEST_WORKSPACE_ID);
  await caseListPage.waitForTableLoad();
  await caseListPage.clickNewCaseButton();
  await caseFormPage.createCase({ title: caseTitle, customFields: { category: 'task' } });
  await caseListPage.waitForTableLoad();
  await caseListPage.fillSearchFilter(caseTitle);
  await caseListPage.clickCaseByTitle(caseTitle);
  await caseDetailPage.waitForPageLoad();

  await page.getByTestId('new-memo-button').click();
  await page.locator('#memo-title').fill(memoTitle);
  await page.getByTestId('memo-form-submit').click();
  await expect(page.getByTestId('memo-row').filter({ hasText: memoTitle })).toBeVisible();
}

test.describe('Case memos', () => {
  test('open a memo page, archive it, find it under Archived, and restore it', async ({ page }) => {
    const caseDetailPage = new CaseDetailPage(page);
    const memoDetailPage = new MemoDetailPage(page);

    const caseTitle = `Memo Case ${uniq()}`;
    const memoTitle = `Memo ${uniq()}`;
    await setupCaseWithMemo(page, caseTitle, memoTitle);

    const memoRow = () => page.getByTestId('memo-row').filter({ hasText: memoTitle });

    // A row click navigates to the dedicated memo page (own URL).
    await memoRow().click();
    await memoDetailPage.waitForLoaded();
    await expect(page).toHaveURL(/\/memos\/[A-Za-z0-9-]+$/);
    await expect(memoDetailPage.title).toHaveText(memoTitle);

    // Archive through the confirm dialog; the page flips to the archived
    // state in place (badge + unarchive) without navigating away.
    await memoDetailPage.archive();
    await expect(page).toHaveURL(/\/memos\/[A-Za-z0-9-]+$/);
    await expect(memoDetailPage.unarchiveButton).toBeVisible();
    await expect(memoDetailPage.editButton).toHaveCount(0);

    // Back on the case: it drops off the Active list…
    await memoDetailPage.backToCase();
    await caseDetailPage.waitForPageLoad();
    await page.getByTestId('memo-filter-active').click();
    await expect(memoRow()).toHaveCount(0);

    // …and shows up under the Archived filter.
    await page.getByTestId('memo-filter-archived').click();
    await expect(memoRow()).toBeVisible();

    // Restore it from its page; the Active list has it back.
    await memoRow().click();
    await memoDetailPage.waitForLoaded();
    await memoDetailPage.unarchive();
    await memoDetailPage.backToCase();
    await caseDetailPage.waitForPageLoad();
    await page.getByTestId('memo-filter-active').click();
    await expect(memoRow()).toBeVisible();
  });

  test('edit a memo from its page and persist the change across a reload', async ({ page }) => {
    const memoDetailPage = new MemoDetailPage(page);

    const caseTitle = `Edit Case ${uniq()}`;
    const memoTitle = `Memo ${uniq()}`;
    const renamedTitle = `Renamed ${uniq()}`;
    await setupCaseWithMemo(page, caseTitle, memoTitle);

    await page.getByTestId('memo-row').filter({ hasText: memoTitle }).click();
    await memoDetailPage.waitForLoaded();

    await memoDetailPage.editTitle(renamedTitle);
    await expect(memoDetailPage.title).toHaveText(renamedTitle);

    // Reload the memo URL directly: the rename round-tripped to the backend.
    await page.reload();
    await memoDetailPage.waitForLoaded();
    await expect(memoDetailPage.title).toHaveText(renamedTitle);
  });

  test('create a memo with a markdown body, preview it, and see it rendered on the memo page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const memoDetailPage = new MemoDetailPage(page);

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

    // Open the memo page: the markdown body is rendered, including the GFM table.
    await memoRow().click();
    await memoDetailPage.waitForLoaded();
    await expect(page).toHaveURL(/\/memos\/[A-Za-z0-9-]+$/);
    const bodyHeading = page.getByRole('heading', { level: 1, name: 'Heading' });
    await expect(bodyHeading).toBeVisible();
    await expect(page.getByText('bold item')).toBeVisible();
    await expect(page.getByRole('table')).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell one' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell two' })).toBeVisible();

    // Reload the memo URL directly (deep link) to confirm the value
    // round-tripped to the backend and the page renders standalone.
    await page.reload();
    await memoDetailPage.waitForLoaded();
    await expect(page.getByRole('heading', { level: 1, name: 'Heading' })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'cell one' })).toBeVisible();
  });
});
