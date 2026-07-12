import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Private Case Mode', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  // Real end-to-end access control: no page.route mocking. A UI-created
  // private case has no synced Slack channel members in the e2e environment,
  // so its ChannelUserIDs is empty and the no-auth user is not a member — the
  // backend genuinely restricts it. This is the one test that drives the
  // actual access-control code path; the mocked tests below only verify the
  // frontend's rendering of fabricated access-denied / private payloads.
  test('a UI-created private case is genuinely access-restricted (no mocking)', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: `Real Private ${Date.now()}`,
      description: 'genuinely private, no channel members synced',
      customFields: { category: 'bug' },
      isPrivate: true,
    });
    await caseListPage.waitForTableLoad();

    // The list renders it as an access-denied "Private" row (the title is
    // withheld by the backend's RestrictCase, so we key off the label).
    const label = page.getByTestId('access-denied-label').first();
    await expect(label).toBeVisible();
    await expect(label).toHaveText('Private');
    await expect(page.getByTestId('private-lock-icon').first()).toBeVisible();

    // Clicking the restricted row must not open a detail page.
    const urlBefore = page.url();
    await label.click();
    const navigated = await page
      .waitForURL(/\/cases\/\d+/, { timeout: 1500 })
      .then(() => true)
      .catch(() => false);
    expect(navigated).toBeFalsy();
    expect(page.url()).toBe(urlBefore);
  });

  test('should display "Private" label for access-denied cases in case list', async ({
    page,
  }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create a case first
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Private Access Test',
      description: 'Testing access denied display',
      customFields: { category: 'bug' },
    });
    await caseListPage.waitForTableLoad();

    // Intercept GetCases to simulate a private, access-denied case
    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();
      if (postData?.operationName === 'GetCases') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.cases && json.data.cases.length > 0) {
          // Make the first case private and access denied
          json.data.cases[0].isPrivate = true;
          json.data.cases[0].accessDenied = true;
        }

        await route.fulfill({ response, json });
      } else {
        await route.continue();
      }
    });

    // Reload to trigger the intercepted response
    await page.reload();
    await caseListPage.waitForTableLoad();

    // Verify "Private" label is displayed
    const label = page.getByTestId('access-denied-label').first();
    await expect(label).toBeVisible();
    await expect(label).toHaveText('Private');

    // Verify lock icon is visible
    const lock = page.getByTestId('private-lock-icon').first();
    await expect(lock).toBeVisible();
  });

  test('should prevent navigation when clicking access-denied case row', async ({
    page,
  }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Non-Clickable Private Case',
      description: 'Should not navigate',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    // Intercept to make it access denied
    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();
      if (postData?.operationName === 'GetCases') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.cases) {
          for (const c of json.data.cases) {
            c.isPrivate = true;
            c.accessDenied = true;
          }
        }
        await route.fulfill({ response, json });
      } else {
        await route.continue();
      }
    });

    await page.reload();
    await caseListPage.waitForTableLoad();

    // Get current URL
    const urlBefore = page.url();

    // Try clicking the access-denied row
    const row = page.getByTestId('access-denied-label').first();
    await row.click();

    // Prove no navigation to a case-detail URL happens: wait for one and
    // expect the wait to time out. Resolves early (and fails) if the row
    // wrongly navigates — unlike a blind fixed sleep.
    const navigated = await page
      .waitForURL(/\/cases\/\d+/, { timeout: 1500 })
      .then(() => true)
      .catch(() => false);
    expect(navigated).toBeFalsy();
    expect(page.url()).toBe(urlBefore);
  });

  test('should display private badge on case detail page', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Private Badge Detail Test',
      description: 'Testing private badge on detail',
      customFields: { category: 'bug' },
    });
    await caseListPage.waitForTableLoad();

    // Intercept GetCase to mark as private (but accessible)
    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();
      if (postData?.operationName === 'GetCase') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.case) {
          json.data.case.isPrivate = true;
          json.data.case.accessDenied = false;
        }
        await route.fulfill({ response, json });
      } else {
        await route.continue();
      }
    });

    await caseListPage.fillSearchFilter('Private Badge Detail Test');
    await caseListPage.clickCaseByTitle('Private Badge Detail Test');
    await caseDetailPage.waitForPageLoad();

    // Verify Private badge is visible
    const isVisible = await caseDetailPage.isPrivateBadgeVisible();
    expect(isVisible).toBeTruthy();

    const badgeText = await caseDetailPage.getPrivateBadgeText();
    expect(badgeText).toContain('Private');
  });

  test('should display channel members section with member count', async ({
    page,
  }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Channel Members Test',
      description: 'Testing channel members section',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    // Intercept to inject channel members data
    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();

      if (postData?.operationName === 'GetCase') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.case) {
          json.data.case.isPrivate = true;
          json.data.case.channelUserCount = 3;
          json.data.case.slackChannelID = 'C-TEST-MEMBERS';
        }
        await route.fulfill({ response, json });
      } else if (postData?.operationName === 'GetCaseMembers') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              case: {
                channelUserCount: 3,
                channelUsers: {
                  items: [
                    {
                      id: 'U001',
                      name: 'alice',
                      realName: 'Alice Smith',
                      imageUrl: null,
                    },
                    {
                      id: 'U002',
                      name: 'bob',
                      realName: 'Bob Jones',
                      imageUrl: null,
                    },
                    {
                      id: 'U003',
                      name: 'charlie',
                      realName: 'Charlie Brown',
                      imageUrl: null,
                    },
                  ],
                  totalCount: 3,
                  hasMore: false,
                },
              },
            },
          }),
        });
      } else {
        await route.continue();
      }
    });

    await caseListPage.fillSearchFilter('Channel Members Test');
    await caseListPage.clickCaseByTitle('Channel Members Test');
    await caseDetailPage.waitForPageLoad();

    // Verify channel members section is visible
    const isVisible = await caseDetailPage.isChannelMembersSectionVisible();
    expect(isVisible).toBeTruthy();

    // Verify section title includes member count
    const sectionTitle = await caseDetailPage.getChannelMembersSectionTitle();
    expect(sectionTitle).toContain('Channel Members');
    expect(sectionTitle).toContain('3');

    // Verify sync button is visible
    const syncVisible = await caseDetailPage.isSyncButtonVisible();
    expect(syncVisible).toBeTruthy();

    // Verify members are rendered
    const memberCount = await caseDetailPage.getMemberCount();
    expect(memberCount).toBe(3);
  });

  test('should sync channel members when clicking sync button', async ({
    page,
  }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Sync Members Test',
      description: 'Testing sync functionality',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    let syncCalled = false;

    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();

      if (postData?.operationName === 'GetCase') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.case) {
          json.data.case.isPrivate = true;
          json.data.case.channelUserCount = 2;
          json.data.case.slackChannelID = 'C-SYNC-TEST';
        }
        await route.fulfill({ response, json });
      } else if (postData?.operationName === 'GetCaseMembers') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              case: {
                channelUserCount: 2,
                channelUsers: {
                  items: [
                    {
                      id: 'U001',
                      name: 'alice',
                      realName: 'Alice Smith',
                      imageUrl: null,
                    },
                    {
                      id: 'U002',
                      name: 'bob',
                      realName: 'Bob Jones',
                      imageUrl: null,
                    },
                  ],
                  totalCount: 2,
                  hasMore: false,
                },
              },
            },
          }),
        });
      } else if (postData?.operationName === 'SyncCaseChannelUsers') {
        syncCalled = true;
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              syncCaseChannelUsers: {
                id: postData.variables.id,
                channelUserCount: 2,
                __typename: 'Case',
              },
            },
          }),
        });
      } else {
        await route.continue();
      }
    });

    await caseListPage.fillSearchFilter('Sync Members Test');
    await caseListPage.clickCaseByTitle('Sync Members Test');
    await caseDetailPage.waitForPageLoad();

    // Click sync button
    await caseDetailPage.clickSyncButton();

    // Verify sync mutation was called
    expect(syncCalled).toBeTruthy();
  });

  test('should filter channel members by name', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Filter Members Test',
      description: 'Testing member filter',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    await page.route('**/graphql', async (route) => {
      const postData = route.request().postDataJSON();

      if (postData?.operationName === 'GetCase') {
        const response = await route.fetch();
        const json = await response.json();

        if (json.data?.case) {
          json.data.case.isPrivate = true;
          json.data.case.channelUserCount = 3;
          json.data.case.slackChannelID = 'C-FILTER-TEST';
        }
        await route.fulfill({ response, json });
      } else if (postData?.operationName === 'GetCaseMembers') {
        const filter = postData?.variables?.filter || '';
        const allMembers = [
          {
            id: 'U001',
            name: 'alice',
            realName: 'Alice Smith',
            imageUrl: null,
          },
          {
            id: 'U002',
            name: 'bob',
            realName: 'Bob Jones',
            imageUrl: null,
          },
          {
            id: 'U003',
            name: 'charlie',
            realName: 'Charlie Brown',
            imageUrl: null,
          },
        ];

        const filtered = filter
          ? allMembers.filter(
              (m) =>
                m.name.includes(filter.toLowerCase()) ||
                m.realName.toLowerCase().includes(filter.toLowerCase()),
            )
          : allMembers;

        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              case: {
                channelUserCount: 3,
                channelUsers: {
                  items: filtered,
                  totalCount: filtered.length,
                  hasMore: false,
                },
              },
            },
          }),
        });
      } else {
        await route.continue();
      }
    });

    await caseListPage.fillSearchFilter('Filter Members Test');
    await caseListPage.clickCaseByTitle('Filter Members Test');
    await caseDetailPage.waitForPageLoad();

    // Verify all 3 members initially
    await expect(caseDetailPage.memberItems()).toHaveCount(3);

    // Filter by "alice" — the debounced re-render settles via auto-retry,
    // no fixed sleep needed.
    await caseDetailPage.filterMembers('Alice');
    await expect(caseDetailPage.memberItems()).toHaveCount(1);
  });

  test('should not show channel members section when count is 0', async ({
    page,
  }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Create a case (no channel members by default)
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'No Members Case',
      description: 'No channel members',
      customFields: { category: 'task' },
    });
    await caseListPage.waitForTableLoad();

    await caseListPage.fillSearchFilter('No Members Case');
    await caseListPage.clickCaseByTitle('No Members Case');
    await caseDetailPage.waitForPageLoad();

    // Channel members section should NOT be visible
    const isVisible = await caseDetailPage.isChannelMembersSectionVisible();
    expect(isVisible).toBeFalsy();
  });
});
