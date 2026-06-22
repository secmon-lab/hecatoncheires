import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Knowledge create/edit form
 * (/ws/:id/knowledge/new and /ws/:id/knowledge/:id). The same React page backs
 * both create and edit.
 */
export class KnowledgeFormPage extends BasePage {
  private readonly titleInput: Locator;
  private readonly claimInput: Locator;
  private readonly tagInput: Locator;
  private readonly saveButton: Locator;
  private readonly deleteButton: Locator;

  constructor(page: Page) {
    super(page);
    this.titleInput = page.getByTestId('knowledge-title-input');
    this.claimInput = page.getByTestId('knowledge-claim-input');
    this.tagInput = page.getByTestId('tag-input');
    this.saveButton = page.locator('button').filter({ hasText: /^Save$/ });
    this.deleteButton = page.locator('button').filter({ hasText: /^Delete$/ });
  }

  async navigateNew(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/knowledge/new`);
  }

  async fillTitle(title: string): Promise<void> {
    await this.titleInput.fill(title);
  }

  async fillClaim(claim: string): Promise<void> {
    await this.claimInput.fill(claim);
  }

  async getTitleValue(): Promise<string> {
    return this.titleInput.inputValue();
  }

  async getClaimValue(): Promise<string> {
    return this.claimInput.inputValue();
  }

  /**
   * Create a brand-new tag from the tag editor: type its name and click the
   * "Create …" option in the dropdown. Tags are first-class; this exercises the
   * create-tag-then-reference flow.
   */
  async addNewTag(name: string): Promise<void> {
    await this.tagInput.fill(name);
    await this.page.getByTestId('tag-create-option').click();
    await this.chip(name).waitFor({ state: 'visible', timeout: 5000 });
  }

  /**
   * Select an EXISTING tag by typing its name and clicking the matching
   * suggestion (reuse path, no duplicate tag created).
   */
  async addExistingTag(name: string): Promise<void> {
    await this.tagInput.fill(name);
    await this.page.getByTestId('tag-suggestion').filter({ hasText: name }).first().click();
    await this.chip(name).waitFor({ state: 'visible', timeout: 5000 });
  }

  /**
   * A selected-tag chip's remove control (uniquely labelled per tag name).
   */
  chip(name: string): Locator {
    return this.page.getByLabel(`Remove ${name}`);
  }

  async hasSelectedTag(name: string): Promise<boolean> {
    try {
      await this.chip(name).waitFor({ state: 'visible', timeout: 3000 });
      return true;
    } catch {
      return false;
    }
  }

  async save(): Promise<void> {
    await this.saveButton.click();
  }

  /**
   * Save a NEW entry and wait until the app navigates to the persisted entry's
   * detail URL (…/knowledge/<uuid>, no longer "new").
   */
  async saveNewAndWait(): Promise<void> {
    await this.save();
    await this.page.waitForURL(/\/knowledge\/(?!new$)[^/]+$/, { timeout: 10000 });
  }

  /**
   * Delete the current entry: open the confirm modal and confirm.
   */
  async deleteAndConfirm(): Promise<void> {
    await this.deleteButton.click();
    // The confirm dialog has its own danger "Delete" button.
    const dialog = this.page.locator('[role="dialog"]');
    await dialog.waitFor({ state: 'visible', timeout: 5000 });
    await dialog.locator('button').filter({ hasText: /^Delete$/ }).click();
    await this.waitForNavigation();
  }

  async isTagInputVisible(): Promise<boolean> {
    try {
      await this.tagInput.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async isPageLoaded(): Promise<boolean> {
    try {
      await this.titleInput.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
