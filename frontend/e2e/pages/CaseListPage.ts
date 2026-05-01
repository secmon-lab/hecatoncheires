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
    return this.page.locator('tr').filter({ hasText: title }).first();
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
    // Wait for React to finish rendering
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
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

  /**
   * Fill the search filter input
   */
  async fillSearchFilter(text: string): Promise<void> {
    await this.page.getByTestId('search-filter').fill(text);
    // Wait for React to re-render filtered results
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
  }

  /**
   * Clear the search filter
   */
  async clearSearchFilter(): Promise<void> {
    await this.page.getByTestId('search-filter').clear();
    // Wait for React to re-render unfiltered results
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
  }

  /**
   * Open the column selector popover
   */
  async openColumnSelector(): Promise<void> {
    await this.page.getByTestId('column-selector-button').click();
    await this.page.getByTestId('column-selector-popover').waitFor({ state: 'visible' });
  }

  /**
   * Toggle a column's visibility by its key
   */
  async toggleColumn(columnKey: string): Promise<void> {
    await this.page.getByTestId(`column-toggle-${columnKey}`).click();
  }

  /**
   * Check if a column header is visible in the table
   */
  async isColumnVisible(columnName: string): Promise<boolean> {
    const header = this.casesTable.locator('th').filter({ hasText: columnName });
    return await header.isVisible();
  }

  /**
   * Go to a specific page
   */
  async goToPage(direction: 'next' | 'prev'): Promise<void> {
    const currentInfo = await this.page.getByTestId('pagination-info').textContent() || '';
    if (direction === 'next') {
      await this.page.getByTestId('pagination-next').click();
    } else {
      await this.page.getByTestId('pagination-prev').click();
    }
    // Wait for pagination info to change, indicating the page has updated
    await this.page.waitForFunction(
      (prev) => {
        const el = document.querySelector('[data-testid="pagination-info"]');
        return el && el.textContent !== prev;
      },
      currentInfo,
      { timeout: 5000 }
    );
  }

  /**
   * Get the current pagination info text (e.g. "1 / 2")
   */
  async getPaginationInfo(): Promise<string> {
    return await this.page.getByTestId('pagination-info').textContent() || '';
  }

  /**
   * Get the number of rows currently displayed
   */
  async getRowCount(): Promise<number> {
    const rows = await this.casesTable.locator('tbody tr').all();
    return rows.length;
  }

  /**
   * Check if a case row shows the "Private" access denied label
   */
  async isAccessDenied(title: string): Promise<boolean> {
    const row = this.getCaseRowByTitle(title);
    const label = row.getByTestId('access-denied-label');
    return await label.isVisible();
  }

  /**
   * Get the text content of the access denied label in a row
   */
  async getAccessDeniedText(): Promise<string> {
    const label = this.page.getByTestId('access-denied-label').first();
    return (await label.textContent()) || '';
  }

  /**
   * Check if a private lock icon is visible for a case
   */
  async isPrivateLockVisible(title: string): Promise<boolean> {
    const row = this.getCaseRowByTitle(title);
    const lock = row.getByTestId('private-lock-icon');
    return await lock.isVisible();
  }

  /**
   * Click a status tab
   */
  async clickStatusTab(status: 'Open' | 'Closed'): Promise<void> {
    if (status === 'Open') {
      await this.page.getByTestId('status-tab-open').click();
    } else {
      await this.page.getByTestId('status-tab-closed').click();
    }
    await this.waitForTableLoad();
  }
}
