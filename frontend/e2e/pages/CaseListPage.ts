import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case List page
 */
export class CaseListPage extends BasePage {
  // Locators
  private readonly pageTitle: Locator;
  private readonly newCaseButton: Locator;
  private readonly casesTable: Locator;
  private readonly loadingIndicator: Locator;
  private readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h2').filter({ hasText: /Management/ });
    this.newCaseButton = page.locator('button').filter({ hasText: /New/ });
    this.casesTable = page.locator('table');
    this.loadingIndicator = page.locator('text=Loading...');
    this.errorMessage = page.locator('text=/Error:/');
  }

  /**
   * Navigate to the case list page
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/cases`);
  }

  /**
   * Click the "New Case" button to open the form
   */
  async clickNewCaseButton(): Promise<void> {
    await this.newCaseButton.click();
  }

  /**
   * Get a case row by title
   */
  getCaseRowByTitle(title: string): Locator {
    return this.page.locator('tr').filter({ hasText: title });
  }

  /**
   * Click on a case row to view details
   */
  async clickCaseByTitle(title: string): Promise<void> {
    await this.getCaseRowByTitle(title).click();
    await this.waitForNavigation();
  }

  /**
   * Check if a case with the given title exists
   */
  async caseExists(title: string): Promise<boolean> {
    try {
      await this.getCaseRowByTitle(title).waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for the cases table to load
   */
  async waitForTableLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    // Wait for either table or "No data available" message
    await Promise.race([
      this.casesTable.waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
      this.page.locator('text=No data available').waitFor({ state: 'visible', timeout: 10000 }).catch(() => {}),
    ]);
    // Give a small delay for rendering
    await this.page.waitForTimeout(500);
  }

  /**
   * Get all case titles from the table
   */
  async getCaseTitles(): Promise<string[]> {
    await this.waitForTableLoad();
    const rows = await this.casesTable.locator('tbody tr').all();
    const titles: string[] = [];

    for (const row of rows) {
      const titleCell = row.locator('td').nth(1); // Title is usually the second column
      const text = await titleCell.textContent();
      if (text) {
        titles.push(text.trim());
      }
    }

    return titles;
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

  /**
   * Check if there's an error message
   */
  async hasError(): Promise<boolean> {
    try {
      await this.errorMessage.waitFor({ state: 'visible', timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }
}
