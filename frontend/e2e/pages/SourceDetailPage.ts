import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Source Detail page
 */
export class SourceDetailPage extends BasePage {
  // Locators
  private readonly sourceTitle: Locator;
  private readonly sourceContent: Locator;
  private readonly deleteButton: Locator;
  private readonly backButton: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.sourceTitle = page.locator('h1, h2').first();
    this.sourceContent = page.locator('[data-testid="source-content"], .content').first();
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to a specific source detail page
   */
  async navigate(workspaceId: string, sourceId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/sources/${sourceId}`);
  }

  /**
   * Get the source title
   */
  async getTitle(): Promise<string> {
    return await this.sourceTitle.textContent() || '';
  }

  /**
   * Get the source content
   */
  async getContent(): Promise<string> {
    return await this.sourceContent.textContent() || '';
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
    await this.sourceTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Check if the page is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.sourceTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
