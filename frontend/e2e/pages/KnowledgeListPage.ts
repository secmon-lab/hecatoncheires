import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Knowledge List page
 */
export class KnowledgeListPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly knowledgeTable: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, h2').filter({ hasText: /Knowledge/ });
    this.knowledgeTable = page.locator('table');
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to the knowledge list page
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/knowledges`);
  }

  /**
   * Get a knowledge row by title
   */
  getKnowledgeRowByTitle(title: string): Locator {
    return this.page.locator('tr').filter({ hasText: title });
  }

  /**
   * Click on a knowledge row to view details
   */
  async clickKnowledgeByTitle(title: string): Promise<void> {
    await this.getKnowledgeRowByTitle(title).click();
    await this.waitForNavigation();
  }

  /**
   * Check if a knowledge with the given title exists
   */
  async knowledgeExists(title: string): Promise<boolean> {
    try {
      await this.getKnowledgeRowByTitle(title).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for the knowledge table to load
   */
  async waitForTableLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    // Wait for either table or "No data available" message
    await Promise.race([
      this.knowledgeTable.waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
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
