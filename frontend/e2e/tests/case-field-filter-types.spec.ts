import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseKanbanPage } from '../pages/CaseKanbanPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { CaseFieldFilters } from '../pages/CaseFieldFilters';

test.use({ timezoneId: 'America/Los_Angeles' });

test('appends text, zero-valued numbers, dates and title-searched reference IDs', async ({ page }) => {
  const list = new CaseListPage(page);
  const form = new CaseFormPage(page);
  const filters = new CaseFieldFilters(page);
  const prefix = `Typed fields ${Date.now()}`;
  const refs: string[] = [];
  for (const name of ['Alpha', 'Beta']) {
    await list.navigate('test');
    await list.clickNewCaseButton();
    await form.createCase({ title: `${prefix} target ${name}`, customFields: { category: 'task' } });
    await list.fillSearchFilter(`${prefix} target ${name}`);
    await list.clickCaseByTitle(`${prefix} target ${name}`);
    refs.push(new URL(page.url()).pathname.split('/').pop()!);
  }
  for (const [name, ticket, estimate, due, ref] of [
    ['one', 'a,b&c', '0', '2026-10-01', 'Alpha'],
    ['two', 'second', '2', '2026-10-02', 'Beta'],
    ['excluded', 'other', '9', '2026-10-03', 'Beta'],
  ]) {
    await list.navigate('review');
    await list.clickNewCaseButton();
    await form.createCase({ title: `${prefix} ${name}`, customFields: { ticket, estimate, due, related: `${prefix} target ${ref}` } });
    await list.fillSearchFilter(`${prefix} ${name}`);
    await list.clickCaseByTitle(`${prefix} ${name}`);
    // Without Slack, explicitly assign a configured board status.
    await page.getByTestId('aside-board-status').selectOption('in_review');
  }
  // Archive an existing target after references are saved. A fresh filter
  // must still discover it, without a bookmarked selected reference ID.
  const detail = new CaseDetailPage(page);
  await detail.navigate('test', Number(refs[0]));
  await detail.clickCloseButton();
  await detail.clickArchive();
  await expect(detail.archivedBadge()).toBeVisible();
  await list.navigate('review');
  await filters.addValue('ticket', 'Ticket', 'a,b&c');
  await filters.addValue('ticket', 'Ticket', 'second');
  await filters.addValue('estimate', 'Estimate', '0');
  await filters.addValue('estimate', 'Estimate', '2');
  await filters.addValue('due', 'Due', '2026-10-01');
  await filters.addValue('due', 'Due', '2026-10-02');
  await filters.addCondition('related', 'Related');
  const related = page.getByTestId('case-field-filter-related').getByRole('combobox');
  await related.fill(`${prefix} target Alpha`);
  await page.getByRole('option', { name: `${prefix} target Alpha (#${refs[0]})`, exact: true }).click();
  await expect(list.getCaseRowByTitle(`${prefix} one`)).toBeVisible();
  await expect(list.getCaseRowByTitle(`${prefix} two`)).toHaveCount(0);
  await related.fill(`${prefix} target Beta`);
  await page.getByRole('option', { name: `${prefix} target Beta (#${refs[1]})`, exact: true }).click();
  await filters.close();
  await expect(list.getCaseRowByTitle(`${prefix} two`)).toBeVisible();
  await expect(list.getCaseRowByTitle(`${prefix} excluded`)).toHaveCount(0);
  expect(new URL(page.url()).searchParams.getAll('field.ticket')).toEqual(['a,b&c', 'second']);
  expect(new URL(page.url()).searchParams.getAll('field.related')).toEqual(refs);
  const sharedUrl = page.url();
  await page.reload();
  await expect(page).toHaveURL(sharedUrl);
  await expect(list.getCaseRowByTitle(`${prefix} two`)).toBeVisible();
  const summary = page.getByTestId('case-field-filters-summary');
  await expect(summary).toContainText(`${prefix} target Alpha`);
  await summary.getByRole('button', { name: 'Remove 2 from Estimate', exact: true }).click();
  await expect(list.getCaseRowByTitle(`${prefix} two`)).toHaveCount(0);
  await expect(list.getCaseRowByTitle(`${prefix} one`)).toBeVisible();
  await page.getByTestId('case-field-filters-clear-summary').click();
  await expect(list.getCaseRowByTitle(`${prefix} excluded`)).toBeVisible();
  const board = new CaseKanbanPage(page);
  await board.navigate('review');
  await filters.addCondition('related', 'Related');
  await page.getByTestId('case-field-filter-related').getByRole('combobox').fill(`${prefix} target Alpha`);
  await page.getByRole('option', { name: `${prefix} target Alpha (#${refs[0]})`, exact: true }).click();
  await filters.close();
  await expect(page.getByTestId('case-kanban-board')).toContainText(`${prefix} one`);
  await expect(page.getByTestId('case-kanban-board')).not.toContainText(`${prefix} two`);
  const boardUrl = page.url();
  await page.reload();
  await expect(page).toHaveURL(boardUrl);
  await expect(page.getByTestId('case-kanban-board')).toContainText(`${prefix} one`);
  await expect(page.getByTestId('case-field-filters-summary')).toContainText(`${prefix} target Alpha`);
});


test('the displayed custom date matches the filter and survives inline editing west of UTC', async ({ page }) => {
  const list = new CaseListPage(page);
  const form = new CaseFormPage(page);
  const filters = new CaseFieldFilters(page);
  const title = `Calendar day ${Date.now()}`;
  await list.navigate('review');
  await list.clickNewCaseButton();
  await form.createCase({ title, customFields: { due: '2026-10-01' } });
  await filters.addValue('due', 'Due', '2026-10-01');
  await filters.close();
  await expect(list.getCaseRowByTitle(title)).toBeVisible();
  await list.openColumnSelector();
  await page.getByTestId('column-toggle-field:due').getByRole('checkbox').check();
  await page.getByTestId('column-selector-button').click();
  await expect(list.getCaseRowByTitle(title)).toContainText('10/1/2026');
  await list.clickCaseByTitle(title);
  const due = page.getByTestId('field-due');
  await expect(due).toHaveText('10/1/2026');
  await due.click();
  await page.getByTestId('field-due-input').fill('2026-10-02');
  await page.getByTestId('field-due-input').press('Enter');
  await expect(due).toHaveText('10/2/2026');
  await page.reload();
  await expect(due).toHaveText('10/2/2026');
  await list.navigate('review');
  await filters.addValue('due', 'Due', '2026-10-01');
  await expect(list.getCaseRowByTitle(title)).toHaveCount(0);
  await filters.addValue('due', 'Due', '2026-10-02');
  await filters.close();
  await expect(list.getCaseRowByTitle(title)).toBeVisible();
});
