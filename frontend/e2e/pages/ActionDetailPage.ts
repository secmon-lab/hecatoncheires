import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Action Modal (detail/edit/delete)
 */
export class ActionDetailPage extends BasePage {
  // Locators
  private readonly modalTitle: Locator;
  private readonly deleteButton: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.modalTitle = page.locator('#modal-title');
    this.deleteButton = page.locator('button').filter({ hasText: /Delete/ }).first();
    this.loadingIndicator = page.locator('text=Loading...');
  }

  /**
   * Navigate to a specific action via permalink (opens kanban + modal)
   */
  async navigate(workspaceId: string, actionId: number): Promise<void> {
    await this.goto(`/ws/${workspaceId}/actions/${actionId}`);
  }

  /**
   * Get the action title displayed in the modal body
   */
  async getTitle(): Promise<string> {
    // Multiple elements share class names like "titleText" (kanban cards + modal h3),
    // so target the modal title via its dedicated test id.
    const titleText = this.page.getByTestId('action-title');
    await titleText.waitFor({ state: 'visible', timeout: 5000 });
    return await titleText.textContent() || '';
  }

  /**
   * Inline-edit the action title (Linear-style: click → input → Enter).
   */
  async setTitle(next: string): Promise<void> {
    await this.page.getByTestId('action-title').click();
    const input = this.page.getByTestId('action-title-input');
    await input.waitFor({ state: 'visible', timeout: 3000 });
    await input.fill(next);
    await input.press('Enter');
  }

  /**
   * Inline-edit the action description.
   */
  async setDescription(next: string): Promise<void> {
    await this.page.getByTestId('action-description').click();
    const input = this.page.getByTestId('action-description-input');
    await input.waitFor({ state: 'visible', timeout: 3000 });
    await input.fill(next);
    await this.page.getByTestId('action-description-save').click();
  }

  /**
   * Click the Delete button in the modal
   */
  async clickDelete(): Promise<void> {
    await this.deleteButton.click();
  }

  /**
   * Wait for the modal to load
   */
  async waitForPageLoad(): Promise<void> {
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
    await this.modalTitle.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Check if the modal is loaded
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.modalTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get the current status value from the hidden dropdown
   */
  async getStatus(): Promise<string> {
    const dropdown = this.page.getByTestId('status-dropdown');
    return await dropdown.inputValue();
  }

  /**
   * Change the action status via the hidden dropdown
   */
  async changeStatus(newStatus: string): Promise<void> {
    const dropdown = this.page.getByTestId('status-dropdown');
    const responsePromise = this.page.waitForResponse(
      (resp) => resp.url().includes('/graphql') && resp.status() === 200
    );
    await dropdown.selectOption(newStatus);
    await responsePromise;
    // The mutation response settles the network, but the controlled <select>
    // only reflects the new value after Apollo normalises the cache and React
    // re-renders. Wait for that so callers reading getStatus() don't race.
    await expect(dropdown).toHaveValue(newStatus, { timeout: 5000 });
  }

  // --- Action steps (checklist) ---

  /** The step-list section within the action modal. */
  stepList(): Locator {
    return this.page.getByTestId('action-step-list');
  }

  /** A step row located by its title text. */
  stepRowByTitle(title: string): Locator {
    return this.stepList()
      .locator('[data-testid^="action-step-row-"]')
      .filter({ hasText: title });
  }

  /** The done/total progress pill text (e.g. "1/2"); empty when no steps. */
  async getStepProgress(): Promise<string> {
    const pill = this.stepList().getByTestId('action-step-progress');
    if (!(await pill.count())) return '';
    return (await pill.textContent()) || '';
  }

  /** Add a step through the inline add-row and wait for it to render. */
  async addStep(title: string): Promise<void> {
    await this.stepList().getByTestId('action-step-add-button').click();
    const input = this.stepList().getByTestId('action-step-add-input');
    await input.fill(title);
    await input.press('Enter');
    await expect(this.stepRowByTitle(title)).toBeVisible();
  }

  /** Toggle a step's done checkbox by its title. */
  async toggleStep(title: string): Promise<void> {
    await this.stepRowByTitle(title)
      .locator('[data-testid^="action-step-checkbox-"]')
      .click();
  }

  /** Whether the step with the given title is marked done. */
  async isStepDone(title: string): Promise<boolean> {
    const checked = await this.stepRowByTitle(title)
      .locator('[data-testid^="action-step-checkbox-"]')
      .getAttribute('aria-checked');
    return checked === 'true';
  }

  /** Delete a step by its title (hover reveals the row's delete control). */
  async deleteStep(title: string): Promise<void> {
    const row = this.stepRowByTitle(title);
    await row.hover();
    await row.locator('[data-testid^="action-step-delete-"]').click();
    await expect(row).toHaveCount(0);
  }

  /**
   * Close the modal by clicking the close button
   */
  async closeModal(): Promise<void> {
    await this.page.getByRole('button', { name: 'Close' }).click();
    await this.page.locator('[class*="backdrop"]').waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
  }
}
