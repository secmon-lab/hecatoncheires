/**
 * Test data definitions for E2E tests
 */

export interface TestCase {
  title: string;
  description: string;
  fields?: Record<string, any>;
}

export interface TestAction {
  title: string;
  description: string;
}

export const testCases: TestCase[] = [
  {
    title: 'E2E Test Case 1',
    description: 'This is a test case created by Playwright E2E test',
    fields: {
      category: 'bug',
      priority: 'high',
    },
  },
  {
    title: 'E2E Test Case 2',
    description: 'Another test case for verification',
    fields: {
      category: 'feature',
      priority: 'medium',
    },
  },
];

export const testActions: TestAction[] = [
  {
    title: 'E2E Test Action 1',
    description: 'This is a test action created by Playwright E2E test',
  },
  {
    title: 'E2E Test Action 2',
    description: 'Another test action for verification',
  },
];

export const TEST_WORKSPACE_ID = 'test';
export const TEST_USER_ID = 'U000000000';
