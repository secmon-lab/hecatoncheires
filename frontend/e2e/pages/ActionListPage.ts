import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Kanban Board page
 */
export class ActionListPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly newActionButton: Locator;
  private readonly kanbanBoard: Locator;
  private readonly loadingIndicator: Locator;
  private readonly searchInput: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h2').filter({ hasText: /Action/ });
    this.newActionButton = page.locator('button').filter({ hasText: /New Action/ });
    this.kanbanBoard = page.getByTestId('kanban-board');
    this.loadingIndicator = page.locator('text=Loading...');
    this.searchInput = page.getByTestId('action-search-input');
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
   * Get an action card by title
   */
  getActionCardByTitle(title: string): Locator {
    return this.page.getByTestId('action-card').filter({ hasText: title }).first();
  }

  /**
   * Click on an action card to open the modal
   */
  async clickActionByTitle(title: string): Promise<void> {
    await this.getActionCardByTitle(title).click();
  }

  /**
   * Check if an action with the given title exists on the kanban board
   */
  async actionExists(title: string): Promise<boolean> {
    try {
      await this.getActionCardByTitle(title).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for the kanban board to load
   */
  async waitForBoardLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    await this.kanbanBoard.waitFor({ state: 'visible', timeout: 10000 }).catch(() => {});
    // Wait for React to finish rendering
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
  }

  /**
   * Alias for backward compatibility
   */
  async waitForTableLoad(): Promise<void> {
    await this.waitForBoardLoad();
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

  private getColumnSlug(columnTitle: string): string {
    return columnTitle.toLowerCase().replace(/\s+/g, '-');
  }

  /**
   * Get the count displayed for a specific kanban column
   */
  async getColumnCount(columnTitle: string): Promise<number> {
    const column = this.page.getByTestId(`kanban-column-${this.getColumnSlug(columnTitle)}`);
    const countText = await column.locator('[class*="count"]').textContent();
    return parseInt(countText || '0');
  }

  /**
   * Check if a kanban column exists
   */
  async columnExists(columnTitle: string): Promise<boolean> {
    try {
      await this.page.getByTestId(`kanban-column-${this.getColumnSlug(columnTitle)}`).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Enter text in the search filter
   */
  async searchActions(text: string): Promise<void> {
    await this.searchInput.fill(text);
  }

  /**
   * Clear all filters
   */
  async clearFilters(): Promise<void> {
    const clearButton = this.page.getByTestId('action-filter-clear');
    if (await clearButton.isVisible()) {
      await clearButton.click();
    }
  }
}
