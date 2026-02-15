import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Detail page
 */
export class ActionDetailPage extends BasePage {
  // Locators
  private readonly actionTitle: Locator;
  private readonly actionDescription: Locator;
  private readonly deleteButton: Locator;
  private readonly backButton: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    // Action title is the h1 element inside main content area (not the page title)
    this.actionTitle = page.locator('main h1').first();
    // Description is the paragraph immediately after the title
    this.actionDescription = page.locator('main h1 + p').first();
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to a specific action detail page
   */
  async navigate(workspaceId: string, actionId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/actions/${actionId}`);
  }

  /**
   * Get the action title
   */
  async getTitle(): Promise<string> {
    return await this.actionTitle.textContent() || '';
  }

  /**
   * Get the action description
   */
  async getDescription(): Promise<string> {
    return await this.actionDescription.textContent() || '';
  }

  /**
   * Click the delete button
   */
  async clickDelete(): Promise<void> {
    await this.deleteButton.click();
  }

  /**
   * Click the back button
   */
  async clickBack(): Promise<void> {
    await this.backButton.click();
    await this.waitForNavigation();
  }

  /**
   * Wait for the page to load
   */
  async waitForPageLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    await this.actionTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Check if the page is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.actionTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get the current status value from the dropdown
   */
  async getStatus(): Promise<string> {
    const dropdown = this.page.getByTestId('status-dropdown');
    return await dropdown.inputValue();
  }

  /**
   * Change the action status via the dropdown
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
   * Check if the status dropdown is disabled
   */
  async isStatusDropdownDisabled(): Promise<boolean> {
    const dropdown = this.page.getByTestId('status-dropdown');
    return await dropdown.isDisabled();
  }
}
