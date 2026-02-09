---
paths:
  - "frontend/**/*"
---

# Frontend Testing Requirements

## Playwright E2E Test Updates

**CRITICAL: When modifying or adding frontend code, you MUST also update or add corresponding Playwright E2E tests.**

### Requirements

- **New components/pages**: Create corresponding Page Object classes in `frontend/e2e/pages/` and test cases in `frontend/e2e/tests/`
- **Component modifications**: Update affected test cases to reflect the changes
- **UI changes**: Update selectors in Page Object classes to match new DOM structure, text, or attributes
- **Form/interaction changes**: Update test cases to verify the new behavior
- **Modal/dialog/toast additions**: Include accessibility attributes (`role`, `aria-*`) and update tests

### Test Execution

- Local: `task test:e2e`
- CI: GitHub Actions automatically runs E2E tests on PRs

### Test File Locations

- **Page Objects**: `frontend/e2e/pages/`
- **Test Cases**: `frontend/e2e/tests/`
- **Fixtures**: `frontend/e2e/fixtures/`

### Best Practices

- Use Page Object Pattern to keep tests maintainable
- Avoid `waitForTimeout()` - rely on Playwright's auto-waiting
- Use `.first()` on locators to avoid strict mode violations when multiple matches exist
- Add accessibility attributes to interactive elements for better test reliability
- Use `main h1` instead of just `h1` to target content area specifically
