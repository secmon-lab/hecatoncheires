import { expect, Page } from '@playwright/test';

/** Shared condition composer on the Case list and either board. */
export class CaseFieldFilters {
  constructor(private readonly page: Page) {}

  async open(): Promise<void> {
    if (!await this.page.getByTestId('case-field-filters-panel').isVisible()) {
      await this.page.getByTestId('case-field-filters-button').click();
    }
  }

  async addCondition(fieldId: string, fieldName: string): Promise<void> {
    await this.open();
    if (!await this.page.getByTestId(`case-field-filter-${fieldId}`).isVisible()) {
      await this.page.getByRole('combobox', { name: 'Add condition', exact: true }).click();
      await this.page.getByRole('option', { name: fieldName, exact: true }).click();
    }
  }

  async toggleOption(fieldId: string, optionName: string, fieldName?: string): Promise<void> {
    const name = fieldName ?? fieldId.split('_').map((s) => s.charAt(0).toUpperCase() + s.slice(1)).join(' ');
    await this.addCondition(fieldId, name);
    const field = this.page.getByTestId(`case-field-filter-${fieldId}`);
    const remove = field.getByRole('button', { name: `Remove ${optionName}`, exact: true });
    if (await remove.isVisible()) {
      await remove.click();
      return;
    }
    await field.getByRole('combobox').click();
    await this.page.getByRole('option', { name: optionName, exact: true }).click();
  }

  async addValue(fieldId: string, fieldName: string, value: string): Promise<void> {
    await this.addCondition(fieldId, fieldName);
    const field = this.page.getByTestId(`case-field-filter-${fieldId}`);
    await field.getByLabel(fieldName, { exact: true }).fill(value);
    await field.getByRole('button', { name: 'Add', exact: true }).click();
  }

  async close(): Promise<void> {
    await this.page.getByTestId('case-field-filters-button').click();
    await expect(this.page.getByTestId('case-field-filters-panel')).toBeHidden();
  }

  async clear(): Promise<void> {
    await this.open();
    await this.page.getByTestId('case-field-filters-clear').click();
  }
}
