import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case Form (Create/Edit) modal
 */
export class CaseFormPage extends BasePage {
  // Locators
  private readonly titleInput: Locator;
  private readonly descriptionInput: Locator;
  private readonly submitButton: Locator;
  private readonly cancelButton: Locator;
  private readonly closeButton: Locator;

  constructor(page: Page) {
    super(page);
    this.titleInput = page.locator('input[placeholder*="title"]').first();
    this.descriptionInput = page.locator('textarea[placeholder*="description"]').first();
    this.submitButton = page.locator('button').filter({ hasText: /Save/ }).first();
    this.cancelButton = page.locator('button').filter({ hasText: /Cancel/ }).first();
    this.closeButton = page.locator('button').first(); // X button
  }

  /**
   * Wait for the form modal to be visible
   */
  async waitForFormVisible(): Promise<void> {
    // Wait for the modal title to appear
    await this.page.locator('h2').filter({ hasText: /New Case|Edit Case/ }).waitFor({ state: 'visible', timeout: 5000 });
    // Small delay to ensure form is ready
    await this.page.waitForTimeout(300);
  }

  /**
   * Fill in the case title
   */
  async fillTitle(title: string): Promise<void> {
    await this.titleInput.fill(title);
  }

  /**
   * Fill in the case description
   */
  async fillDescription(description: string): Promise<void> {
    await this.descriptionInput.fill(description);
  }

  /**
   * Fill a custom field by field ID
   * Automatically detects if it's a select or input field
   */
  async fillCustomField(fieldId: string, value: string): Promise<void> {
    // Capitalize first letter of fieldId for label matching
    const fieldLabel = fieldId.charAt(0).toUpperCase() + fieldId.slice(1);

    // Try to find as a select/combobox by accessible name or label
    const selectByLabel = this.page.getByRole('combobox', { name: new RegExp(fieldLabel, 'i') });
    const selectCount = await selectByLabel.count();

    if (selectCount > 0) {
      // It's a select field - use selectOption
      // Capitalize first letter of value to match options like "Bug", "Feature"
      const optionValue = value.charAt(0).toUpperCase() + value.slice(1);
      await selectByLabel.first().selectOption({ label: optionValue });
    } else {
      // Try as a regular input field
      const inputField = this.page.locator(`[name="${fieldId}"], [data-field-id="${fieldId}"], input[placeholder*="${fieldLabel}"]`).first();
      await inputField.fill(value);
    }
  }

  /**
   * Select a value from a select field
   */
  async selectFieldValue(fieldId: string, value: string): Promise<void> {
    const field = this.page.locator(`select[name="${fieldId}"], [data-field-id="${fieldId}"] select`).first();
    await field.selectOption(value);
  }

  /**
   * Submit the form
   */
  async submit(): Promise<void> {
    await this.submitButton.click();
    // Wait for the modal to close by checking if the title is hidden
    await this.page.locator('h2').filter({ hasText: /New Case|Edit Case/ }).waitFor({ state: 'hidden', timeout: 10000 });
  }

  /**
   * Cancel the form
   */
  async cancel(): Promise<void> {
    await this.cancelButton.click();
    await this.page.locator('h2').filter({ hasText: /New Case|Edit Case/ }).waitFor({ state: 'hidden', timeout: 5000 });
  }

  /**
   * Close the form using the close button
   */
  async close(): Promise<void> {
    await this.closeButton.click();
    await this.page.locator('h2').filter({ hasText: /New Case|Edit Case/ }).waitFor({ state: 'hidden', timeout: 5000 });
  }

  /**
   * Check if the form is visible
   */
  async isVisible(): Promise<boolean> {
    try {
      await this.modal.waitFor({ state: 'visible', timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Fill in the complete case form and submit
   */
  async createCase(data: {
    title: string;
    description: string;
    customFields?: Record<string, string>;
  }): Promise<void> {
    await this.waitForFormVisible();
    await this.fillTitle(data.title);
    await this.fillDescription(data.description);

    // Fill custom fields if provided
    if (data.customFields) {
      for (const [fieldId, value] of Object.entries(data.customFields)) {
        await this.fillCustomField(fieldId, value);
      }
    }

    await this.submit();
  }
}
