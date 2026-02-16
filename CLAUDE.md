# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Hecatoncheires is an AI-native project/case management platform built with Go and React. It provides a customizable GraphQL API with support for AI-powered analysis and automation. The system allows defining custom fields via TOML configuration files, making it adaptable to various use cases including security risk management, task management, and incident tracking.

## Common Development Commands

### Building and Testing
- `task` - Run default task (GraphQL code generation)
- `task build` - Build the complete application (frontend + backend)
- `task build:frontend` - Build frontend only
- `task graphql` - Generate GraphQL code from schema
- `task run` - Build and run the server
- `task dev:frontend` - Run frontend development server
- `go build` - Build the main binary
- `go test ./...` - Run all tests
- `go test ./pkg/path/to/package` - Run tests for specific package

### Code Generation
- `go tool gqlgen generate` - Generate GraphQL resolvers and types from schema
- Mock generation planned for future when more interfaces are defined

## Important Development Guidelines

### Error Handling
- Use `github.com/m-mizutani/goerr/v2` for error handling
- Must wrap errors with `goerr.Wrap` to maintain error context
- Add helpful variables with `goerr.V` for debugging
- **NEVER silently swallow errors** — returning a default/empty value while discarding the error (e.g., `return emptyResult, nil` in an `if err != nil` block) is strictly prohibited. Errors MUST always be propagated to the caller via `goerr.Wrap` or returned directly. This applies to ALL contexts including GraphQL resolvers — do not justify swallowing errors with "graceful degradation" or "it's just auxiliary data". If an operation fails, the caller must know about it
- **NEVER check error messages using `strings.Contains(err.Error(), ...)`**
- **ALWAYS use `errors.Is(err, targetErr)` or `errors.As(err, &target)` for error type checking**
- Error discrimination must be done by error types, not by parsing error messages

### Testing Best Practices
- ALWAYS write tests for ALL code you create. This is NON-NEGOTIABLE.
- Writing code without tests is UNACCEPTABLE.
- Use standard Go testing package
- Use Memory repository from `pkg/repository/memory` for repository testing
- Test both memory and firestore implementations when applicable
- Every function, method, and handler MUST have corresponding tests
- Tests must be written BEFORE declaring the task complete

### Code Visibility
- Do not expose unnecessary methods, variables and types
- Use `export_test.go` to expose items needed only for testing
- **NEVER place default values inside internal/private functions**
  - Default values should be controlled at the caller's level (e.g., CLI flags, configuration)
  - Internal functions should receive all necessary parameters from their callers
  - This ensures configurability and avoids hidden magic values

## Architecture

### Core Structure
The application follows Domain-Driven Design (DDD) with clean architecture:

- `pkg/cli/` - CLI commands and configuration
- `pkg/controller/` - Interface adapters
  - `graphql/` - GraphQL resolvers
  - `http/` - HTTP server and routing
- `pkg/domain/` - Domain layer
  - `interfaces/` - Repository and service interfaces
  - `model/` - Domain models (IoC data structures)
- `pkg/repository/` - Data persistence implementations
  - `firestore/` - Firestore backend
  - `memory/` - In-memory backend (testing/development)
- `pkg/usecase/` - Application use cases orchestrating domain operations
- `pkg/utils/` - Shared utilities (logging, etc.)
- `frontend/` - React TypeScript application
- `graphql/` - GraphQL schema definitions

### Key Components

#### GraphQL API
- Schema-first design using gqlgen
- GraphQL playground available at `/graphiql` (configurable)
- Type-safe resolvers in `pkg/controller/graphql/`

#### Frontend
- React with TypeScript
- Vite for development and building
- pnpm for package management (faster and more efficient than npm)
- Apollo Client for GraphQL integration
- Embedded into Go binary via `//go:embed`
- Development mode: Hot reload on port 5173
- Production mode: Served from embedded files

##### Frontend CSS Styling Guidelines
**NEVER hardcode color values, spacing, or sizes in CSS files.** Always use CSS variables defined in `frontend/src/styles/global.css`.

**Colors - Always use semantic variables:**
- Borders: `var(--border-default)`, `var(--border-light)`, `var(--border-medium)`, `var(--border-hover)`, `var(--border-strong)`
- Backgrounds: `var(--bg-paper)`, `var(--bg-subtle)`, `var(--bg-muted)`, `var(--bg-highlight)`
- Text: `var(--text-heading)`, `var(--text-body)`, `var(--text-muted)`, `var(--text-label)`
- Status: `var(--color-error)`, `var(--color-success)`, `var(--color-warning)`, `var(--color-info)`
- Primary: `var(--color-primary)`, `var(--color-primary-light)`, `var(--color-primary-dark)`

**Spacing - Always use spacing variables:**
- `var(--spacing-xs)` (4px), `var(--spacing-sm)` (8px), `var(--spacing-md-sm)` (12px)
- `var(--spacing-md-lg)` (14px), `var(--spacing-md)` (16px), `var(--spacing-lg)` (24px)
- `var(--spacing-xl)` (32px), `var(--spacing-xxl)` (48px)

**Units - Use rem for responsiveness:**
- Convert pixel values to rem (1rem = 16px)
- Examples: `20px` → `1.25rem`, `10px` → `0.625rem`
- Exception: 1px borders can remain as px

**Bad examples (DO NOT DO THIS):**
```css
border: 1px solid #E5E7EB;  /* Hardcoded color */
padding: 14px 16px;         /* Hardcoded spacing */
right: 20px;                /* Hardcoded size */
```

