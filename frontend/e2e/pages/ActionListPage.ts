import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action List page
 */
export class ActionListPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly newActionButton: Locator;
  private readonly actionsTable: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, h2').filter({ hasText: /Action/ });
    this.newActionButton = page.locator('button').filter({ hasText: /New Action/ });
    this.actionsTable = page.locator('table');
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to the action list page
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/actions`);
  }

  /**
   * Click the "New Action" button
   */
  async clickNewActionButton(): Promise<void> {
    await this.newActionButton.click();
  }

  /**
   * Get an action row by title
   */
  getActionRowByTitle(title: string): Locator {
    return this.page.locator('tr').filter({ hasText: title });
  }

  /**
   * Click on an action row to view details
   */
  async clickActionByTitle(title: string): Promise<void> {
    await this.getActionRowByTitle(title).click();
    await this.waitForNavigation();
  }

  /**
   * Check if an action with the given title exists
   */
  async actionExists(title: string): Promise<boolean> {
    try {
      await this.getActionRowByTitle(title).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for the actions table to load
   */
  async waitForTableLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    // Wait for either table or "No data available" message
    await Promise.race([
      this.actionsTable.waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
      this.page.locator('text=No data available').waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
    ]);
    await this.page.waitForTimeout(500);
  }

  /**
   * Check if the page is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.pageTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
