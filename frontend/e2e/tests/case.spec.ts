import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Case Management', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to case list page
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('should display case list page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);

    // Verify the page is loaded
    const isLoaded = await caseListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('should create a new case', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Click "New Case" button
    await caseListPage.clickNewCaseButton();

    // Fill in the form
    await caseFormPage.createCase({
      title: 'E2E Test Case',
      description: 'This is a test case created by Playwright',
      customFields: {
        category: 'bug',
      },
    });

    // Verify the case appears in the list
    const exists = await caseListPage.caseExists('E2E Test Case');
    expect(exists).toBeTruthy();
  });

  test('should view case details', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a test case first
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case for Detail View',
      description: 'Testing case detail view',
      customFields: {
        category: 'bug',
      },
    });

    // Wait for table to update
    await caseListPage.waitForTableLoad();

    // Click on the case to view details
    await caseListPage.clickCaseByTitle('Case for Detail View');

    // Verify detail page is loaded
    const isLoaded = await caseDetailPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();

    // Verify the title matches
    const title = await caseDetailPage.getTitle();
    expect(title).toContain('Case for Detail View');
  });

  test('should edit a case', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a test case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case to Edit',
      description: 'Original description',
      customFields: {
        category: 'feature',
      },
    });

    // Wait for table to update
    await caseListPage.waitForTableLoad();

    // Open the case detail
    await caseListPage.clickCaseByTitle('Case to Edit');

    // Click edit button
    await caseDetailPage.clickEdit();

    // Update the case
    await caseFormPage.fillDescription('Updated description');
    await caseFormPage.submit();

    // Verify the description was updated
    const description = await caseDetailPage.getDescription();
    expect(description).toContain('Updated description');
  });

  test('should list multiple cases', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create multiple cases
    const caseCount = 3;
    for (let i = 1; i <= caseCount; i++) {
      await caseListPage.clickNewCaseButton();
      await caseFormPage.createCase({
        title: `Case ${i}`,
        description: `Description for case ${i}`,
        customFields: {
          category: 'task',
        },
      });
    }

    // Reload to get fresh data
    await page.reload();
    await caseListPage.waitForTableLoad();

    // Verify all cases are listed
    for (let i = 1; i <= caseCount; i++) {
      const exists = await caseListPage.caseExists(`Case ${i}`);
      expect(exists).toBeTruthy();
    }
  });
});
