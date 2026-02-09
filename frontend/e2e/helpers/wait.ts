import { Page, Locator } from '@playwright/test';

/**
 * Helper functions for waiting and retrying
 */

/**
 * Wait for a condition to be true with retry
 */
export async function waitForCondition(
  conditionFn: () => Promise<boolean>,
  options: {
    timeout?: number;
    interval?: number;
    errorMessage?: string;
  } = {}
): Promise<void> {
  const {
    timeout = 10000,
    interval = 500,
    errorMessage = 'Condition not met within timeout',
  } = options;

  const startTime = Date.now();

  while (Date.now() - startTime < timeout) {
    if (await conditionFn()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, interval));
  }

  throw new Error(errorMessage);
}

/**
 * Wait for network idle
 */
export async function waitForNetworkIdle(
  page: Page,
  timeout: number = 30000
): Promise<void> {
  await page.waitForLoadState('networkidle', { timeout });
}

/**
 * Wait for an element to be visible with custom timeout
 */
export async function waitForVisible(
  locator: Locator,
  timeout: number = 10000
): Promise<void> {
  await locator.waitFor({ state: 'visible', timeout });
}

/**
 * Wait for an element to be hidden with custom timeout
 */
export async function waitForHidden(
  locator: Locator,
  timeout: number = 10000
): Promise<void> {
  await locator.waitFor({ state: 'hidden', timeout });
}

/**
 * Retry an action until it succeeds or timeout
 */
export async function retryAction<T>(
  actionFn: () => Promise<T>,
  options: {
    maxAttempts?: number;
    interval?: number;
    errorMessage?: string;
  } = {}
): Promise<T> {
  const {
    maxAttempts = 3,
    interval = 1000,
    errorMessage = 'Action failed after max attempts',
  } = options;

  let lastError: Error | undefined;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await actionFn();
    } catch (error) {
      lastError = error as Error;
      if (attempt < maxAttempts) {
        await new Promise((resolve) => setTimeout(resolve, interval));
      }
    }
  }

  throw new Error(`${errorMessage}: ${lastError?.message}`);
}
