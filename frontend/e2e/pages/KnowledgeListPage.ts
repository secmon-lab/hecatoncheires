import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Knowledge List page (/ws/:id/knowledge).
 */
export class KnowledgeListPage extends BasePage {
  private readonly pageTitle: Locator;
  private readonly addButton: Locator;
  private readonly searchInput: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1').filter({ hasText: /Knowledge/ });
    this.addButton = page.locator('button').filter({ hasText: /Add Knowledge/ });
    this.searchInput = page.locator('input.h-search');
  }

  /**
   * Navigate to the knowledge list page.
   */
  async navigate(workspaceId: string = 'test'): Promise<void> {
    await this.goto(`/ws/${workspaceId}/knowledge`);
  }

  /**
   * Click the "Add Knowledge" button to open the new-knowledge form.
   */
  async clickAddKnowledge(): Promise<void> {
    await this.addButton.click();
    await this.waitForNavigation();
  }

  /**
   * Type into the search box (debounced server-side substring search).
   */
  async search(text: string): Promise<void> {
    await this.searchInput.fill(text);
    // The list debounces input by 300ms before issuing the search query.
    await this.page.waitForTimeout(450);
  }

  async clearSearch(): Promise<void> {
    await this.searchInput.fill('');
    await this.page.waitForTimeout(450);
  }

  /**
   * A knowledge card identified by its title text.
   */
  getCardByTitle(title: string): Locator {
    return this.page.locator('.card').filter({ hasText: title });
  }

  /**
   * Whether a knowledge card with the given title is visible.
   */
  async cardExists(title: string): Promise<boolean> {
    try {
      await this.getCardByTitle(title).first().waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async clickCardByTitle(title: string): Promise<void> {
    await this.getCardByTitle(title).first().click();
    await this.waitForNavigation();
  }

  /**
   * Click a tag filter chip by its (name) label. Filter chips are buttons,
   * distinguishing them from the read-only card tag chips (spans).
   */
  async clickTagFilter(label: string): Promise<void> {
    await this.page.locator('button.chip').filter({ hasText: label }).first().click();
    // Re-query is near-instant but give the list a frame to re-render.
    await this.page.waitForTimeout(300)
  }

  /**
   * Whether a card shows a given tag label (tag chip rendered inside the card).
   */
  async cardHasTag(title: string, tagLabel: string): Promise<boolean> {
    try {
      await this.getCardByTitle(title).first().locator('.chip', { hasText: tagLabel })
        .first().waitFor({ state: 'visible', timeout: 5000 })
      return true
    } catch {
      return false
    }
  }

  async isPageLoaded(): Promise<boolean> {
    try {
      await this.pageTitle.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
