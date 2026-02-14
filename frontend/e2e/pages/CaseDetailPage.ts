import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case Detail page
 */
export class CaseDetailPage extends BasePage {
  // Locators
  private readonly caseTitle: Locator;
  private readonly caseDescription: Locator;
  private readonly editButton: Locator;
  private readonly deleteButton: Locator;
  private readonly backButton: Locator;
  private readonly loadingIndicator: Locator;
  private readonly slackChannelButton: Locator;

  constructor(page: Page) {
    super(page);
    // Case title is the h1 element inside main content area (not the page title)
    this.caseTitle = page.locator('main h1').first();
    // Description is the paragraph after the title row
    this.caseDescription = page.locator('main p').first();
    this.editButton = page.locator('button').filter({ hasText: /Edit/ }).first();
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
    this.slackChannelButton = page.locator('a').filter({ hasText: /^#/ }).first();
  }

  /**
   * Navigate to a specific case detail page
   */
  async navigate(workspaceId: string, caseId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/cases/${caseId}`);
  }

  /**
   * Get the case title
   */
  async getTitle(): Promise<string> {
    return await this.caseTitle.textContent() || '';
  }

  /**
   * Get the case description
   */
  async getDescription(): Promise<string> {
    return await this.caseDescription.textContent() || '';
  }

  /**
   * Click the edit button
   */
  async clickEdit(): Promise<void> {
    await this.editButton.click();
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
    await this.caseTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Get a custom field value by field name
   */
  async getCustomFieldValue(fieldName: string): Promise<string> {
    const field = this.page.locator(`[data-field-name="${fieldName}"], td, div`).filter({ hasText: fieldName }).first();
    return await field.textContent() || '';
  }

  /**
   * Check if the page is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.caseTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if the Slack channel button is visible
   */
  async isSlackChannelButtonVisible(): Promise<boolean> {
    return await this.slackChannelButton.isVisible();
  }

  /**
   * Get the Slack channel button text
   */
  async getSlackChannelButtonText(): Promise<string> {
    return await this.slackChannelButton.textContent() || '';
  }

  /**
   * Get the Slack channel button href
   */
  async getSlackChannelButtonHref(): Promise<string> {
    return await this.slackChannelButton.getAttribute('href') || '';
  }
}
