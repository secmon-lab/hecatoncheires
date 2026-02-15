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
    await caseListPage.fillSearchFilter('E2E Test Case');
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
    await caseListPage.fillSearchFilter('Case for Detail View');
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
    await caseListPage.fillSearchFilter('Case to Edit');
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

  test('should not display Slack channel button when slackChannelID is empty', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case (without Slack configured, slackChannelID will be empty)
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case Without Slack',
      description: 'No Slack channel linked',
      customFields: {
        category: 'bug',
      },
    });

    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Case Without Slack');
    await caseListPage.clickCaseByTitle('Case Without Slack');
    await caseDetailPage.waitForPageLoad();

    // Slack channel button should not be visible
    const isVisible = await caseDetailPage.isSlackChannelButtonVisible();
    expect(isVisible).toBeFalsy();
  });

  test('should display Slack channel button when slackChannelID is present', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case first
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case With Slack',
      description: 'Has Slack channel linked',
      customFields: {
        category: 'bug',
      },
    });

    await caseListPage.waitForTableLoad();

    // Intercept GraphQL response to inject Slack channel data
    await page.route('**/graphql', async (route) => {
      const request = route.request();
      const postData = request.postDataJSON();

      // Only intercept GetCase query
      if (postData?.operationName === 'GetCase') {
        const response = await route.fetch();
        const json = await response.json();

        // Inject Slack channel data into the response
        if (json.data?.case) {
          json.data.case.slackChannelID = 'C1234567890';
          json.data.case.slackChannelName = 'test-channel';
          json.data.case.slackChannelURL = 'https://test-workspace.slack.com/archives/C1234567890';
        }

        await route.fulfill({ response, json });
      } else {
        await route.continue();
      }
    });

    await caseListPage.fillSearchFilter('Case With Slack');
    await caseListPage.clickCaseByTitle('Case With Slack');
    await caseDetailPage.waitForPageLoad();

    // Slack channel button should be visible
    const isVisible = await caseDetailPage.isSlackChannelButtonVisible();
    expect(isVisible).toBeTruthy();

    // Verify button text contains channel name
    const buttonText = await caseDetailPage.getSlackChannelButtonText();
    expect(buttonText).toContain('#test-channel');

    // Verify button href points to Slack
    const href = await caseDetailPage.getSlackChannelButtonHref();
    expect(href).toBe('https://test-workspace.slack.com/archives/C1234567890');
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

  test('should create a case without description', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Click "New Case" button
    await caseListPage.clickNewCaseButton();

    // Create case with title only (no description)
    await caseFormPage.createCase({
      title: 'Case Without Description',
      customFields: {
        category: 'bug',
      },
    });

    // Verify the case appears in the list
    await caseListPage.fillSearchFilter('Case Without Description');
    const exists = await caseListPage.caseExists('Case Without Description');
    expect(exists).toBeTruthy();

    // Verify case detail
    await caseListPage.clickCaseByTitle('Case Without Description');
    const title = await caseDetailPage.getTitle();
    expect(title).toContain('Case Without Description');
  });

  test('should filter cases by search text', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create cases with different titles
    const cases = ['Alpha Issue', 'Beta Bug', 'Gamma Issue'];
    for (const title of cases) {
      await caseListPage.clickNewCaseButton();
      await caseFormPage.createCase({
        title,
        description: `Description for ${title}`,
        customFields: { category: 'task' },
      });
    }

    await page.reload();
    await caseListPage.waitForTableLoad();

    // Search for "Issue"
    await caseListPage.fillSearchFilter('Issue');

    // Verify filtered results
    const alphaExists = await caseListPage.caseExists('Alpha Issue');
    expect(alphaExists).toBeTruthy();
    const gammaExists = await caseListPage.caseExists('Gamma Issue');
    expect(gammaExists).toBeTruthy();
    const betaExists = await caseListPage.caseExists('Beta Bug');
    expect(betaExists).toBeFalsy();

    // Clear filter and verify all cases appear
    await caseListPage.clearSearchFilter();
    for (const title of cases) {
      const exists = await caseListPage.caseExists(title);
      expect(exists).toBeTruthy();
    }
  });

  test('should toggle column visibility', async ({ page }) => {
    const caseListPage = new CaseListPage(page);

    // Verify ID column is visible
    let isVisible = await caseListPage.isColumnVisible('ID');
    expect(isVisible).toBeTruthy();

    // Open column selector and toggle ID off
    await caseListPage.openColumnSelector();
    await caseListPage.toggleColumn('id');

    // Verify ID column is hidden
    isVisible = await caseListPage.isColumnVisible('ID');
    expect(isVisible).toBeFalsy();

    // Toggle ID back on
    await caseListPage.toggleColumn('id');
    isVisible = await caseListPage.isColumnVisible('ID');
    expect(isVisible).toBeTruthy();
  });

  test('should paginate cases', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create 21 cases to trigger pagination
    for (let i = 1; i <= 21; i++) {
      await caseListPage.clickNewCaseButton();
      await caseFormPage.createCase({
        title: `Paginated Case ${i}`,
        customFields: { category: 'task' },
      });
    }

    await page.reload();
    await caseListPage.waitForTableLoad();

    // Filter to only show cases from this test to avoid interference
    await caseListPage.fillSearchFilter('Paginated Case');

    // Verify first page has 20 rows
    const rowCount = await caseListPage.getRowCount();
    expect(rowCount).toBe(20);

    // Verify pagination info
    const paginationInfo = await caseListPage.getPaginationInfo();
    expect(paginationInfo).toContain('1 / 2');

    // Go to next page
    await caseListPage.goToPage('next');
    const page2RowCount = await caseListPage.getRowCount();
    expect(page2RowCount).toBeGreaterThanOrEqual(1);

    // Go back to first page
    await caseListPage.goToPage('prev');
    const backInfo = await caseListPage.getPaginationInfo();
    expect(backInfo).toContain('1 / 2');
  });

  test('should reflect closed case in list after closing', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case To Close',
      description: 'Will be closed',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    // Verify it's in Open tab
    await caseListPage.fillSearchFilter('Case To Close');
    const existsInOpen = await caseListPage.caseExists('Case To Close');
    expect(existsInOpen).toBeTruthy();

    // Open detail and close the case
    await caseListPage.clickCaseByTitle('Case To Close');
    await caseDetailPage.waitForPageLoad();
    await caseDetailPage.clickCloseButton();

    // Go back to list
    await caseDetailPage.clickBack();
    await caseListPage.waitForTableLoad();

    // Verify it's no longer in Open tab
    const existsAfterClose = await caseListPage.caseExists('Case To Close');
    expect(existsAfterClose).toBeFalsy();

    // Switch to Closed tab and verify it's there
    await caseListPage.clickStatusTab('Closed');
    const existsInClosed = await caseListPage.caseExists('Case To Close');
    expect(existsInClosed).toBeTruthy();
  });

  test('should display timestamps on case detail page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case With Timestamps',
      description: 'Testing timestamps display',
      customFields: { category: 'bug' },
    });

    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Case With Timestamps');
    await caseListPage.clickCaseByTitle('Case With Timestamps');
    await caseDetailPage.waitForPageLoad();

    // Verify timestamps are visible
    const timestampsVisible = await caseDetailPage.isTimestampsVisible();
    expect(timestampsVisible).toBeTruthy();

    // Verify Created timestamp is non-empty
    const createdTimestamp = await caseDetailPage.getCreatedTimestamp();
    expect(createdTimestamp.length).toBeGreaterThan(0);

    // Verify Updated timestamp is non-empty
    const updatedTimestamp = await caseDetailPage.getUpdatedTimestamp();
    expect(updatedTimestamp.length).toBeGreaterThan(0);
  });

  test('should display Fields section on case detail page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case with custom fields
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case With Fields',
      description: 'Testing fields display',
      customFields: { category: 'bug' },
    });

    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Case With Fields');
    await caseListPage.clickCaseByTitle('Case With Fields');
    await caseDetailPage.waitForPageLoad();

    // Verify Fields section is visible
    const fieldsVisible = await caseDetailPage.isFieldsSectionVisible();
    expect(fieldsVisible).toBeTruthy();
  });

  test('should display empty action state for new case', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case (no actions)
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case With No Actions',
      description: 'Testing empty action state',
      customFields: { category: 'task' },
    });

    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Case With No Actions');
    await caseListPage.clickCaseByTitle('Case With No Actions');
    await caseDetailPage.waitForPageLoad();

    // Verify empty action state is visible
    const emptyStateVisible = await caseDetailPage.isEmptyActionStateVisible();
    expect(emptyStateVisible).toBeTruthy();
  });

  test('should show status badge on case detail page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case Status Badge',
      description: 'Testing status badge',
      customFields: { category: 'task' },
    });

    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Case Status Badge');
    await caseListPage.clickCaseByTitle('Case Status Badge');
    await caseDetailPage.waitForPageLoad();

    // Verify status badge shows "Open"
    const status = await caseDetailPage.getStatus();
    expect(status).toBe('Open');
  });

  test('should reflect reopened case in list after reopening', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create and close a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Case To Reopen',
      description: 'Will be reopened',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    // Filter to find the case reliably in case of pagination
    await caseListPage.fillSearchFilter('Case To Reopen');
    await caseListPage.clickCaseByTitle('Case To Reopen');
    await caseDetailPage.waitForPageLoad();
    await caseDetailPage.clickCloseButton();
    await caseDetailPage.clickBack();
    await caseListPage.waitForTableLoad();

    // Switch to Closed tab
    await caseListPage.clearSearchFilter();
    await caseListPage.clickStatusTab('Closed');
    const existsInClosed = await caseListPage.caseExists('Case To Reopen');
    expect(existsInClosed).toBeTruthy();

    // Reopen the case
    await caseListPage.clickCaseByTitle('Case To Reopen');
    await caseDetailPage.waitForPageLoad();
    await caseDetailPage.clickReopenButton();

    // Go back to list
    await caseDetailPage.clickBack();
    await caseListPage.waitForTableLoad();

    // Verify it's no longer in Closed tab
    await caseListPage.clickStatusTab('Closed');
    const existsAfterReopen = await caseListPage.caseExists('Case To Reopen');
    expect(existsAfterReopen).toBeFalsy();

    // Switch to Open tab and verify it's there
    await caseListPage.clickStatusTab('Open');
    await caseListPage.fillSearchFilter('Case To Reopen');
    const existsInOpen = await caseListPage.caseExists('Case To Reopen');
    expect(existsInOpen).toBeTruthy();
  });
});
