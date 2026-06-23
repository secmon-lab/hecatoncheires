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

  constructor(page: Page) {
    super(page);
    this.fileInput = page.locator('input[type="file"]');
    this.dropzone = page.locator('[role="button"][aria-label]').first();
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
