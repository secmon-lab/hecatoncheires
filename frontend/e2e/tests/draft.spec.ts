import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// End-to-end coverage for the Save-as-draft → Edit-or-Submit flow that
// replaced the old dedicated /drafts pages.
//
// What this spec validates:
//   - The "Save as draft" button on the Case creation modal persists a
//     workspace-visible DRAFT (even when required fields are missing).
//   - The new draft surfaces under the Drafts tab on the Case list and
//     opens the same /cases/:id detail page as any other case.
//   - The detail page swaps Close/Reopen for Edit/Discard while the case
//     is still DRAFT, and Related Actions UI is hidden.
//   - The Edit modal pre-populates current values and offers Save-as-
//     draft (overwrite) plus Submit (atomic edit-and-promote).
//   - After Submit the page renders the freshly-OPEN case with the
//     edited title.
//   - Discard removes the draft and returns to the list.

test.describe('Drafts (Save-as-draft → Edit / Submit)', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('save as draft, surface on Drafts tab, then submit to promote', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const draftTitle = `E2E draft ${Date.now()}`;
    const promotedTitle = `${draftTitle} (final)`;

    // Open the New Case modal and use Save as draft (skipping required
    // fields is allowed for drafts).
    await caseListPage.clickNewCaseButton();
    await caseFormPage.waitForFormVisible();
    await caseFormPage.fillTitle(draftTitle);
    await caseFormPage.fillDescription('half-written body');
    await caseFormPage.clickSaveAsDraft();

    // The page navigates to the Case list; the Drafts tab carries the
    // new entry.
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    expect(await caseListPage.caseExists(draftTitle)).toBeTruthy();
    expect(await caseListPage.getDraftsTabCount()).toBeGreaterThanOrEqual(1);

    // Clicking a draft row should land on /cases/:id (not a /drafts/:id
    // detail page — those were retired).
    await caseListPage.clickCaseByTitle(draftTitle);
    await expect(page).toHaveURL(/\/cases\/\d+$/);
    await caseDetailPage.waitForPageLoad();
    expect(await caseDetailPage.isDraftHeader()).toBeTruthy();
    expect(await caseDetailPage.isDraftBadgeVisible()).toBeTruthy();
    // No Slack channel link, no Related Actions UI on a draft.
    expect(await caseDetailPage.isSlackChannelButtonVisible()).toBeFalsy();
    expect(await caseDetailPage.isCloseButtonVisible()).toBeFalsy();

    // Open the Edit modal and verify it pre-populates and shows the
    // draft-edit footer (Save-as-draft overwrite + Submit).
    await caseDetailPage.clickEditDraft();
    await caseFormPage.waitForFormVisible();
    expect(await caseFormPage.isDraftEditMode()).toBeTruthy();

    // Rename and promote in one shot — the backend SubmitDraft mutation
    // applies the edits atomically and activates the case.
    await caseFormPage.fillTitle(promotedTitle);
    await caseFormPage.fillCustomField('category', 'bug');
    await caseFormPage.submitDraftPromote();

    // The detail page now renders the OPEN case with the edited title.
    await caseDetailPage.waitForPageLoad();
    expect(await caseDetailPage.isDraftHeader()).toBeFalsy();
    expect(await caseDetailPage.isCloseButtonVisible()).toBeTruthy();
    const titleText = await caseDetailPage.getTitle();
    expect(titleText).toContain(promotedTitle);
  });

  test('overwrite the draft via Save-as-draft without promoting it', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const originalTitle = `E2E draft-overwrite ${Date.now()}`;
    const editedTitle = `${originalTitle} v2`;

    await caseListPage.clickNewCaseButton();
    await caseFormPage.waitForFormVisible();
    await caseFormPage.fillTitle(originalTitle);
    await caseFormPage.clickSaveAsDraft();

    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    await caseListPage.clickCaseByTitle(originalTitle);
    await caseDetailPage.waitForPageLoad();

    // Edit and save as draft (overwrite). The case must stay in DRAFT.
    await caseDetailPage.clickEditDraft();
    await caseFormPage.waitForFormVisible();
    await caseFormPage.fillTitle(editedTitle);
    await caseFormPage.clickDraftOverwrite();
    await caseDetailPage.waitForPageLoad();
    expect(await caseDetailPage.isDraftHeader()).toBeTruthy();
    const titleText = await caseDetailPage.getTitle();
    expect(titleText).toContain(editedTitle);
  });

  test('discard removes the draft and returns to the case list', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    const draftTitle = `E2E draft-discard ${Date.now()}`;

    await caseListPage.clickNewCaseButton();
    await caseFormPage.waitForFormVisible();
    await caseFormPage.fillTitle(draftTitle);
    await caseFormPage.clickSaveAsDraft();

    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    await caseListPage.clickCaseByTitle(draftTitle);
    await caseDetailPage.waitForPageLoad();

    await caseDetailPage.clickDiscardDraft();
    await page.waitForURL(new RegExp(`/ws/${TEST_WORKSPACE_ID}/cases$`));
    await caseListPage.waitForTableLoad();
    await caseListPage.clickStatusTab('Draft');
    expect(await caseListPage.caseExists(draftTitle)).toBeFalsy();
  });
});
