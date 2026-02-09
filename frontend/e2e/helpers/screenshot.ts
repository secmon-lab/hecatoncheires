import { Page } from '@playwright/test';
import * as path from 'path';

/**
 * Helper functions for taking screenshots
 */

/**
 * Take a full page screenshot
 */
export async function takeFullPageScreenshot(
  page: Page,
  name: string
): Promise<void> {
  const screenshotPath = path.join('screenshots', `${name}.png`);
  await page.screenshot({ path: screenshotPath, fullPage: true });
}

/**
 * Take a screenshot of a specific element
 */
export async function takeElementScreenshot(
  page: Page,
  selector: string,
  name: string
): Promise<void> {
  const element = page.locator(selector).first();
  const screenshotPath = path.join('screenshots', `${name}.png`);
  await element.screenshot({ path: screenshotPath });
}

/**
 * Take a screenshot with timestamp
 */
export async function takeTimestampedScreenshot(
  page: Page,
  prefix: string = 'screenshot'
): Promise<void> {
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const name = `${prefix}-${timestamp}`;
  await takeFullPageScreenshot(page, name);
}
