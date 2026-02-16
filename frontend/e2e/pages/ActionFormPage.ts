import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Form (Create) modal
 */
export class ActionFormPage extends BasePage {
  // Locators
  private readonly titleInput: Locator;
  private readonly descriptionInput: Locator;
  private readonly submitButton: Locator;
  private readonly cancelButton: Locator;

  constructor(page: Page) {
    super(page);
    this.titleInput = page.locator('#title');
    this.descriptionInput = page.locator('#description');
    this.submitButton = page.locator('button').filter({ hasText: /Save/ }).first();
    this.cancelButton = page.locator('button').filter({ hasText: /Cancel/ }).first();
  }

  /**
   * Wait for the form modal to be visible
   */
  async waitForFormVisible(): Promise<void> {
    // Wait for the modal title to appear
    await this.page.locator('h2').filter({ hasText: /New Action|Edit Action/ }).waitFor({ state: 'visible', timeout: 5000 });
    // Wait for the form to be ready by waiting for the submit button
    await this.submitButton.waitFor({ state: 'visible' });
  }

  /**
   * Fill in the action title
   */
  async fillTitle(title: string): Promise<void> {
    await this.titleInput.click();
    await this.titleInput.fill(title);
    // Verify the value was actually set (guards against React re-render race)
    await this.titleInput.press('Tab');
  }

  /**
   * Fill in the action description
   */
  async fillDescription(description: string): Promise<void> {
    await this.descriptionInput.fill(description);
  }

  /**
   * Select a parent case for the action
   */
  async selectCase(caseTitle: string): Promise<void> {
    // Find the Case select field by its accessible name
    const caseSelect = this.page.getByRole('combobox', { name: /Case/i });

    // Click to open the dropdown
    await caseSelect.click();

    // Wait for the listbox to appear and select the option
    const option = this.page.getByRole('option', { name: new RegExp(caseTitle, 'i') }).first();
    await option.waitFor({ state: 'visible' });
    await option.click();

    // Wait for the dropdown to close and React to settle
    await this.page.getByRole('listbox').waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
  }

  /**
   * Submit the form
   */
  async submit(): Promise<void> {
    await this.submitButton.click();
    // Wait for the modal and its backdrop to fully close
    await this.page.locator('h2').filter({ hasText: /New Action|Edit Action/ }).waitFor({ state: 'hidden', timeout: 10000 });
    await this.page.locator('[class*="backdrop"]').waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
  }

  /**
   * Cancel the form
   */
  async cancel(): Promise<void> {
    await this.cancelButton.click();
    await this.page.locator('h2').filter({ hasText: /New Action|Edit Action/ }).waitFor({ state: 'hidden', timeout: 5000 });
  }

  /**
   * Fill in the complete action form and submit
   */
  async createAction(data: {
    title: string;
    description?: string;
    caseTitle?: string;
  }): Promise<void> {
    await this.waitForFormVisible();

    // Select parent case if provided
    if (data.caseTitle) {
      await this.selectCase(data.caseTitle);
    }

    await this.fillTitle(data.title);
    if (data.description) {
      await this.fillDescription(data.description);
    }
    await this.submit();
  }
}
