import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case Detail page
 */
export class CaseDetailPage extends BasePage {
  // Locators
  private readonly caseTitle: Locator;
  private readonly caseDescription: Locator;
  private readonly deleteButton: Locator;
  private readonly backButton: Locator;
  private readonly loadingIndicator: Locator;
  private readonly slackChannelLink: Locator;
  private readonly fieldsSection: Locator;
  private readonly emptyActionState: Locator;

  constructor(page: Page) {
    super(page);
    // Case title is the inline-editable text inside the h1 wrapper
    this.caseTitle = page.getByTestId('case-title');
    // Description is the inline-editable long-text component
    this.caseDescription = page.getByTestId('case-description');
    // Delete moved into a kebab/hamburger menu
    this.deleteButton = page.getByTestId('case-delete-menu-item');
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
    // Slack channel link is now in the title row (an <a> tag starting with #)
    this.slackChannelLink = page.locator('a').filter({ hasText: /^#/ }).first();
    // Fields section contains the 2-column grid of field items
    this.fieldsSection = page.locator('h3').filter({ hasText: 'Fields' }).locator('..');
    // Empty action state
    this.emptyActionState = page.locator('p').filter({ hasText: 'No actions yet' }).first();
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
   * Inline-edit the case title (Linear-style: click → input → Enter).
   */
  async setTitle(next: string): Promise<void> {
    await this.caseTitle.click();
    const input = this.page.getByTestId('case-title-input');
    await input.waitFor({ state: 'visible', timeout: 3000 });
    await input.fill(next);
    await input.press('Enter');
  }

  /**
   * Inline-edit the case description (click → textarea → Save).
   */
  async setDescription(next: string): Promise<void> {
    await this.caseDescription.click();
    const input = this.page.getByTestId('case-description-input');
    await input.waitFor({ state: 'visible', timeout: 3000 });
    await input.fill(next);
    await this.page.getByTestId('case-description-save').click();
  }

  /**
   * Click the delete button (opens the kebab menu first, then the delete item)
   */
  async clickDelete(): Promise<void> {
    await this.page.getByTestId('case-menu-button').click();
    await this.deleteButton.waitFor({ state: 'visible', timeout: 3000 });
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
   * Get a field value from the Fields grid by field label name
   */
  async getFieldValue(fieldName: string): Promise<string> {
    // Find the field item that contains a label div with the given name,
    // then get the sibling value div's text
    const fieldItem = this.page
      .locator('div')
      .filter({ has: this.page.locator('div').filter({ hasText: new RegExp(`^${fieldName}$`) }) })
      .first();
    const valueDiv = fieldItem.locator('div').nth(1);
    return await valueDiv.textContent() || '';
  }

  /**
   * Check if the Fields section is visible
   */
  async isFieldsSectionVisible(): Promise<boolean> {
    const fieldsHeading = this.page.locator('h3').filter({ hasText: 'Fields' }).first();
    return await fieldsHeading.isVisible();
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
   * Check if the Slack channel link is visible
   */
  async isSlackChannelButtonVisible(): Promise<boolean> {
    return await this.slackChannelLink.isVisible();
  }

  /**
   * Get the Slack channel link text
   */
  async getSlackChannelButtonText(): Promise<string> {
    return await this.slackChannelLink.textContent() || '';
  }

  /**
   * Get the Slack channel link href
   */
  async getSlackChannelButtonHref(): Promise<string> {
    return await this.slackChannelLink.getAttribute('href') || '';
  }

  /**
   * Get the current case status (Open/Closed)
   */
  async getStatus(): Promise<string> {
    const chip = this.page.locator('main').locator('span').filter({ hasText: /^(Open|Closed)$/ }).first();
    return await chip.textContent() || '';
  }

  /**
   * Check if timestamps section is visible
   */
  async isTimestampsVisible(): Promise<boolean> {
    return await this.page.getByTestId('created-timestamp-value').isVisible();
  }

  /**
   * Get the Created timestamp text
   */
  async getCreatedTimestamp(): Promise<string> {
    return await this.page.getByTestId('created-timestamp-value').textContent() || '';
  }

  /**
   * Get the Updated timestamp text
   */
  async getUpdatedTimestamp(): Promise<string> {
    return await this.page.getByTestId('updated-timestamp-value').textContent() || '';
  }

  /**
   * Check if the empty action state is visible
   */
  async isEmptyActionStateVisible(): Promise<boolean> {
    return await this.emptyActionState.isVisible();
  }

  /**
   * Click the Close button (opens confirmation modal and confirms)
   */
  async clickCloseButton(): Promise<void> {
    // Click the Close button in the header (scoped by data-testid) to open confirmation modal
    const closeBtn = this.page.getByTestId('close-case-button');
    await closeBtn.click();

    // Wait for the confirmation modal and click the confirm Close button (scoped by data-testid)
    const confirmBtn = this.page.getByTestId('confirm-close-button');
    await confirmBtn.waitFor({ state: 'visible', timeout: 5000 });
    const responsePromise = this.page.waitForResponse(
      (resp) => resp.url().includes('/graphql') && resp.status() === 200
    );
    await confirmBtn.click();
    await responsePromise;
  }

  /**
   * Click the Reopen button
   */
  async clickReopenButton(): Promise<void> {
    const reopenBtn = this.page.locator('button').filter({ hasText: /Reopen/ }).first();
    const responsePromise = this.page.waitForResponse(
      (resp) => resp.url().includes('/graphql') && resp.status() === 200
    );
    await reopenBtn.click();
    await responsePromise;
  }

  /**
   * Check if Close button is visible
   */
  async isCloseButtonVisible(): Promise<boolean> {
    const closeBtn = this.page.getByTestId('close-case-button');
    return await closeBtn.isVisible();
  }

  /**
   * Check if Reopen button is visible
   */
  async isReopenButtonVisible(): Promise<boolean> {
    const reopenBtn = this.page.locator('button').filter({ hasText: /Reopen/ }).first();
    return await reopenBtn.isVisible();
  }

  /**
   * Click the Add Action button in the empty state
   */
  async clickAddActionInEmptyState(): Promise<void> {
    const addBtn = this.page.locator('button').filter({ hasText: /Add Action/ }).first();
    await addBtn.click();
  }

  /**
   * Check if the Private badge is visible on case detail
   */
  async isPrivateBadgeVisible(): Promise<boolean> {
    return await this.page.getByTestId('private-badge').isVisible();
  }

  /**
   * Get the Private badge text
   */
  async getPrivateBadgeText(): Promise<string> {
    return (await this.page.getByTestId('private-badge').textContent()) || '';
  }

  /**
   * Check if the Channel Members section is visible
   */
  async isChannelMembersSectionVisible(): Promise<boolean> {
    return await this.page.getByTestId('channel-members-section').isVisible();
  }

  /**
   * Get the channel members section title text (includes count)
   */
  async getChannelMembersSectionTitle(): Promise<string> {
    const section = this.page.getByTestId('channel-members-section');
    const title = section.locator('h3').first();
    return (await title.textContent()) || '';
  }

  /**
   * Click the Sync button in channel members section
   */
  async clickSyncButton(): Promise<void> {
    const syncBtn = this.page.getByTestId('sync-members-button');
    const responsePromise = this.page.waitForResponse(
      (resp) => resp.url().includes('/graphql') && resp.status() === 200
    );
    await syncBtn.click();
    await responsePromise;
  }

  /**
   * Check if Sync button is visible
   */
  async isSyncButtonVisible(): Promise<boolean> {
    return await this.page.getByTestId('sync-members-button').isVisible();
  }

  /**
   * Filter channel members by name
   */
  async filterMembers(text: string): Promise<void> {
    await this.page.getByTestId('member-search-filter').fill(text);
    await this.page.evaluate(() => new Promise(resolve => requestAnimationFrame(resolve)));
  }

  /**
   * Get the number of member items currently displayed
   */
  async getMemberCount(): Promise<number> {
    const section = this.page.getByTestId('channel-members-section');
    const members = await section.locator('[class*="memberItem"]').all();
    return members.length;
  }
}
