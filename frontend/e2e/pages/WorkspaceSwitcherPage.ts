import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the workspace switcher in the top bar. It is rendered on
 * every workspace-scoped page, so this object is used alongside whichever
 * page object drives the surrounding screen.
 */
export class WorkspaceSwitcherPage extends BasePage {
  private readonly trigger: Locator;
  private readonly menu: Locator;
  private readonly moreButton: Locator;

  constructor(page: Page) {
    super(page);
    // The switcher is the first control in the top bar; the breadcrumb and the
    // user menu are separate elements, so scoping to header > .h-ws is stable.
    this.trigger = page.locator('header button.h-ws');
    this.menu = page.getByTestId('workspace-switcher-menu');
    this.moreButton = page.getByTestId('workspace-switcher-more');
  }

  /**
   * Open the dropdown and wait for it to render.
   */
  async open(): Promise<void> {
    await this.trigger.click();
    await this.menu.waitFor({ state: 'visible', timeout: 10000 });
  }

  /**
   * Whether the dropdown is currently rendered.
   */
  async isOpen(): Promise<boolean> {
    return this.menu.isVisible();
  }

  /**
   * The trigger button's reported expanded state.
   */
  async triggerAriaExpanded(): Promise<string | null> {
    return this.trigger.getAttribute('aria-expanded');
  }

  /**
   * The element id the trigger claims to control, and the dropdown's own id,
   * so a test can assert the two match.
   */
  async triggerAriaControls(): Promise<string | null> {
    return this.trigger.getAttribute('aria-controls');
  }

  async menuElementId(): Promise<string | null> {
    return this.menu.getAttribute('id');
  }

  /**
   * Whether the browser's focus currently sits on the trigger button.
   */
  async isTriggerFocused(): Promise<boolean> {
    return this.trigger.evaluate((el) => el === document.activeElement);
  }

  /**
   * Move keyboard focus into the dropdown (the first workspace row).
   */
  async focusFirstEntry(): Promise<void> {
    await this.menu.locator('[data-testid^="workspace-switcher-item-"]').first().focus();
  }

  /**
   * Press Escape from wherever focus currently is.
   */
  async pressEscape(): Promise<void> {
    await this.page.keyboard.press('Escape');
    await this.menu.waitFor({ state: 'hidden', timeout: 10000 });
  }

  /**
   * Workspace ids currently rendered in the dropdown, in display order.
   */
  async visibleWorkspaceIds(): Promise<string[]> {
    const items = await this.menu.locator('[data-testid^="workspace-switcher-item-"]').all();
    const ids: string[] = [];
    for (const item of items) {
      const testId = await item.getAttribute('data-testid');
      if (testId) ids.push(testId.replace('workspace-switcher-item-', ''));
    }
    return ids;
  }

  /**
   * Whether the given workspace's row shows the favorite star.
   */
  async isStarred(workspaceId: string): Promise<boolean> {
    return this.page.getByTestId(`workspace-switcher-star-${workspaceId}`).isVisible();
  }

  /**
   * Index of the first row without a favorite star, i.e. where the favorites
   * block ends. Returns the row count when every visible row is starred.
   */
  async firstUnstarredIndex(): Promise<number> {
    const ids = await this.visibleWorkspaceIds();
    for (let i = 0; i < ids.length; i++) {
      if (!(await this.isStarred(ids[i]))) return i;
    }
    return ids.length;
  }

  /**
   * Whether the "show more" toggle is present (i.e. entries are collapsed).
   */
  async hasMoreToggle(): Promise<boolean> {
    return this.moreButton.isVisible();
  }

  /**
   * Reveal the collapsed entries.
   */
  async expand(): Promise<void> {
    await this.moreButton.click();
    await this.moreButton.and(this.page.locator('[aria-expanded="true"]')).waitFor({
      state: 'visible',
      timeout: 10000,
    });
  }

  /**
   * Click a workspace row to switch into that workspace.
   */
  async selectWorkspace(workspaceId: string): Promise<void> {
    await this.page.getByTestId(`workspace-switcher-item-${workspaceId}`).click();
    await this.waitForNavigation();
  }
}
