import { test, expect } from '@playwright/test';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { ActionDetailPage } from '../pages/ActionDetailPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for the action checklist (StepList) inside the action modal:
// add → complete (with reload persistence) → delete, asserting the progress
// pill tracks each change. Previously untested end to end.
function uniq(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

test.describe('Action steps (checklist)', () => {
  let parentCaseTitle: string;
  let actionTitle: string;

  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    parentCaseTitle = `Steps Parent ${uniq()}`;
    actionTitle = `Steps Action ${uniq()}`;

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

    // Open the action modal — steps live inside it.
    await actionListPage.clickActionByTitle(actionTitle);
    await actionDetailPage.waitForPageLoad();
  });

  test('add, complete (persisted), and delete steps', async ({ page }) => {
    const actionDetailPage = new ActionDetailPage(page);

    // No steps yet → no progress pill.
    expect(await actionDetailPage.getStepProgress()).toBe('');

    // Add two steps; the pill appears and counts them.
    await actionDetailPage.addStep('Step One');
    await actionDetailPage.addStep('Step Two');
    expect(await actionDetailPage.getStepProgress()).toBe('0/2');

    // Complete the first step; progress advances and the box is checked.
    await actionDetailPage.toggleStep('Step One');
    await expect
      .poll(() => actionDetailPage.getStepProgress())
      .toBe('1/2');
    expect(await actionDetailPage.isStepDone('Step One')).toBeTruthy();
    expect(await actionDetailPage.isStepDone('Step Two')).toBeFalsy();

    // Reload the modal (its URL carries the action id) — the completion
    // round-tripped to the backend.
    await page.reload();
    await actionDetailPage.waitForPageLoad();
    await expect(actionDetailPage.stepRowByTitle('Step One')).toBeVisible();
    expect(await actionDetailPage.getStepProgress()).toBe('1/2');
    expect(await actionDetailPage.isStepDone('Step One')).toBeTruthy();

    // Delete the second step; the total drops and the row is gone.
    await actionDetailPage.deleteStep('Step Two');
    await expect
      .poll(() => actionDetailPage.getStepProgress())
      .toBe('1/1');
    await expect(actionDetailPage.stepRowByTitle('Step Two')).toHaveCount(0);
  });
});
