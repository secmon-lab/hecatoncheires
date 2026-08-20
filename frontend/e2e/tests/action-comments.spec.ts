import { test, expect } from '@playwright/test';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { ActionDetailPage } from '../pages/ActionDetailPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for comments on an Action: post → reload (persisted) → edit
// (marked edited) → tab filtering → delete. The deep-link highlight is covered
// by the Vitest suite because it needs a comment id, which the UI does not
// surface.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Action comments', () => {
  let parentCaseTitle: string;
  let actionTitle: string;

  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    parentCaseTitle = `Comments Parent ${uniq()}`;
    actionTitle = `Comments Action ${uniq()}`;

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: parentCaseTitle,
      customFields: { category: 'task' },
    });

    await actionListPage.navigate(TEST_WORKSPACE_ID);
    await actionListPage.waitForBoardLoad();
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({ title: actionTitle, caseTitle: parentCaseTitle });
    await actionListPage.waitForBoardLoad();

    // Comments live in the activity feed inside the action modal.
    await actionListPage.clickActionByTitle(actionTitle);
    await actionDetailPage.waitForPageLoad();
  });

  test('post, persist, edit, filter by tab, and delete a comment', async ({ page }) => {
    const actionDetailPage = new ActionDetailPage(page);

    const firstBody = `investigated the alert ${uniq()}`;
    const secondBody = `revised after checking the runbook ${uniq()}`;

    // Post: the comment appears in the feed and is not marked edited.
    await actionDetailPage.addComment(firstBody);
    expect(await actionDetailPage.isCommentEdited(firstBody)).toBeFalsy();

    // The composer is cleared for the next comment.
    await expect(page.getByTestId('action-comment-body-textarea')).toHaveValue('');

    // Reload the modal (its URL carries the action id) — the comment
    // round-tripped to the backend rather than living in the Apollo cache.
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    await expect(actionDetailPage.commentRowByBody(firstBody)).toBeVisible();

    // Edit: the body is replaced and the row is marked edited.
    await actionDetailPage.editComment(firstBody, secondBody);
    await expect(actionDetailPage.commentRowByBody(firstBody)).toHaveCount(0);
    await expect.poll(() => actionDetailPage.isCommentEdited(secondBody)).toBeTruthy();

    // The edit also persisted.
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    await expect(actionDetailPage.commentRowByBody(secondBody)).toBeVisible();
    expect(await actionDetailPage.isCommentEdited(secondBody)).toBeTruthy();

    // Tab filtering: comments show on Comments, and are hidden on History
    // (which lists the action's structural change events instead).
    await actionDetailPage.selectActivityTab('comments');
    await expect(actionDetailPage.commentRowByBody(secondBody)).toBeVisible();
    await actionDetailPage.selectActivityTab('history');
    await expect(actionDetailPage.commentRowByBody(secondBody)).toHaveCount(0);
    await expect(page.getByTestId('activity-event-created')).toBeVisible();
    await actionDetailPage.selectActivityTab('all');
    await expect(actionDetailPage.commentRowByBody(secondBody)).toBeVisible();

    // Delete: the row is gone and stays gone after a reload.
    await actionDetailPage.deleteComment(secondBody);
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    await expect(actionDetailPage.commentRowByBody(secondBody)).toHaveCount(0);
  });

  test('the submit button stays disabled for a blank body', async ({ page }) => {
    const submit = page.getByTestId('action-comment-submit');
    const textarea = page.getByTestId('action-comment-body-textarea');

    await expect(submit).toBeDisabled();

    await textarea.fill('   ');
    await expect(submit).toBeDisabled();

    await textarea.fill('a real comment');
    await expect(submit).toBeEnabled();
  });
});
