import { test, expect } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// E2E coverage for the EN/JA language switcher in UserMenu. i18n is a
// mandatory project requirement, yet the switch UI itself was untested.
// Verifies the toggle changes the active language, re-renders localized text,
// and persists across a reload (localStorage key `hecatoncheires-lang`).
const LANG_STORAGE_KEY = 'hecatoncheires-lang';

test.describe('Language switcher', () => {
  test('switches EN/JA, relabels the UI, and persists across reload', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();

    // Open the user menu; English is the starting language.
    await page.getByTestId('user-menu-button').click();
    await expect(page.getByTestId('lang-option-en')).toHaveClass(/on/);
    const logoutEN = (await page.getByTestId('logout-button').textContent())?.trim() || '';
    expect(logoutEN.length).toBeGreaterThan(0);

    // Switch to Japanese: the JA option becomes active and localized copy
    // actually changes (the logout label is no longer the English string).
    await page.getByTestId('lang-option-ja').click();
    await expect(page.getByTestId('lang-option-ja')).toHaveClass(/on/);
    await expect(page.getByTestId('lang-option-en')).not.toHaveClass(/on/);
    await expect(page.getByTestId('logout-button')).not.toHaveText(logoutEN);
    expect(await page.evaluate((k) => localStorage.getItem(k), LANG_STORAGE_KEY)).toBe('ja');

    // Persist across a reload: JA remains the active language.
    await page.reload();
    await caseListPage.waitForTableLoad();
    await page.getByTestId('user-menu-button').click();
    await expect(page.getByTestId('lang-option-ja')).toHaveClass(/on/);

    // Switch back to English and confirm the preference flips back.
    await page.getByTestId('lang-option-en').click();
    await expect(page.getByTestId('lang-option-en')).toHaveClass(/on/);
    await expect(page.getByTestId('logout-button')).toHaveText(logoutEN);
    expect(await page.evaluate((k) => localStorage.getItem(k), LANG_STORAGE_KEY)).toBe('en');
  });
});
