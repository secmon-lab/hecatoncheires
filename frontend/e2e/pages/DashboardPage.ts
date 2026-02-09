import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Dashboard page
 */
export class DashboardPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly casesLink: Locator;
  private readonly actionsLink: Locator;
  private readonly knowledgeLink: Locator;
  private readonly sourcesLink: Locator;

  constructor(page: Page) {
    super(page);
    // Use the Hecatoncheires logo/title as the indicator that dashboard is loaded
    this.pageTitle = page.locator('text=Hecatoncheires').first();
    this.casesLink = page.locator('text=Cases').first();
    this.actionsLink = page.locator('text=Actions').first();
    this.knowledgeLink = page.locator('text=Knowledges').first();
    this.sourcesLink = page.locator('text=Sources').first();
  }

  /**
   * Navigate to the dashboard
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}`);
  }

  /**
   * Check if the dashboard is displayed
   */
  async isDashboardVisible(): Promise<boolean> {
    try {
      await this.pageTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Navigate to Cases page
   */
  async goToCases(): Promise<void> {
    await this.casesLink.click();
    await this.waitForNavigation();
  }

  /**
   * Navigate to Actions page
   */
  async goToActions(): Promise<void> {
    await this.actionsLink.click();
    await this.waitForNavigation();
  }

  /**
   * Navigate to Knowledge page
   */
  async goToKnowledge(): Promise<void> {
    await this.knowledgeLink.click();
    await this.waitForNavigation();
  }

  /**
   * Navigate to Sources page
   */
  async goToSources(): Promise<void> {
    await this.sourcesLink.click();
    await this.waitForNavigation();
  }
}
