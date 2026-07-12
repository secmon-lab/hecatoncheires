import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for CaseDeleteDialog — the type-to-confirm safety gate that
// guards case deletion. Previously untested. Verifies the Delete button stays
// disabled until the exact title is retyped, then that confirming actually
// removes the case.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Case delete (type-to-confirm)', () => {
  test('Delete is gated on retyping the exact title, then removes the case', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const title = `Delete Me ${uniq()}`;

    // Create the case and open its detail page.
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title, customFields: { category: 'bug' } });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    await caseListPage.clickCaseByTitle(title);
    await caseDetailPage.waitForPageLoad();

    // Open the delete dialog (kebab → Delete).
    await caseDetailPage.clickDelete();

    const confirmInput = page.getByTestId('delete-case-confirm-input');
    const confirmButton = page.getByTestId('confirm-delete-case-button');
    await expect(confirmInput).toBeVisible();

    // Gate: disabled with no text, disabled with a wrong title…
    await expect(confirmButton).toBeDisabled();
    await confirmInput.fill('not the title');
    await expect(confirmButton).toBeDisabled();

    // …enabled only once the exact title is retyped.
    await confirmInput.fill(title);
    await expect(confirmButton).toBeEnabled();

    // Confirm: the app returns to the case list and the case is gone.
    await confirmButton.click();
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases(\\?|$)`));
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    expect(await caseListPage.caseExists(title)).toBeFalsy();
  });
});
