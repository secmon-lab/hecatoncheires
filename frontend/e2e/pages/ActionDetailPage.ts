import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Modal (detail/edit/delete)
 */
export class ActionDetailPage extends BasePage {
  // Locators
  private readonly modalTitle: Locator;
  private readonly editButton: Locator;
  private readonly deleteButton: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.modalTitle = page.locator('#modal-title');
    this.editButton = page.locator('button').filter({ hasText: /Edit/ }).first();
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to a specific action via permalink (opens kanban + modal)
   */
  async navigate(workspaceId: string, actionId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/actions/${actionId}`);
  }

  /**
   * Get the action title displayed in the modal body
   */
  async getTitle(): Promise<string> {
    const titleText = this.page.locator('[class*="titleText"]');
    await titleText.waitFor({ state: 'visible', timeout: 5000 });
    return await titleText.textContent() || '';
  }

  /**
   * Click the Edit button in the modal
   */
  async clickEdit(): Promise<void> {
    await this.editButton.click();
  }

  /**
   * Click the Delete button in the modal
   */
  async clickDelete(): Promise<void> {
    await this.deleteButton.click();
  }

  /**
   * Wait for the modal to load
   */
  async waitForPageLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    await this.modalTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Check if the modal is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.modalTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get the current status value from the hidden dropdown
   */
  async getStatus(): Promise<string> {
    const dropdown = this.page.getByTestId('status-dropdown');
    return await dropdown.inputValue();
  }

  /**
   * Change the action status via the hidden dropdown
   */
  async changeStatus(newStatus: string): Promise<void> {
    const dropdown = this.page.getByTestId('status-dropdown');
    const responsePromise = this.page.waitForResponse(
      (resp) => resp.url().includes('/graphql') && resp.status() === 200
    );
    await dropdown.selectOption(newStatus);
    await responsePromise;
  }

  /**
   * Close the modal by clicking the close button
   */
  async closeModal(): Promise<void> {
    await this.page.getByRole('button', { name: 'Close' }).click();
    await this.page.locator('[class*="backdrop"]').waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
  }
}
