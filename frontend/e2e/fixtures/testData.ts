/**
 * Shared constants for E2E tests.
 */

// The workspace seeded by config.test.toml (see frontend/scripts/e2e.sh).
export const TEST_WORKSPACE_ID = 'test';

// The user the backend auto-authenticates as under `--no-auth=U000000000`
// (see frontend/scripts/e2e.sh). Kept in sync with that flag so assertions
// about the acting user have a single source of truth.
export const TEST_USER_ID = 'U000000000';
