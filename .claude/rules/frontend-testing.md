---
paths:
  - "frontend/**/*"
---

# Frontend Testing Requirements

## Vitest Unit Tests

**CRITICAL: When modifying or adding frontend logic (utility functions, hooks, parsers, etc.), you MUST write corresponding Vitest unit tests.**

### Requirements

- **New utility functions**: Create `*.test.ts` file alongside the source file (e.g., `notion.ts` → `notion.test.ts`)
- **Modified logic**: Update affected unit tests to reflect the changes
- **Test framework**: Use Vitest with `import { describe, it, expect } from 'vitest'`

### Test Execution

**MANDATORY: After any frontend code change, you MUST run both Vitest unit tests AND E2E tests.**

- Run `pnpm test` in `frontend/` to execute Vitest unit tests
- Run `task test:e2e` to execute Playwright E2E tests
- Both MUST pass before considering the task complete

### Test File Locations (Unit Tests)

- Unit test files live alongside their source files: `src/utils/notion.test.ts`, `src/hooks/useXxx.test.ts`, etc.
- Configuration: `vite.config.ts` (test section)

## Playwright E2E Tests

**CRITICAL: When modifying or adding frontend components/pages, you MUST also update or add corresponding Playwright E2E tests.**

### Requirements

- **New components/pages**: Create corresponding Page Object classes in `frontend/e2e/pages/` and test cases in `frontend/e2e/tests/`
- **Component modifications**: Update affected test cases to reflect the changes
- **UI changes**: Update selectors in Page Object classes to match new DOM structure, text, or attributes
- **Form/interaction changes**: Update test cases to verify the new behavior
- **Modal/dialog/toast additions**: Include accessibility attributes (`role`, `aria-*`) and update tests

### Test File Locations (E2E)

- **Page Objects**: `frontend/e2e/pages/`
- **Test Cases**: `frontend/e2e/tests/`
- **Fixtures**: `frontend/e2e/fixtures/`

### Best Practices

- Use Page Object Pattern to keep tests maintainable
- Avoid `waitForTimeout()` - rely on Playwright's auto-waiting
- Use `.first()` on locators to avoid strict mode violations when multiple matches exist
- Add accessibility attributes to interactive elements for better test reliability
- Use `main h1` instead of just `h1` to target content area specifically
