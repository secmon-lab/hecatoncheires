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
    // h1 is "{caseLabel}s" (e.g. "Cases") in the redesigned tactical layout.
    this.pageTitle = page.locator('main h1').first();
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
   * The row's title anchor. Rows navigate through a real <a>, so this is the
   * element that carries the detail-page href.
   */
  getCaseLinkByTitle(title: string): Locator {
    return this.getCaseRowByTitle(title).locator('a[data-testid^="case-row-link-"]').first();
  }

  /**
   * Click on a case row to view details
   */
  async clickCaseByTitle(title: string): Promise<void> {
    await this.getCaseLinkByTitle(title).click();
    await this.waitForNavigation();
  }

  /**
   * Click a row in a column other than the title. The whole row is a link, so
   * this must navigate to the same place the title does.
   */
  async clickCaseRowCellByHeader(title: string, header: string): Promise<void> {
    const headers = await this.casesTable.locator('thead th').allTextContents();
    const index = headers.findIndex((h) => h.trim().toLowerCase() === header.toLowerCase());
    if (index < 0) {
      throw new Error(`column header not found: ${header} (have: ${headers.join(', ')})`);
    }
    await this.getCaseRowByTitle(title).locator('td').nth(index).click();
    await this.waitForNavigation();
  }

  /**
   * The href the row link points at (relative path, e.g. /ws/test/cases/12).
   */
  async getCaseHrefByTitle(title: string): Promise<string> {
    const href = await this.getCaseLinkByTitle(title).getAttribute('href');
    return href ?? '';
  }

  /**
   * Modifier-click a case row the way a user opens it in a background tab, and
   * return the page the browser opened.
   */
  async openCaseInNewTabByTitle(title: string): Promise<Page> {
    const [newPage] = await Promise.all([
      this.page.context().waitForEvent('page'),
      this.getCaseLinkByTitle(title).click({ modifiers: ['ControlOrMeta'] }),
    ]);
    // A background tab starts at about:blank and only then navigates, so wait
    // for the detail URL rather than for a load state.
    await newPage.waitForURL(/\/cases\/\d+$/, { timeout: 10000 });
    return newPage;
  }

  /**
   * Whether the row for the given case title shows the "Test" badge.
   */
  async caseRowHasTestBadge(title: string): Promise<boolean> {
    return await this.getCaseRowByTitle(title).getByTestId('test-badge').isVisible();
  }

  /**
   * The text of one cell of a case row, selected by its column header. Use it
   * to read what a column actually renders (e.g. the Status column, which
   * shows the configured board status in a thread-mode workspace and the
   * lifecycle Open/Closed badge in a channel-mode one).
   */
  async getCaseRowCellTextByHeader(title: string, header: string): Promise<string> {
    const headers = await this.casesTable.locator('thead th').allTextContents();
    const index = headers.findIndex((h) => h.trim().toLowerCase() === header.toLowerCase());
    if (index < 0) {
      throw new Error(`column header not found: ${header} (have: ${headers.join(', ')})`);
    }
    const cell = this.getCaseRowByTitle(title).locator('td').nth(index);
    return (await cell.textContent())?.trim() ?? '';
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
   * Open the first case the currently rendered page links to.
   *
   * Do not snapshot the rows first and then click one by its text: the list
   * query is cache-and-network, so the rows rendered from the cache are
   * replaced when the network result lands, and rows other workers closed or
   * deleted in the shared test workspace disappear in that swap. A row index
   * or title captured before it can no longer resolve afterwards. Resolving
   * the row at click time survives the swap.
   *
   * Access-denied rows expose no detail page and carry no link, so matching
   * the link (not the row) skips them.
   */
  async clickFirstCase(): Promise<void> {
    await this.waitForTableLoad();
    await this.casesTable.locator('tbody a[data-testid^="case-row-link-"]').first().click();
    await this.waitForNavigation();
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
   * Choose how many rows a page shows (20 / 50 / 100 / 200).
   */
  async setPageSize(size: number): Promise<void> {
    await this.page.getByTestId('page-size-select').selectOption(String(size));
    // Wait for React to re-render the table with the new slice
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
  }

  /**
   * The page size currently selected in the footer control.
   */
  async getPageSize(): Promise<number> {
    return Number(await this.page.getByTestId('page-size-select').inputValue());
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
   * Click a status tab. "Draft" surfaces the workspace-wide drafts list
   * that lives under the same Case list page (no separate /drafts route).
   */
  async clickStatusTab(status: 'Open' | 'Closed' | 'Draft' | 'Archived' | 'All'): Promise<void> {
    if (status === 'Open') {
      await this.page.getByTestId('status-tab-open').click();
    } else if (status === 'Closed') {
      await this.page.getByTestId('status-tab-closed').click();
    } else if (status === 'All') {
      await this.page.getByTestId('status-tab-all').click();
    } else if (status === 'Archived') {
      await this.page.getByTestId('status-tab-archived').click();
    } else {
      await this.page.getByTestId('status-tab-draft').click();
    }
    await this.waitForTableLoad();
  }

  /**
   * Returns the visible count next to the Drafts tab label.
   */
  async getDraftsTabCount(): Promise<number> {
    const tab = this.page.getByTestId('status-tab-draft');
    await tab.waitFor({ state: 'visible', timeout: 5000 });
    const text = (await tab.textContent()) || '';
    const m = text.match(/(\d+)/);
    return m ? Number(m[1]) : 0;
  }

  /**
   * Tick the selection checkbox of one row. Available on the Drafts, Closed
   * and Archived tabs; the row must be accessible (an access-denied row's box
   * is disabled).
   */
  async selectRowByTitle(title: string): Promise<void> {
    await this.getCaseRowByTitle(title)
      .locator('input[data-testid^="bulk-row-checkbox-"]')
      .check();
  }

  /**
   * The "N selected" text of the bulk action bar.
   */
  async getBulkSelectedCount(): Promise<number> {
    const label = this.page.getByTestId('bulk-selected-count');
    await label.waitFor({ state: 'visible', timeout: 5000 });
    const m = ((await label.textContent()) || '').match(/(\d+)/);
    return m ? Number(m[1]) : 0;
  }

  /**
   * Archive the currently selected rows (Closed tab). The server archives them
   * in the background, so the rows disappear as soon as the call is accepted.
   */
  async clickBulkArchive(): Promise<void> {
    await this.page.getByTestId('bulk-archive-button').click();
  }

  /**
   * Restore the currently selected rows (Archived tab).
   */
  async clickBulkUnarchive(): Promise<void> {
    await this.page.getByTestId('bulk-unarchive-button').click();
  }

  /**
   * Whether the bulk action bar currently offers the given action.
   */
  async hasBulkAction(action: 'archive' | 'unarchive' | 'submit' | 'delete'): Promise<boolean> {
    return await this.page.getByTestId(`bulk-${action}-button`).isVisible();
  }
}
