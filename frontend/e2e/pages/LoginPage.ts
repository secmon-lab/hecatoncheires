import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Login page
 */
export class LoginPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly loginButton: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h2').filter({ hasText: /Hecatoncheires/ });
    this.loginButton = page.locator('button').filter({ hasText: /Sign in/ });
  }

  /**
   * Navigate to the login page
   */
  async navigate(): Promise<void> {
    await this.goto('/');
  }

  /**
   * Click the login button
   */
  async clickLogin(): Promise<void> {
    await this.loginButton.click();
    await this.waitForNavigation();
  }

  /**
   * Check if the login page is displayed
   */
  async isLoginPageVisible(): Promise<boolean> {
    try {
      await this.pageTitle.waitFor({ state: 'visible', timeout: 5000 });
      await this.loginButton.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
