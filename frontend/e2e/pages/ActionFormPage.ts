import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Form (Create) modal
 */
export class ActionFormPage extends BasePage {
  // Locators
  private readonly modal: Locator;
  private readonly titleInput: Locator;
  private readonly descriptionInput: Locator;
  private readonly submitButton: Locator;
  private readonly cancelButton: Locator;

  constructor(page: Page) {
    super(page);
    // Find modal by heading text (New Action, etc.)
    this.modal = page.locator('h2').filter({ hasText: /Action/ }).locator('..').locator('..').locator('..');
    this.titleInput = page.locator('input[placeholder*="title"]').first();
    this.descriptionInput = page.locator('textarea[placeholder*="description"]').first();
    this.submitButton = page.locator('button').filter({ hasText: /Save/ }).first();
    this.cancelButton = page.locator('button').filter({ hasText: /Cancel/ }).first();
  }

  /**
   * Wait for the form modal to be visible
   */
  async waitForFormVisible(): Promise<void> {
    // Wait for the modal title to appear
    await this.page.locator('h2').filter({ hasText: /New Action|Edit Action/ }).waitFor({ state: 'visible', timeout: 5000 });
    // Small delay to ensure form is ready
    await this.page.waitForTimeout(300);
  }

  /**
   * Fill in the action title
   */
  async fillTitle(title: string): Promise<void> {
    await this.titleInput.fill(title);
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

    // Wait for the listbox to appear
    await this.page.waitForTimeout(300);

    // Select by option with partial text match (case title appears in option like "Title (ID: X)")
    await this.page.getByRole('option', { name: new RegExp(caseTitle, 'i') }).first().click();
  }

  /**
   * Submit the form
   */
  async submit(): Promise<void> {
    await this.submitButton.click();
    // Wait for the modal to close by checking if the title is hidden
    await this.page.locator('h2').filter({ hasText: /New Action|Edit Action/ }).waitFor({ state: 'hidden', timeout: 10000 });
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
    description: string;
    caseTitle?: string;
  }): Promise<void> {
    await this.waitForFormVisible();

    // Select parent case if provided
    if (data.caseTitle) {
      await this.selectCase(data.caseTitle);
    }

    await this.fillTitle(data.title);
    await this.fillDescription(data.description);
    await this.submit();
  }
}
