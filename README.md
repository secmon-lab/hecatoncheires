# Hecatoncheires

An AI-native, customizable project/case management platform with Slack integration.

## Overview

Hecatoncheires is a flexible project and case management system that adapts to your workflow through configuration files. Define custom fields, integrate with Slack, and leverage AI for knowledge extraction and analysis.

## Key Features

- **Customizable Fields**: Define project-specific fields via TOML configuration
- **AI-Powered**: Automatic knowledge extraction from various sources (Notion, Slack, etc.)
- **Slack Integration**: Native Slack channel and message integration
- **GraphQL API**: Type-safe, flexible API for frontend and integrations
- **Field Types**: Support for text, numbers, dates, URLs, user references, and select fields with metadata

## Quick Start

1. Create a `config.toml` file with your field definitions (see [Configuration Guide](docs/config.md) and [examples/config.toml](examples/config.toml))
2. Set up Firestore and Slack credentials (see [Authentication](docs/auth.md) and [Slack Integration](docs/slack.md))
3. Run the server:
   ```bash
   hecatoncheires serve --firestore-project-id=YOUR_PROJECT_ID
   ```
4. Access the web UI at `http://localhost:8080`

## Documentation

| Document | Description |
|----------|-------------|
| [Configuration Guide](docs/config.md) | TOML config file, CLI flags, field types, and validation rules |
| [Authentication](docs/auth.md) | Slack OAuth setup and no-auth development mode |
| [Slack Integration](docs/slack.md) | Events API, webhooks, and channel management |

## Development

### Prerequisites

- Go 1.21+
- Node.js and pnpm (for frontend)
- Google Cloud Firestore

### Building

```bash
task build           # Build complete application (frontend + backend)
task graphql         # Generate GraphQL code from schema
task dev:frontend    # Run frontend development server
```

### Testing

#### Unit Tests

Run Go unit tests:
```bash
go test ./...
```

#### E2E Tests

Hecatoncheires includes end-to-end tests using Playwright to verify the complete application workflow.

**Prerequisites:**
- Node.js 18+ and pnpm
- The backend server must be running with `--repository-backend=memory` for testing

**Run E2E tests:**

```bash
# Install dependencies and run tests
cd frontend
pnpm install
pnpm run test:e2e

# Or use the task command from the project root
task test:e2e
```

**Other E2E test commands:**

```bash
# Run tests with UI mode (interactive)
task test:e2e:ui

# Run tests in headed mode (show browser)
task test:e2e:headed

# Run tests in debug mode
task test:e2e:debug

# Show test report
task test:e2e:report
```

**Manual E2E test setup:**

If you want to run tests manually with a running server:

1. Start the backend server in memory mode:
   ```bash
   go run . serve \
     --repository-backend=memory \
     --config=frontend/e2e/fixtures/config.test.toml \
     --no-auth=U000000000 \
     --addr=:8080
   ```

2. In another terminal, run the E2E tests:
   ```bash
   cd frontend
   BASE_URL=http://localhost:8080 pnpm run test:e2e
   ```

**CI/CD Integration:**

E2E tests run automatically on pull requests via GitHub Actions. Test results and screenshots are uploaded as artifacts when tests fail.
