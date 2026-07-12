import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { CaseAgentPage } from '../pages/CaseAgentPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for the Case Agent settings page (previously untested end to
// end). Drives the "additional prompt" editor: edit → save → reload, asserting
// the value round-trips to the backend.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Case agent settings', () => {
  test('edits the additional prompt and persists it across reload', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const agentPage = new CaseAgentPage(page);

    const title = `Agent Case ${uniq()}`;
    const promptText = `Investigate the payment webhook first (${uniq()})`;

    // Create a case and capture its id from the detail URL.
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title, customFields: { category: 'task' } });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    await caseListPage.clickCaseByTitle(title);
    await caseDetailPage.waitForPageLoad();

    const match = page.url().match(/\/cases\/(\d+)/);
    expect(match).not.toBeNull();
    const caseId = match![1];

    // Open the agent settings page and edit the additional prompt.
    await agentPage.navigate(TEST_WORKSPACE_ID, caseId);
    expect(await agentPage.isPageLoaded()).toBeTruthy();
    await agentPage.editPrompt(promptText);

    // The saved prompt renders in the read-only view.
    await expect(agentPage.promptDisplay()).toContainText(promptText);

    // Reload proves it round-tripped to the backend.
    await page.reload();
    expect(await agentPage.isPageLoaded()).toBeTruthy();
    await expect(agentPage.promptDisplay()).toContainText(promptText);
  });
});
