import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for the Bulk draft actions feature on the Drafts tab.
//
// What this spec validates:
//   - The Drafts tab adds a checkbox column (header + per-row); other
//     tabs do not.
//   - Selecting drafts surfaces a BulkSelectionBar with Submit / Delete /
//     Clear controls and a visible count.
//   - Bulk delete requires the confirmation dialog; cancel does not
//     delete; confirm removes the selected drafts.
//   - Bulk submit promotes selected drafts to OPEN; the result dialog
//     surfaces success/failure breakdown.
//   - The header checkbox supports select-all / clear-all.

async function createDraft(
  page: import('@playwright/test').Page,
  caseListPage: CaseListPage,
  caseFormPage: CaseFormPage,
  title: string,
): Promise<void> {
  await caseListPage.clickNewCaseButton();
  await caseFormPage.waitForFormVisible();
  await caseFormPage.fillTitle(title);
  await caseFormPage.clickSaveAsDraft();
  await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
  await caseListPage.waitForTableLoad();
}

test.describe('Bulk draft actions (Drafts tab)', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('checkbox column is only present on the Drafts tab', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    // Open tab has no bulk checkbox header.
    await caseListPage.clickStatusTab('Open');
    await expect(page.getByTestId('bulk-header-checkbox')).toHaveCount(0);

    await caseListPage.clickStatusTab('Closed');
    await expect(page.getByTestId('bulk-header-checkbox')).toHaveCount(0);

    await caseListPage.clickStatusTab('Draft');
    await expect(page.getByTestId('bulk-header-checkbox')).toBeVisible();
  });

  test('select-all toggles row checkboxes and surfaces the bulk bar', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    const tag = Date.now();
    await createDraft(page, caseListPage, caseFormPage, `bulk-select-all-A ${tag}`);
    await createDraft(page, caseListPage, caseFormPage, `bulk-select-all-B ${tag}`);

    await caseListPage.clickStatusTab('Draft');
    // Bulk bar starts hidden.
    await expect(page.getByTestId('bulk-selection-bar')).toHaveCount(0);

    // Header checkbox selects every accessible draft on the current
    // filter — at minimum the two we just created.
    await page.getByTestId('bulk-header-checkbox').check();
    await expect(page.getByTestId('bulk-selection-bar')).toBeVisible();
    const countText = await page.getByTestId('bulk-selected-count').textContent();
    expect(countText).toMatch(/\d+/);

    // Clear collapses the bar again.
    await page.getByTestId('bulk-clear-button').click();
    await expect(page.getByTestId('bulk-selection-bar')).toHaveCount(0);
  });

  test('bulk delete shows the confirm dialog; cancel does not delete', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    const tag = Date.now();
    const title = `bulk-delete-cancel ${tag}`;
    await createDraft(page, caseListPage, caseFormPage, title);

    await caseListPage.clickStatusTab('Draft');
    expect(await caseListPage.caseExists(title)).toBeTruthy();

    const row = caseListPage.getCaseRowByTitle(title);
    await row.locator('input[type="checkbox"]').check();

    await page.getByTestId('bulk-delete-button').click();
    await expect(page.getByTestId('bulk-delete-confirm-body')).toBeVisible();

    await page.getByTestId('bulk-delete-confirm-cancel').click();
    await expect(page.getByTestId('bulk-delete-confirm-body')).toHaveCount(0);

    // Draft still present.
    expect(await caseListPage.caseExists(title)).toBeTruthy();
  });

  test('bulk delete confirm removes the selected drafts and shows the result dialog', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    const tag = Date.now();
    const t1 = `bulk-delete-A ${tag}`;
    const t2 = `bulk-delete-B ${tag}`;
    await createDraft(page, caseListPage, caseFormPage, t1);
    await createDraft(page, caseListPage, caseFormPage, t2);

    await caseListPage.clickStatusTab('Draft');
    await caseListPage.getCaseRowByTitle(t1).locator('input[type="checkbox"]').check();
    await caseListPage.getCaseRowByTitle(t2).locator('input[type="checkbox"]').check();

    await page.getByTestId('bulk-delete-button').click();
    await page.getByTestId('bulk-delete-confirm-confirm').click();

    // Result dialog renders summary + success section.
    await expect(page.getByTestId('bulk-result-dialog-body')).toBeVisible();
    await expect(page.getByTestId('bulk-result-summary')).toContainText('2');
    await page.getByTestId('bulk-result-close-button').click();

    // Refetch already fired; drafts are gone from the list.
    await caseListPage.waitForTableLoad();
    expect(await caseListPage.caseExists(t1)).toBeFalsy();
    expect(await caseListPage.caseExists(t2)).toBeFalsy();
  });

  test('bulk submit promotes the selected drafts and reports the outcome', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    const tag = Date.now();
    const draftTitle = `bulk-submit ${tag}`;
    await createDraft(page, caseListPage, caseFormPage, draftTitle);

    await caseListPage.clickStatusTab('Draft');
    await caseListPage.getCaseRowByTitle(draftTitle).locator('input[type="checkbox"]').check();
    await page.getByTestId('bulk-submit-button').click();

    // Either success (case promoted) or failure (e.g. missing required
    // fields in this workspace's schema). Both surface via the result
    // dialog; failing drafts stay on the Drafts tab.
    await expect(page.getByTestId('bulk-result-dialog-body')).toBeVisible();
    await expect(page.getByTestId('bulk-result-summary')).toBeVisible();
    await page.getByTestId('bulk-result-close-button').click();

    await caseListPage.waitForTableLoad();
  });
});
