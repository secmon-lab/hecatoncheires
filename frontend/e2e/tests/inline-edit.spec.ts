import { test, expect, type Page } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// Asserts that no GraphQL response since `start()` carried an `errors` payload.
// The previous bug shape was a 200 OK with `errors:[{message:"... validation
// failed ..."}]`, which a quick visual smoke test would miss.
function graphqlErrorWatcher(page: Page) {
  const failures: string[] = [];
  const handler = async (response: import('@playwright/test').Response) => {
    if (!response.url().includes('/graphql')) return;
    if (response.request().method() !== 'POST') return;
    try {
      const body = await response.json();
      if (body && Array.isArray(body.errors) && body.errors.length > 0) {
        failures.push(body.errors.map((e: any) => e.message).join(' | '));
      }
    } catch {
      // Non-JSON or stream already consumed — ignore.
    }
  };
  page.on('response', handler);
  return {
    failures,
    stop: () => page.off('response', handler),
  };
}

// The shared E2E config (config.test.toml) exposes:
//   - SELECT (category, priority)
//   - TEXT   (description)
// We exercise inline-edit on each of these. NUMBER / DATE / URL /
// MULTI_SELECT / USER / MULTI_USER are validated by Go-level tests
// (validateNumber, etc.) plus the live "risk" config; covering them here
// would require a wider field set in config.test.toml, which makes the
// New-Case modal taller than the viewport and breaks unrelated tests.
test.describe('Inline edit — covered field types', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('saves SELECT and TEXT fields without GraphQL errors', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Seed a case so we can edit its custom fields inline.
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Inline edit field coverage',
      description: 'Used by inline-edit.spec',
      customFields: { category: 'bug' },
    });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Inline edit field coverage');
    await caseListPage.clickCaseByTitle('Inline edit field coverage');
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();

    const watcher = graphqlErrorWatcher(page);

    // SELECT — change category from bug → feature.
    await page.getByTestId('field-category').click();
    await page.getByTestId('field-category-option-feature').click();
    await page.waitForResponse((r) => r.url().includes('/graphql') && r.request().method() === 'POST');

    // SELECT (newly set) — priority high.
    await page.getByTestId('field-priority').click();
    await page.getByTestId('field-priority-option-high').click();
    await page.waitForResponse((r) => r.url().includes('/graphql') && r.request().method() === 'POST');

    // TEXT — custom-field "description".
    await page.getByTestId('field-description').click();
    const textInput = page.getByTestId('field-description-input');
    await textInput.fill('hello world');
    await textInput.press('Enter');
    await page.waitForResponse((r) => r.url().includes('/graphql') && r.request().method() === 'POST');

    watcher.stop();
    expect(watcher.failures, `GraphQL responses returned errors: ${watcher.failures.join('\n')}`).toEqual([]);

    // Persist check: reload and verify values are still present.
    await page.reload();
    await caseDetailPage.waitForPageLoad();
    await expect(page.getByTestId('field-category')).toContainText('Feature');
    await expect(page.getByTestId('field-priority')).toContainText('High');
    await expect(page.getByTestId('field-description')).toContainText('hello world');
  });
});
