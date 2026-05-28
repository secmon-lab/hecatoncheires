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

The fastest way to try Hecatoncheires locally (in-memory backend, no auth) is in
[docs/getting_started.md](docs/getting_started.md). In short:

1. Create a `config.toml` file with your field definitions (see [Configuration](docs/configuration.md) and [examples/config.toml](examples/config.toml))
2. Run the server with the memory backend:
   ```bash
   go run . serve --repository-backend=memory --config=config.toml --no-auth=U000000000 --addr=:8080
   ```
3. Access the web UI at `http://localhost:8080`

For a production deployment (Firestore, Cloud Storage, LLM provider, Slack
credentials), see [docs/deployment.md](docs/deployment.md).

## Documentation

Full documentation lives in [docs/](docs/README.md), organized by audience.

| Document | Description |
|----------|-------------|
| [Documentation index](docs/README.md) | Reading paths by audience |
| [Concepts](docs/concepts.md) | Core concepts and glossary |
| [Getting Started](docs/getting_started.md) | Run locally in minutes |
| [Deployment](docs/deployment.md) | Production deployment overview |
| [Configuration](docs/configuration.md) | `config.toml` complete reference |
| [CLI Reference](docs/cli.md) | Subcommands, flags, and environment variables |
| [Slack Integration](docs/slack.md) | Slack App setup (OAuth, Events, Interactivity, Slash) |
| [Integrations](docs/integrations.md) | Notion and GitHub |
| [User Guide](docs/user_guide.md) | End-user Slack workflows |
| [Operations](docs/operations.md) | Observability, runbook, backup |
| [Developing](docs/develop/README.md) | Architecture and contributor guide |

## Development

### Prerequisites

- Go 1.21+
- Node.js 18+ (for frontend)
- Corepack-managed pnpm (see below; the version is pinned via the `packageManager` field in `frontend/package.json`)
- Google Cloud Firestore

#### pnpm via Corepack

This repo pins the pnpm version in `frontend/package.json` (`packageManager` field).
Enable Corepack once on your machine and it will automatically install the right pnpm:

```bash
corepack enable
```

Do NOT install pnpm globally with `npm install -g pnpm` — that bypasses the pin
and is the most common cause of the lockfile being unexpectedly rewritten when
you run e2e or build commands.

If you intentionally want to update dependencies, run `pnpm install` inside
`frontend/` on its own and commit the resulting `pnpm-lock.yaml` change.
Day-to-day commands (build, e2e, etc.) use `--frozen-lockfile` and will fail
fast if the lockfile is out of sync rather than silently rewriting it.

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
# Install dependencies (only when you want to update them)
cd frontend
pnpm install
pnpm run test:e2e

# Or use the task command from the project root. This runner installs with
# --frozen-lockfile and will refuse to silently rewrite pnpm-lock.yaml.
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
