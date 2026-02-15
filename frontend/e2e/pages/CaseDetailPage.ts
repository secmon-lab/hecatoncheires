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
  private readonly slackChannelLink: Locator;
  private readonly fieldsSection: Locator;
  private readonly timestampsSection: Locator;
  private readonly emptyActionState: Locator;

  constructor(page: Page) {
    super(page);
    // Case title is the h1 element inside main content area
    this.caseTitle = page.locator('main h1').first();
    // Description is the paragraph with description class
    this.caseDescription = page.locator('main p').first();
    this.editButton = page.locator('button').filter({ hasText: /Edit/ }).first();
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.backButton = page.locator('button, a').filter({ hasText: /Back/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
    // Slack channel link is now in the title row (an <a> tag starting with #)
    this.slackChannelLink = page.locator('a').filter({ hasText: /^#/ }).first();
    // Fields section contains the 2-column grid of field items
    this.fieldsSection = page.locator('h3').filter({ hasText: 'Fields' }).locator('..');
    // Timestamps row below description (text-transform: uppercase makes visible text "CREATED")
    this.timestampsSection = page.locator('main').locator('span').filter({ hasText: /^created$/i }).locator('..').first();
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
    // text-transform: uppercase makes visible text "CREATED", use case-insensitive match
    const createdLabel = this.page.locator('main').locator('span').filter({ hasText: /^created$/i }).first();
    return await createdLabel.isVisible();
  }

  /**
   * Get the Created timestamp text
   */
  async getCreatedTimestamp(): Promise<string> {
    const createdLabel = this.page.locator('main').locator('span').filter({ hasText: /^created$/i }).first();
    const container = createdLabel.locator('..');
    const valueSpan = container.locator('span').nth(1);
    return await valueSpan.textContent() || '';
  }

  /**
   * Get the Updated timestamp text
   */
  async getUpdatedTimestamp(): Promise<string> {
    const updatedLabel = this.page.locator('main').locator('span').filter({ hasText: /^updated$/i }).first();
    const container = updatedLabel.locator('..');
    const valueSpan = container.locator('span').nth(3);
    return await valueSpan.textContent() || '';
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
    // Click the Close button in the header to open confirmation modal
    const closeBtn = this.page.locator('button').filter({ hasText: /^Close$/ }).first();
    await closeBtn.click();

    // Wait for the confirmation modal and click the confirm Close button
    const modal = this.page.locator('[role="dialog"]');
    await modal.waitFor({ state: 'visible', timeout: 5000 });
    const confirmBtn = modal.locator('button').filter({ hasText: /^Close$/ });
    await confirmBtn.click();
    await this.page.waitForTimeout(1000);
  }

  /**
   * Click the Reopen button
   */
  async clickReopenButton(): Promise<void> {
    const reopenBtn = this.page.locator('button').filter({ hasText: /Reopen/ }).first();
    await reopenBtn.click();
    await this.page.waitForTimeout(1000);
  }

  /**
   * Check if Close button is visible
   */
  async isCloseButtonVisible(): Promise<boolean> {
    const closeBtn = this.page.locator('button').filter({ hasText: /^Close$/ }).first();
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
}
