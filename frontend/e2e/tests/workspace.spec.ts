import { test, expect } from '@playwright/test';
import { DashboardPage } from '../pages/DashboardPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { HomePage } from '../pages/HomePage';
import { WorkspaceSwitcherPage } from '../pages/WorkspaceSwitcherPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// The second workspace loaded by config.review.test.toml (see e2e.sh).
// Used here to prove per-workspace data isolation.
const REVIEW_WORKSPACE_ID = 'review';

test.describe('Workspace', () => {
  test('should display correct workspace context', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);

    // Navigate to test workspace
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Verify we're in the correct workspace by checking URL
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}`);

    // Verify dashboard is visible
    const isVisible = await dashboardPage.isDashboardVisible();
    expect(isVisible).toBeTruthy();
  });

  test('should maintain workspace context when navigating', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);
    const caseListPage = new CaseListPage(page);

    // Start from dashboard
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Navigate to cases
    await dashboardPage.goToCases();

    // Verify workspace is maintained in URL
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}/cases`);

    // Verify case list loaded correctly
    const isLoaded = await caseListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('keeps case data isolated between workspaces', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create a uniquely-titled case in the `test` workspace.
    const title = `Isolation Case ${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title,
      description: 'Belongs to the test workspace only',
      customFields: { category: 'task' },
    });

    // It is visible in the `test` workspace it was created in.
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    expect(await caseListPage.caseExists(title)).toBeTruthy();

    // It must NOT leak into the `review` workspace — data is scoped per
    // workspace on the backend, and this is the real guarantee worth testing
    // (the old spec only re-checked the URL).
    await caseListPage.navigate(REVIEW_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    expect(await caseListPage.caseExists(title)).toBeFalsy();
  });
});

// The backend for these tests loads extra-workspaces/ (see e2e.sh), which puts
// the workspace count above the switcher's 10-row display limit.
// Favorite-mutating cases live in home.spec.ts instead: the favorite set is
// per-user server state shared by every spec, so they must not run in parallel
// with each other.
test.describe('Workspace switcher', () => {
  test('shows ten entries and reveals the rest through the more toggle', async ({ page }) => {
    const home = new HomePage(page);
    const caseListPage = new CaseListPage(page);
    const switcher = new WorkspaceSwitcherPage(page);

    // The Home chooser lists every workspace, so it is the reference set the
    // expanded switcher is compared against.
    await home.navigate();
    await home.waitForWorkspaceChooserLoad();
    const allWorkspaceIds = await home.workspaceCardOrder();
    expect(allWorkspaceIds.length).toBeGreaterThan(10);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await switcher.open();

    const collapsed = await switcher.visibleWorkspaceIds();
    expect(collapsed).toHaveLength(10);
    expect(await switcher.hasMoreToggle()).toBeTruthy();

    await switcher.expand();
    const expanded = await switcher.visibleWorkspaceIds();
    expect([...expanded].sort()).toEqual([...allWorkspaceIds].sort());
  });

  test('announces its state to assistive tech and closes on Escape', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const switcher = new WorkspaceSwitcherPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    expect(await switcher.triggerAriaExpanded()).toBe('false');

    await switcher.open();
    expect(await switcher.triggerAriaExpanded()).toBe('true');
    expect(await switcher.triggerAriaControls()).toBe(await switcher.menuElementId());

    // Escape must work from inside the dropdown, not only while the trigger
    // still holds focus, and it must hand focus back to the trigger.
    await switcher.focusFirstEntry();
    expect(await switcher.isTriggerFocused()).toBeFalsy();

    await switcher.pressEscape();
    expect(await switcher.isOpen()).toBeFalsy();
    expect(await switcher.isTriggerFocused()).toBeTruthy();
    expect(await switcher.triggerAriaExpanded()).toBe('false');
  });

  test('switches workspace from an entry that was hidden behind the more toggle', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const switcher = new WorkspaceSwitcherPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await switcher.open();
    const collapsed = await switcher.visibleWorkspaceIds();

    await switcher.expand();
    const expanded = await switcher.visibleWorkspaceIds();
    const revealedId = expanded.find((id) => !collapsed.includes(id));
    expect(revealedId).toBeTruthy();

    await switcher.selectWorkspace(revealedId!);
    expect(page.url()).toContain(`/ws/${revealedId}/`);
    await caseListPage.waitForTableLoad();
  });
});
