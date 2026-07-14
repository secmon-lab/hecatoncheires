import { Page, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Page Object for the Case Agent settings page
 * (/ws/:workspaceId/cases/:id/agent).
 */
export class CaseAgentPage extends BasePage {
  async navigate(workspaceId: string, caseId: string): Promise<void> {
    await this.goto(`/ws/${workspaceId}/cases/${caseId}/agent`);
  }

  /** The read-only rendered "additional prompt" area (shown when not editing). */
  promptDisplay(): Locator {
    return this.page.getByTestId('agent-prompt-display');
  }

  /** Enter edit mode, replace the additional prompt, and save. */
  async editPrompt(text: string): Promise<void> {
    await this.page.getByTestId('agent-prompt-edit-button').click();
    const textarea = this.page.getByTestId('agent-prompt-textarea');
    await textarea.waitFor({ state: 'visible', timeout: 5000 });
    await textarea.fill(text);
    await this.page.getByTestId('agent-prompt-save-button').click();
    // Editing collapses back to the read-only view once the save resolves.
    await textarea.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {});
  }

  async isPageLoaded(): Promise<boolean> {
    try {
      await this.page.getByTestId('agent-prompt-edit-button').waitFor({ state: 'visible', timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
