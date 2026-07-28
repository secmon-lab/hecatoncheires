import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case Import flow:
 *   /ws/:workspaceId/imports/new  → upload a YAML file
 *   /ws/:workspaceId/imports/:id  → review preview and execute
 *
 * The flow has no list page by design (sessions are addressable only by
 * URL), so callers either land here from CaseList → [Import] or by
 * navigating directly to the detail URL.
 */
export class ImportPage extends BasePage {
  private readonly fileInput: Locator;
  private readonly dropzone: Locator;
  private readonly executeButton: Locator;
  private readonly openCasesListButton: Locator;
  private readonly statusBadge: Locator;
  private readonly schemaToggle: Locator;
  private readonly copySchemaButton: Locator;
  private readonly pasteModeButton: Locator;
  private readonly fileModeButton: Locator;
  private readonly pasteTextarea: Locator;
  private readonly pasteSubmitButton: Locator;

  constructor(page: Page) {
    super(page);
    this.fileInput = page.locator('input[type="file"]');
    this.dropzone = page.locator('[role="button"][aria-label]').first();
    this.pasteModeButton = page.getByTestId('import-mode-paste');
    this.fileModeButton = page.getByTestId('import-mode-file');
    this.pasteTextarea = page.getByTestId('import-paste-textarea');
    this.pasteSubmitButton = page.getByTestId('import-paste-submit');
    // Match the accessible name in both locales so cross-locale
    // automation stays green regardless of the configured default lang.
    // Status badges are uppercase ASCII so they remain locale-stable.
    this.executeButton = page.getByRole('button', {
      name: /Execute import|取り込みを実行/i,
    });
    this.openCasesListButton = page.getByRole('button', {
      name: /Open cases list|ケース一覧へ/i,
    });
    this.statusBadge = page.locator('main span').filter({
      hasText: /^(PENDING|APPLIED|FAILED)$/,
    }).first();
    // The "schema" summary text differs across locales ("YAML schema" in
    // en / "YAML スキーマ" in ja). Anchor on structure instead — the
    // single <details><summary> block on the New Import page.
    this.schemaToggle = page.locator('main details > summary').first();
    this.copySchemaButton = page.locator('main details button').first();
  }

  /** Go to the New Import page. */
  async navigateNew(workspaceId: string): Promise<void> {
    await this.goto(`/ws/${workspaceId}/imports/new`);
    await this.dropzone.waitFor({ state: 'visible' });
  }

  /**
   * Upload a YAML file by setting the hidden <input type="file"> directly.
   * Returns the import session ID extracted from the URL after redirect.
   */
  async uploadYaml(yamlContent: string, fileName = 'incidents.yaml'): Promise<string> {
    await this.fileInput.setInputFiles({
      name: fileName,
      mimeType: 'application/yaml',
      buffer: Buffer.from(yamlContent, 'utf-8'),
    });
    return await this.waitForSessionId();
  }

  /** Switch the New Import page to the paste input surface. */
  async switchToPasteMode(): Promise<void> {
    await this.pasteModeButton.click();
    await this.pasteTextarea.waitFor({ state: 'visible' });
  }

  /** Switch back to the file dropzone. */
  async switchToFileMode(): Promise<void> {
    await this.fileModeButton.click();
    await this.dropzone.waitFor({ state: 'visible' });
  }

  /**
   * Type YAML into the paste box and submit it. Returns the import
   * session ID extracted from the URL after redirect.
   */
  async pasteYaml(yamlContent: string): Promise<string> {
    await this.switchToPasteMode();
    await this.pasteTextarea.fill(yamlContent);
    await this.pasteSubmitButton.click();
    return await this.waitForSessionId();
  }

  /** True when the paste box holds no content (submit stays disabled). */
  async isPasteSubmitEnabled(): Promise<boolean> {
    return await this.pasteSubmitButton.isEnabled();
  }

  /** Read the current content of the paste box. */
  async readPasteBox(): Promise<string> {
    return await this.pasteTextarea.inputValue();
  }

  /** Empty the paste box (submit must go back to disabled). */
  async clearPasteBox(): Promise<void> {
    await this.pasteTextarea.fill('');
  }

  /**
   * Page-level paste: put the YAML on the real clipboard and press
   * ⌘V / Ctrl+V with no input focused, the way a user does it. Driving
   * the actual key press (rather than dispatching a synthetic event) is
   * what verifies the document-level listener receives the paste and
   * that the focus check lets it through.
   */
  async pasteOnPage(yamlContent: string): Promise<void> {
    await this.page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await this.page.evaluate(
      (text) => navigator.clipboard.writeText(text),
      yamlContent,
    );
    // Nothing must hold focus, or the paste lands in that element instead.
    await this.page.evaluate(() => {
      const active = document.activeElement;
      if (active instanceof HTMLElement) active.blur();
    });
    await this.page.keyboard.press('ControlOrMeta+V');
    await this.pasteTextarea.waitFor({ state: 'visible' });
  }

  private async waitForSessionId(): Promise<string> {
    // The detail URL is /imports/<uuid>. We must match only the
    // session-id form here — a bare /imports/[^/]+ regex also matches
    // the source /imports/new page, which is what Playwright sees
    // *before* the createCaseImport mutation resolves. Pin the match
    // to UUID v4 shape (8-4-4-4-12 hex) so the wait truly blocks
    // until the post-upload navigate fires.
    await this.page.waitForURL(
      /\/imports\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
      { timeout: 15_000 },
    );
    const url = new URL(this.page.url());
    const m = url.pathname.match(/\/imports\/([0-9a-f-]{8,})$/i);
    if (!m) {
      throw new Error(`unexpected URL after upload: ${this.page.url()}`);
    }
    return m[1];
  }

  /** Open the YAML schema details panel and copy the JSON Schema. */
  async copyJsonSchema(): Promise<void> {
    await this.schemaToggle.click();
    await this.copySchemaButton.click();
  }

  /** Click the Execute import button (must be enabled). */
  async clickExecute(): Promise<void> {
    await this.executeButton.click();
  }

  /** True when Execute is rendered AND enabled. */
  async isExecuteEnabled(): Promise<boolean> {
    if (!(await this.executeButton.isVisible())) return false;
    return await this.executeButton.isEnabled();
  }

  /** Read the session-status badge text ("PENDING" / "APPLIED" / "FAILED"). */
  async readStatus(): Promise<string> {
    await this.statusBadge.waitFor({ state: 'visible' });
    return (await this.statusBadge.textContent())?.trim() ?? '';
  }

  /**
   * Wait for the status badge to reach the given value. Useful right
   * after clicking Execute (Apollo refetches and the badge swaps from
   * PENDING to APPLIED / FAILED).
   */
  async waitForStatus(expected: 'PENDING' | 'APPLIED' | 'FAILED', timeoutMs = 10_000): Promise<void> {
    await this.page.locator('main span').filter({ hasText: new RegExp(`^${expected}$`) }).first().waitFor({
      state: 'visible',
      timeout: timeoutMs,
    });
  }

  /** Click "Open cases list" on the success banner (APPLIED variant). */
  async clickOpenCasesList(): Promise<void> {
    await this.openCasesListButton.click();
  }
}
