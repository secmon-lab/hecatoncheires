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
    this.titleInput = page.getByTestId('case-title-input');
    this.descriptionInput = page.getByTestId('case-description-input');
    this.submitButton = page.getByTestId('case-submit-button');
    this.cancelButton = page.locator('button').filter({ hasText: /Cancel/ }).first();
    this.closeButton = page.locator('button').first(); // X button
  }

  /**
   * Wait for the form modal to be visible
   */
  async waitForFormVisible(): Promise<void> {
    // Wait for the modal title to appear
    await this.page.locator('h2').filter({ hasText: /New Case|Edit Case/ }).waitFor({ state: 'visible', timeout: 5000 });
    // Wait for React to finish rendering the form
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
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
    // SELECT/MULTI_SELECT fields are rendered as react-select with id=fieldId
    const rsInput = this.page.locator(`#${fieldId}`);
    const isReactSelect = (await rsInput.count()) > 0
      && (await rsInput.getAttribute('role')) === 'combobox';
    if (isReactSelect) {
      await rsInput.click();
      const optionValue = value.charAt(0).toUpperCase() + value.slice(1);
      // react-select portals the menu to body; pick by visible text
      const opt = this.page.locator('.rs__option').filter({ hasText: optionValue }).first();
      await opt.waitFor({ state: 'visible', timeout: 3000 });
      await opt.click();
      return;
    }
    // TEXT / NUMBER / etc.
    const fieldLabel = fieldId.charAt(0).toUpperCase() + fieldId.slice(1);
    const inputField = this.page.locator(
      `#${fieldId}, [name="${fieldId}"], [data-field-id="${fieldId}"], input[placeholder*="${fieldLabel}"]`,
    ).first();
    await inputField.fill(value);
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
    description?: string;
    customFields?: Record<string, string>;
  }): Promise<void> {
    await this.waitForFormVisible();
    await this.fillTitle(data.title);
    if (data.description) {
      await this.fillDescription(data.description);
    }

    // Fill custom fields if provided
    if (data.customFields) {
      for (const [fieldId, value] of Object.entries(data.customFields)) {
        await this.fillCustomField(fieldId, value);
      }
    }

    await this.submit();
  }
}