**Good examples:**
```css
border: 1px solid var(--border-default);
padding: var(--spacing-md-lg) var(--spacing-md);
right: 1.25rem;
```

#### Storage Backends
- **Firestore**: Production-ready persistent storage
- **Memory**: In-memory storage for testing and development
- Repository pattern allows easy switching between backends
- Interface defined in `pkg/domain/interfaces/`

##### Firestore Index Policy
- **CRITICAL: Firestore index updates are PROHIBITED in principle**
- When implementing new queries or batch operations:
  - Use existing indexes whenever possible
  - For batch operations, prefer parallel individual queries over complex queries requiring new indexes
  - If a feature absolutely requires a new index, consult with the team first
  - Example: Instead of `Where("case_id", "in", caseIDs).OrderBy(...)` (needs index), use parallel `Where("case_id", "==", caseID).OrderBy(...)` calls
- Test queries locally to ensure they work with existing indexes before deployment
- This policy prevents index management overhead and ensures queries remain simple and maintainable

##### Firestore Naming Policy
- **NEVER use underscore-joined (`_`) naming to encode multiple semantics into a single document/collection name**
  - Bad: `risk_case_counter`, `tenant1_cases`, `prefix_collectionName`
  - This pattern is fragile (ambiguous parsing if IDs contain underscores) and hard to maintain
- **Use Firestore subcollections to represent hierarchical relationships**
  - Good: `tenants/{tenantID}/counters/case` instead of `counters/risk_case_counter`
  - Subcollections naturally express parent-child relationships and are the idiomatic Firestore pattern
- The existing `collectionPrefix` mechanism is a legacy pattern; for new features, prefer subcollections

### Application Modes
- `serve` - HTTP server mode with GraphQL API and frontend

### Future Features (Planned)
The following features are planned but not yet implemented:
- Enhanced AI-powered case analysis and assessment
- Advanced search and query capabilities with full-text search
- Dashboard analytics and visualizations
- Export and integration features (CSV, JSON, webhooks)
- Additional source integrations (GitHub, Jira, etc.)
- Real-time collaboration features
- Advanced workflow automation

## Configuration

The application is configured via CLI flags or environment variables:

- `HECATONCHEIRES_ADDR` - HTTP server address (default: `:8080`)
- `HECATONCHEIRES_GRAPHIQL` - Enable GraphiQL playground (default: `true`)
- Logger configuration (format, level, output destination)

## Testing

Test files follow Go conventions (`*_test.go`). The codebase includes:
- Unit tests for individual components
- Integration tests with repository implementations
- Repository tests use both memory and firestore backends for verification

## Restrictions and Rules

### Directory

- When you are mentioned about `tmp` directory, you SHOULD NOT see `/tmp`. You need to check `./tmp` directory from root of the repository.

### Exposure policy

In principle, do not trust developers who use this library from outside

- Do not export unnecessary methods, structs, and variables
- Assume that exposed items will be changed. Never expose fields that would be problematic if changed
- Use `export_test.go` for items that need to be exposed for testing purposes

### Check

When making changes, before finishing the task, always:
- **WRITE TESTS FIRST - This is MANDATORY, not optional**
- Run `go vet ./...`, `go fmt ./...` to format the code
- Run `golangci-lint run ./...` to check lint error
- Run `gosec -exclude-generated -quiet ./...` to check security issue
- Run `zenv go test ./...` to ensure ALL tests pass
- If frontend files were changed: Run `pnpm test` in `frontend/` to execute Vitest unit tests
- Verify test coverage for your changes - EVERY new function/method MUST be tested

### Language

All comment and character literal in source code must be in English

### Testing

- Test files should have `package {name}_test`. Do not use same package name
- Test file name convention is: `xyz.go` → `xyz_test.go`. Other test file names (e.g., `xyz_e2e_test.go`) are not allowed.
- Repository Tests Location:
  - NEVER create test files in `pkg/repository/firestore/` or `pkg/repository/memory/` subdirectories
  - ALL repository tests MUST be placed directly in `pkg/repository/*_test.go`
  - Use `runRepositoryTest()` helper to test against both memory and firestore implementations
- Repository Tests Best Practices:
  - Always use random IDs (e.g., using `time.Now().UnixNano()`) to avoid test conflicts
  - Never use hardcoded IDs like "msg-001", "user-001" as they cause test failures when running in parallel
  - Always verify ALL fields of returned values, not just checking for nil/existence
  - Compare expected values properly - don't just check if something exists, verify it matches what was saved
  - For timestamp comparisons, use tolerance (e.g., `< time.Second`) to account for storage precision
- Test Skip Policy:
  - **NEVER use `t.Skip()` for anything other than missing environment variables**
  - If a test requires infrastructure (like Firestore index), fix the infrastructure, don't skip the test
  - If a feature is not implemented, write the code, don't skip the test
  - The only acceptable skip pattern: checking for missing environment variables at the beginning of a test

### Test File Checklist (Use this EVERY time)
Before creating or modifying tests:
1. ✓ Is there a corresponding source file for this test file?
2. ✓ Does the test file name match exactly? (`xyz.go` → `xyz_test.go`)
3. ✓ Are all tests for a source file in ONE test file?
4. ✓ No standalone feature/e2e/integration test files?
5. ✓ For repository tests: placed in `pkg/repository/*_test.go`, NOT in firestore/ or memory/ subdirectories?
