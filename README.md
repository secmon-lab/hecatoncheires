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

```bash
go test ./...
```
