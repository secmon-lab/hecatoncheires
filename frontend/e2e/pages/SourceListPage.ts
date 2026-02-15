import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Source List page
 */
export class SourceListPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly sourceTable: Locator;
  private readonly loadingIndicator: Locator;
  private readonly newSourceButton: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, h2').filter({ hasText: /Source/ });
    this.sourceTable = page.locator('table');
    this.loadingIndicator = page.locator('text=Loading...');
    this.newSourceButton = page.locator('button').filter({ hasText: /New Source/ });
  }

  /**
   * Navigate to the source list page
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/sources`);
  }

  /**
   * Get a source row by title
   */
  getSourceRowByTitle(title: string): Locator {
    return this.page.locator('tr').filter({ hasText: title });
  }

  /**
   * Click on a source row to view details
   */
  async clickSourceByTitle(title: string): Promise<void> {
    await this.getSourceRowByTitle(title).click();
    await this.waitForNavigation();
  }

  /**
   * Check if a source with the given title exists
   */
  async sourceExists(title: string): Promise<boolean> {
    try {
      await this.getSourceRowByTitle(title).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Click the "New Source" button
   */
  async clickNewSourceButton(): Promise<void> {
    await this.newSourceButton.click();
  }

  /**
   * Wait for the source table to load
   */
  async waitForTableLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    // Wait for either table or "No data available" message
    await Promise.race([
      this.sourceTable.waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
      this.page.locator('text=No data available').waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
    ]);
    // Wait for React to finish rendering
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
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
