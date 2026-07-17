import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the post-login Home page (the root "/" route).
 */
export class HomePage extends BasePage {
  // Locators
  private readonly root: Locator;
  private readonly openCasesSection: Locator;
  private readonly dueActionsSection: Locator;
  private readonly workspacesSection: Locator;
  private readonly workspaceChooser: Locator;

  constructor(page: Page) {
    super(page);
    this.root = page.getByTestId('home-page');
    this.openCasesSection = page.getByTestId('home-open-cases');
    this.dueActionsSection = page.getByTestId('home-due-actions');
    this.workspacesSection = page.getByTestId('home-workspaces');
    this.workspaceChooser = page.getByTestId('workspace-chooser');
  }

  /**
   * Navigate to the Home page (root path).
   */
  async navigate(): Promise<void> {
    await this.goto('/');
  }

  /**
   * Whether the Home page shell has rendered.
   */
  async isPageLoaded(): Promise<boolean> {
    try {
      await this.root.waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for the workspace chooser grid to finish loading (skeleton gone).
   */
  async waitForWorkspaceChooserLoad(): Promise<void> {
    await this.workspaceChooser.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * List of workspace ids currently rendered as cards, in display order.
   */
  async workspaceCardOrder(): Promise<string[]> {
    const cards = await this.workspaceChooser.locator('[data-testid^="workspace-card-"]').all();
    const ids: string[] = [];
    for (const card of cards) {
      const testId = await card.getAttribute('data-testid');
      if (testId) ids.push(testId.replace('workspace-card-', ''));
    }
    return ids;
  }

  /**
   * Toggle the favorite star for a given workspace id.
   */
  async toggleFavorite(workspaceId: string): Promise<void> {
    await this.page.getByTestId(`workspace-favorite-${workspaceId}`).click();
  }

  /**
   * Whether the given workspace's star is currently marked as favorited.
   */
  async isFavorite(workspaceId: string): Promise<boolean> {
    const pressed = await this.page
      .getByTestId(`workspace-favorite-${workspaceId}`)
      .getAttribute('aria-pressed');
    return pressed === 'true';
  }

  /**
   * Click a workspace card to navigate into it.
   */
  async openWorkspace(workspaceId: string): Promise<void> {
    await this.page.getByTestId(`workspace-card-${workspaceId}`).click();
    await this.waitForNavigation();
  }

  /**
   * Whether the "My Open Cases" section is showing its empty state.
   */
  async openCasesEmpty(): Promise<boolean> {
    return this.openCasesSection.getByText(/No open cases/).isVisible();
  }

  /**
   * Whether the "My Actions Due" section is showing its empty state.
   */
  async dueActionsEmpty(): Promise<boolean> {
    return this.dueActionsSection.getByText(/No due actions/).isVisible();
  }
}
