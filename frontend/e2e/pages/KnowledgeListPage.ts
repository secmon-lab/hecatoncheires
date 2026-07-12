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
   * Type into the search box (debounced server-side substring search) and
   * wait until the debounced SearchKnowledge query actually returns, so the
   * list reflects the filter before the caller asserts on it. Beats a fixed
   * sleep: it neither races the 300ms debounce nor wastes idle time.
   */
  async search(text: string): Promise<void> {
    const settled = this.waitForGraphQL('SearchKnowledge');
    await this.searchInput.fill(text);
    await settled;
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
    // Selecting a tag chip changes the tagIds variable, forcing a fresh
    // GetKnowledges refetch; wait for that response rather than a fixed frame.
    const settled = this.waitForGraphQL('GetKnowledges');
    await this.page.locator('button.chip').filter({ hasText: label }).first().click();
    await settled;
  }

  /**
   * Resolve once a GraphQL response for the given operation returns 200.
   * Register the returned promise before triggering the request so the
   * debounced/async query cannot fire before we start listening.
   */
  private waitForGraphQL(operationName: string): Promise<unknown> {
    return this.page.waitForResponse((resp) => {
      if (!resp.url().includes('/graphql') || resp.status() !== 200) return false;
      try {
        return resp.request().postDataJSON()?.operationName === operationName;
      } catch {
        return false;
      }
    });
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
