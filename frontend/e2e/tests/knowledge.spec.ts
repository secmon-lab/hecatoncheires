import { test, expect } from '@playwright/test';
import { KnowledgeListPage } from '../pages/KnowledgeListPage';
import { KnowledgeFormPage } from '../pages/KnowledgeFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// Unique suffix per run so entries/tags do not collide with other workers or
// prior runs against the shared in-memory workspace.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Knowledge & Tag Management', () => {
  test('creates a knowledge entry with a newly created tag and lists it', async ({ page }) => {
    const list = new KnowledgeListPage(page);
    const form = new KnowledgeFormPage(page);
    const id = uniq();
    const title = `KB Alpha ${id}`;
    const tag = `ops-${id}`;

    await list.navigate(TEST_WORKSPACE_ID);
    expect(await list.isPageLoaded()).toBeTruthy();
    await list.clickAddKnowledge();

    // Fill the form and create a brand-new tag inline (create-then-reference).
    // The claim body carries a GFM table so the Preview tab exercises remark-gfm.
    await form.fillTitle(title);
    await form.fillClaim(
      '## Rule\n- pin GitHub actions by SHA\n\n| Env | Pinned |\n| --- | --- |\n| prod | yes |',
    );
    await form.addNewTag(tag);
    expect(await form.hasSelectedTag(tag)).toBeTruthy();

    // Saving a new entry navigates to its persisted detail URL.
    await form.saveNewAndWait();
    expect(await form.getTitleValue()).toBe(title);

    // Switching the claim to Preview renders the GFM table as a real <table>.
    await page.getByTestId('knowledge-claim-tab-preview').click();
    const claimPreview = page.getByTestId('knowledge-claim-preview');
    await expect(claimPreview.locator('table')).toBeVisible();
    await expect(claimPreview.locator('tbody td')).toHaveText(['prod', 'yes']);
    await expect(claimPreview).not.toContainText('| Env |');
    await page.getByTestId('knowledge-claim-tab-write').click();

    // It shows up in the list, carrying the tag.
    await list.navigate(TEST_WORKSPACE_ID);
    await list.search(title);
    expect(await list.cardExists(title)).toBeTruthy();
    expect(await list.cardHasTag(title, tag)).toBeTruthy();
  });

  test('reuses an existing tag across entries and filters the list by tag', async ({ page }) => {
    const list = new KnowledgeListPage(page);
    const form = new KnowledgeFormPage(page);
    const id = uniq();
    const shared = `shared-${id}`;
    const other = `other-${id}`;
    const titleA = `KB Shared A ${id}`;
    const titleB = `KB Shared B ${id}`;
    const titleC = `KB Other C ${id}`;

    // Entry A — creates the shared tag.
    await list.navigate(TEST_WORKSPACE_ID);
    await list.clickAddKnowledge();
    await form.fillTitle(titleA);
    await form.addNewTag(shared);
    await form.saveNewAndWait();

    // Entry B — REUSES the shared tag via the suggestion dropdown (no dup).
    await form.navigateNew(TEST_WORKSPACE_ID);
    await form.fillTitle(titleB);
    await form.addExistingTag(shared);
    await form.saveNewAndWait();

    // Entry C — a different tag, must be excluded by the shared-tag filter.
    await form.navigateNew(TEST_WORKSPACE_ID);
    await form.fillTitle(titleC);
    await form.addNewTag(other);
    await form.saveNewAndWait();

    // Filter the list by the shared tag.
    await list.navigate(TEST_WORKSPACE_ID);
    await list.clickTagFilter(shared);

    expect(await list.cardExists(titleA)).toBeTruthy();
    expect(await list.cardExists(titleB)).toBeTruthy();
    expect(await list.cardExists(titleC)).toBeFalsy();
  });

  test('edits a knowledge entry and then deletes it', async ({ page }) => {
    const list = new KnowledgeListPage(page);
    const form = new KnowledgeFormPage(page);
    const id = uniq();
    const title = `KB Edit ${id}`;
    const tag = `edit-${id}`;

    // Create.
    await list.navigate(TEST_WORKSPACE_ID);
    await list.clickAddKnowledge();
    await form.fillTitle(title);
    await form.fillClaim('initial body');
    await form.addNewTag(tag);
    await form.saveNewAndWait();

    // Edit the claim and persist; reload proves it round-tripped to the backend.
    const detailUrl = page.url();
    await form.fillClaim('updated body after edit');
    await form.save();
    await page.goto(detailUrl);
    await form.isPageLoaded();
    expect(await form.getClaimValue()).toContain('updated body after edit');

    // Delete it; it disappears from the list.
    await form.deleteAndConfirm();
    await list.navigate(TEST_WORKSPACE_ID);
    await list.search(title);
    expect(await list.cardExists(title)).toBeFalsy();
  });
});
