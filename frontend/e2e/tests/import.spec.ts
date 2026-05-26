import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { ImportPage } from '../pages/ImportPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// End-to-end coverage for the YAML Case Import flow.
//
// What this spec validates:
//   - CaseList → [Import] button navigates to /imports/new.
//   - Dropzone accepts a YAML file and the page redirects to
//     /imports/:sessionId with a PENDING status badge.
//   - The preview lists every Case parsed from the YAML, the Execute
//     button is enabled (no validation errors), and the JSON Schema
//     toggle exposes a copyable JSON Schema view.
//   - Clicking Execute transitions the session to APPLIED.
//   - The "Open cases list" button returns to the Case list, where the
//     imported cases now appear under the Drafts tab.

// NOTE: no assigneeIDs / USER-typed fields here. The preview validates
// Slack-user references against the workspace's SlackUser registry and
// the E2E test workspace is not pre-seeded with users, so any reference
// would surface as "unknown Slack user" and gate Execute off.
const VALID_YAML = `version: 1
cases:
  - title: "__E2E__ Suspicious login"
    description: "Multiple failed attempts from 10.0.0.1."
    isPrivate: false
    actions:
      - title: "Block source IP"
        description: "Add firewall rule"
      - title: "Notify SOC"
  - title: "__E2E__ Failed deployment"
    description: "Canary deploy failed."
    actions:
      - title: "Roll back to v2.2"
`;

const INVALID_YAML = `version: 1
cases:
  - title: ""
    description: "missing title"
    actions:
      - title: "should not run"
`;

test.describe('Case Import (YAML)', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('upload YAML → preview → execute → drafts visible in Cases list', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const importPage = new ImportPage(page);

    // 1. CaseList → [Import] (the button sits to the left of [New Case]).
    await page.getByRole('button', { name: /^Import$/ }).click();
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/imports/new$`));

    // 2. While on /imports/new, the schema panel exists. Verify the
    //    JSON Schema toggle reveals a copyable JSON Schema. Do this
    //    BEFORE uploading because once the upload completes the page
    //    navigates to /imports/:id and the schema panel disappears.
    await importPage.copyJsonSchema();
    await expect(page.locator('main details pre').first()).toContainText(
      '$schema',
    );

    // 3. Upload the YAML and confirm we landed on the detail page.
    const sessionId = await importPage.uploadYaml(VALID_YAML);
    expect(sessionId).toMatch(/^[0-9a-f-]{8,}$/i);
    await expect(page).toHaveURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/imports/${sessionId}$`));
    expect(await importPage.readStatus()).toBe('PENDING');

    // 4. The preview should show both imported cases by title.
    await expect(page.getByText('__E2E__ Suspicious login').first()).toBeVisible();
    await expect(page.getByText('__E2E__ Failed deployment').first()).toBeVisible();

    // 5. Execute → status moves to APPLIED.
    expect(await importPage.isExecuteEnabled()).toBeTruthy();
    await importPage.clickExecute();
    await importPage.waitForStatus('APPLIED');

    // 6. Open Cases list → Drafts tab carries both imported titles.
    await importPage.clickOpenCasesList();
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    expect(await caseListPage.caseExists('__E2E__ Suspicious login')).toBeTruthy();
    expect(await caseListPage.caseExists('__E2E__ Failed deployment')).toBeTruthy();
  });

  test('invalid YAML keeps Execute disabled and creates no drafts', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const importPage = new ImportPage(page);

    // Capture the current Drafts count so we can assert it does not move.
    await caseListPage.clickStatusTab('Draft');
    const beforeCount = await caseListPage.getDraftsTabCount();

    await page.getByRole('button', { name: /^Import$/ }).click();
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/imports/new$`));

    await importPage.uploadYaml(INVALID_YAML, 'bad.yaml');
    expect(await importPage.readStatus()).toBe('PENDING');

    // Execute must be disabled because the session is not valid.
    expect(await importPage.isExecuteEnabled()).toBeFalsy();

    // Return to the Case list — drafts count should be unchanged.
    await page.goto(`/ws/${TEST_WORKSPACE_ID}/cases`);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    const afterCount = await caseListPage.getDraftsTabCount();
    expect(afterCount).toBe(beforeCount);
  });
});
