import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Knowledge Detail page
 */
export class KnowledgeDetailPage extends BasePage {
  // Locators
  private readonly knowledgeTitle: Locator;
  private readonly knowledgeContent: Locator;
  private readonly backButton: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.knowledgeTitle = page.locator('h1, h2').first();
    this.knowledgeContent = page.locator('[data-testid="knowledge-content"], .content, article').first();
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to a specific knowledge detail page
   */
  async navigate(workspaceId: string, knowledgeId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/knowledge/${knowledgeId}`);
  }

  /**
   * Get the knowledge title
   */
  async getTitle(): Promise<string> {
    return await this.knowledgeTitle.textContent() || '';
  }

  /**
   * Get the knowledge content
   */
  async getContent(): Promise<string> {
    return await this.knowledgeContent.textContent() || '';
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
    await this.knowledgeTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Check if the page is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.knowledgeTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
