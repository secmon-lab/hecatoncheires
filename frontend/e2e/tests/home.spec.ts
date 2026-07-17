import { test, expect } from '@playwright/test';
import { HomePage } from '../pages/HomePage';

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
  });

  test('clicking a workspace card navigates into that workspace', async ({ page }) => {
    const home = new HomePage(page);
    await home.navigate();
    await home.waitForWorkspaceChooserLoad();

    await home.openWorkspace(TEST_WORKSPACE_ID);
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}/cases`);
  });
});
