import { test, expect } from '@playwright/test';
import { HomePage } from '../pages/HomePage';
import { CaseListPage } from '../pages/CaseListPage';
import { WorkspaceSwitcherPage } from '../pages/WorkspaceSwitcherPage';

// The second workspace loaded by config.review.test.toml (see e2e.sh),
// alongside the primary `test` workspace — see workspace.spec.ts for the
// same pairing.
const REVIEW_WORKSPACE_ID = 'review';
const TEST_WORKSPACE_ID = 'test';

test.describe('Home', () => {
  test('root path renders the Home page with a workspace chooser', async ({ page }) => {
    const home = new HomePage(page);
    await home.navigate();
    expect(await home.isPageLoaded()).toBeTruthy();
    await home.waitForWorkspaceChooserLoad();
  });

  test('clicking a workspace card navigates into that workspace', async ({ page }) => {
    const home = new HomePage(page);
    await home.navigate();
    await home.waitForWorkspaceChooserLoad();

    await home.openWorkspace(TEST_WORKSPACE_ID);
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}/cases`);
  });
});

// The favorite set is per-user state on the shared E2E backend (one
// --no-auth user for the whole run), so these tests mutate state any other
// spec can observe. `serial` only fixes the order *within this describe* — it
// does not isolate them from other spec files running in parallel. Each test
// therefore restores the favorite it set, so the shared state is unchanged
// once the block finishes.
test.describe.serial('Workspace favorites', () => {
  test('favoriting a workspace moves it to the front of the chooser, and it survives a reload', async ({ page }) => {
    const home = new HomePage(page);
    await home.navigate();
    await home.waitForWorkspaceChooserLoad();

    const initialOrder = await home.workspaceCardOrder();
    expect(initialOrder).toContain(TEST_WORKSPACE_ID);
    expect(initialOrder).toContain(REVIEW_WORKSPACE_ID);

    // Favorite whichever of the two workspaces isn't already first, so the
    // assertion below is meaningful regardless of the initial ordering.
    const toFavorite = initialOrder[0] === REVIEW_WORKSPACE_ID ? TEST_WORKSPACE_ID : REVIEW_WORKSPACE_ID;
    await home.toggleFavorite(toFavorite);

    await expect.poll(() => home.isFavorite(toFavorite)).toBeTruthy();
    await expect.poll(() => home.workspaceCardOrder()).toEqual([
      toFavorite,
      ...initialOrder.filter((id) => id !== toFavorite),
    ]);

    // The favorite set is server-persisted (setFavoriteWorkspaces), not just
    // local component state, so it must still be applied after a reload.
    await page.reload();
    await home.waitForWorkspaceChooserLoad();
    expect(await home.isFavorite(toFavorite)).toBeTruthy();
    expect((await home.workspaceCardOrder())[0]).toBe(toFavorite);

    // Restore the favorite set for the specs that share this backend.
    await home.toggleFavorite(toFavorite);
    await expect.poll(() => home.isFavorite(toFavorite)).toBeFalsy();
  });

  test('a favorited workspace stays in the switcher even when it would be collapsed', async ({ page }) => {
    const home = new HomePage(page);
    const caseListPage = new CaseListPage(page);
    const switcher = new WorkspaceSwitcherPage(page);

    // Find a workspace the switcher currently hides behind the more toggle.
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await switcher.open();
    const collapsed = await switcher.visibleWorkspaceIds();
    await switcher.expand();
    const hiddenId = (await switcher.visibleWorkspaceIds()).find((id) => !collapsed.includes(id));
    expect(hiddenId).toBeTruthy();

    await home.navigate();
    await home.waitForWorkspaceChooserLoad();
    await home.toggleFavorite(hiddenId!);
    await expect.poll(() => home.isFavorite(hiddenId!)).toBeTruthy();

    // Favoriting lifts it out of the collapsed remainder: it is now rendered
    // without expanding, marked with a star, and ahead of every non-favorite.
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await switcher.open();
    const visible = await switcher.visibleWorkspaceIds();
    expect(visible).toContain(hiddenId);
    expect(await switcher.isStarred(hiddenId!)).toBeTruthy();

    const firstUnstarred = await switcher.firstUnstarredIndex();
    expect(visible.indexOf(hiddenId!)).toBeLessThan(firstUnstarred);

    // Leave the favorite set as this suite found it so later runs against a
    // long-lived backend start from the same state.
    await home.navigate();
    await home.waitForWorkspaceChooserLoad();
    await home.toggleFavorite(hiddenId!);
    await expect.poll(() => home.isFavorite(hiddenId!)).toBeFalsy();
  });
});
