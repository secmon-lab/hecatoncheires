# Configuration Guide

Hecatoncheires is configured through a combination of a TOML configuration file and CLI flags (or environment variables).

## Table of Contents

1. [Configuration File (config.toml)](#configuration-file-configtoml)
2. [CLI Flags & Environment Variables](#cli-flags--environment-variables)
3. [Labels](#labels)
4. [Field Definitions](#field-definitions)
5. [Field Types](#field-types)
6. [Options (for select / multi-select)](#options-for-select--multi-select)
7. [Validation Rules](#validation-rules)
8. [Complete Example](#complete-example)

---

## Configuration File (config.toml)

The application requires a TOML configuration file at startup. This file defines custom fields for cases and display labels for entities.

- Default path: `./config.toml`
- Override with `--config` flag or `HECATONCHEIRES_CONFIG` environment variable
- The file **must exist** at startup; a missing file causes an error

### Basic Structure

```toml
# Entity display labels (optional)
[labels]
case = "Risk"

# Custom field definitions
[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
options = [
  { id = "high", name = "High" },
  { id = "low", name = "Low" },
]
```

---

## CLI Flags & Environment Variables

All flags can also be set via environment variables. Environment variables use the prefix `HECATONCHEIRES_` with uppercase, underscore-separated names (e.g., `--log-level` becomes `HECATONCHEIRES_LOG_LEVEL`).

CLI flags take precedence over environment variables.

### Global Flags (Logger)

Available for all commands.

| Flag | Alias | Env Var | Default | Description |
|------|-------|---------|---------|-------------|
| `--log-level` | `-l` | `HECATONCHEIRES_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `-f` | `HECATONCHEIRES_LOG_FORMAT` | `console` | Log format: `console`, `json` |
| `--log-output` | `-o` | `HECATONCHEIRES_LOG_OUTPUT` | `stdout` | Log output: `stdout`, `stderr`, `-`, or a file path |
| `--log-quiet` | `-q` | `HECATONCHEIRES_LOG_QUIET` | `false` | Quiet mode (disables all log output) |
| `--log-stacktrace` | `-s` | `HECATONCHEIRES_LOG_STACKTRACE` | `true` | Show stacktrace in console format |

### Serve Command Flags

The `serve` command (alias: `s`) starts the HTTP server.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--addr` | `HECATONCHEIRES_ADDR` | `:8080` | No | HTTP server address and port |
| `--base-url` | `HECATONCHEIRES_BASE_URL` | - | Yes\* | Application base URL (e.g., `https://your-domain.com`). No trailing slash |
| `--graphiql` | `HECATONCHEIRES_GRAPHIQL` | `true` | No | Enable GraphiQL playground at `/graphiql` |
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | No | Path to TOML configuration file |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | **Yes** | Google Cloud Firestore project ID |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| `--notion-api-token` | `HECATONCHEIRES_NOTION_API_TOKEN` | - | No | Notion API token for Source integration |
| `--no-auth` | `HECATONCHEIRES_NO_AUTH` | - | No | Slack user ID for no-auth mode (development only) |
| `--slack-client-id` | `HECATONCHEIRES_SLACK_CLIENT_ID` | - | Yes\* | Slack OAuth client ID |
| `--slack-client-secret` | `HECATONCHEIRES_SLACK_CLIENT_SECRET` | - | Yes\* | Slack OAuth client secret |
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | No\*\* | Slack Bot User OAuth Token (`xoxb-...`) |
| `--slack-signing-secret` | `HECATONCHEIRES_SLACK_SIGNING_SECRET` | - | No\*\*\* | Slack signing secret for webhook verification |
| `--slack-channel-prefix` | `HECATONCHEIRES_SLACK_CHANNEL_PREFIX` | `risk` | No | Prefix for auto-created Slack channel names |

\* Required for OAuth mode. Alternatively, use `--no-auth` with `--slack-bot-token` for development.

\*\* Required when using `--no-auth`. Also enables user avatar display and Slack user refresh worker.

\*\*\* Required only to enable Slack webhook integration. Without this, webhook endpoints are not registered.

### Migrate Command Flags

The `migrate` command (alias: `m`) manages Firestore indexes.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | **Yes** | Google Cloud Firestore project ID |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| `--dry-run` | - | `false` | No | Preview migration changes without applying |

### Authentication Modes

The serve command supports two authentication modes:

**OAuth Mode (Production)**
```bash
hecatoncheires serve \
  --base-url=https://your-domain.com \
  --slack-client-id=YOUR_CLIENT_ID \
  --slack-client-secret=YOUR_CLIENT_SECRET \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --firestore-project-id=YOUR_PROJECT_ID
```

**No-Auth Mode (Development)**
```bash
hecatoncheires serve \
  --no-auth=U1234567890 \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --firestore-project-id=YOUR_PROJECT_ID
```

`--no-auth` and `--slack-client-id`/`--slack-client-secret` are mutually exclusive. If both are provided, `--no-auth` takes precedence.

---

## Labels

The `[labels]` section customizes entity display names in the UI:

```toml
[labels]
case = "Risk"        # Default: "Case"
```

| Key | Default | Description |
|-----|---------|-------------|
| `case` | `Case` | Display name for the primary entity |

If omitted or empty, the default values are used.

---

## Field Definitions

Each field is defined as an element of the `[[fields]]` array:

```toml
[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
description = "Overall severity assessment"
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique identifier. Must match `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `name` | string | **Yes** | Display name shown in the UI |
| `type` | string | **Yes** | Field type (see [Field Types](#field-types)) |
| `required` | boolean | No | Whether the field is required (default: `false`) |
| `description` | string | No | Help text shown in the UI |

### Field ID Format

Field IDs must be lowercase alphanumeric with optional hyphens:

- Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Must be unique across all fields in the configuration

| Example | Valid | Reason |
|---------|-------|--------|
| `category` | Yes | Simple lowercase |
| `risk-level` | Yes | Hyphen-separated |
| `my-field-123` | Yes | With numbers |
| `MyField` | **No** | Uppercase not allowed |
| `category_id` | **No** | Underscores not allowed |
| `field.name` | **No** | Dots not allowed |
| `-leading` | **No** | Cannot start with hyphen |

---

## Field Types

### `text`

Single-line text input.

```toml
[[fields]]
id = "description"
name = "Description"
type = "text"
```

### `number`

Numeric input.

```toml
[[fields]]
id = "score"
name = "Risk Score"
type = "number"
```

### `select`

Single selection from a predefined list of options. Requires at least one option.

```toml
[[fields]]
id = "priority"
name = "Priority"
type = "select"
required = true
options = [
  { id = "high", name = "High" },
  { id = "medium", name = "Medium" },
  { id = "low", name = "Low" },
]
```

### `multi-select`

Multiple selections from a predefined list of options. Requires at least one option.

```toml
[[fields]]
id = "tags"
name = "Tags"
type = "multi-select"
options = [
  { id = "urgent", name = "Urgent" },
  { id = "review-needed", name = "Review Needed" },
]
```

### `user`

Single Slack user reference. Rendered with user avatar in the UI. Requires Slack integration to be configured.

```toml
[[fields]]
id = "assignee"
name = "Assignee"
type = "user"
```

### `multi-user`

Multiple Slack user references. Requires Slack integration.

```toml
[[fields]]
id = "stakeholders"
name = "Stakeholders"
type = "multi-user"
```

### `date`

Date picker input.

```toml
[[fields]]
id = "deadline"
name = "Deadline"
type = "date"
```

### `url`

URL input with validation.

```toml
[[fields]]
id = "reference"
name = "Reference URL"
type = "url"
```

### Summary

| Type | Description | Requires Options |
|------|-------------|-----------------|
| `text` | Single-line text input | No |
| `number` | Numeric input | No |
| `select` | Single selection from options | **Yes** |
| `multi-select` | Multiple selections from options | **Yes** |
| `user` | Single Slack user reference | No |
| `multi-user` | Multiple Slack user references | No |
| `date` | Date picker | No |
| `url` | URL input | No |

---

## Options (for select / multi-select)

Fields of type `select` or `multi-select` must define at least one option using the `options` property.

### Option Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique identifier within the field. Same format as field ID |
| `name` | string | **Yes** | Display name |
| `description` | string | No | Description of this option |
| `color` | string | No | Color name or hex code (e.g., `red`, `#E53E3E`) |
| `metadata` | table | No | Arbitrary key-value pairs |

Option IDs must be unique within their parent field and follow the same format as field IDs (`^[a-z0-9]+(-[a-z0-9]+)*$`).

### Metadata

The `metadata` property allows attaching arbitrary key-value data to an option. Values can be strings, numbers, or booleans.

```toml
[[fields]]
id = "severity"
name = "Severity"
type = "select"
options = [
  { id = "critical", name = "Critical", metadata = { description = "Requires immediate executive attention", color = "red", score = 5, escalation_required = true } },
  { id = "low", name = "Low", metadata = { description = "Minimal impact", score = 1 } },
]
```

Use cases for metadata:
- **Scoring**: Attach numeric scores for risk calculation
- **Categorization**: Add descriptions and colors for UI display
- **Workflow**: Flag options that trigger specific behaviors

---

## Validation Rules

The configuration file is validated at startup. The following rules are enforced:

| Rule | Error |
|------|-------|
| Configuration file must exist at the specified path | `ErrConfigNotFound` |
| All field IDs must match `^[a-z0-9]+(-[a-z0-9]+)*$` | `ErrInvalidFieldID` |
| All field names must be non-empty | `ErrMissingName` |
| Field type must be one of the 8 supported types | `ErrInvalidFieldType` |
| Field IDs must be unique across the entire configuration | `ErrDuplicateFieldID` |
| `select` and `multi-select` fields must have at least one option | `ErrMissingOptions` |
| All option IDs must match the same pattern as field IDs | `ErrInvalidFieldID` |
| All option names must be non-empty | `ErrMissingName` |
| Option IDs must be unique within their parent field | `ErrDuplicateOptionID` |

If any validation fails, the application exits with a descriptive error message including the field ID and context.

---

## Complete Example

The following example configures Hecatoncheires as a security risk management system. See also [examples/config.toml](../examples/config.toml).

```toml
# Customize entity labels to match your domain
[labels]
case = "Risk"

# Category field: multi-select with metadata
[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true

[[fields.options]]
id = "data-breach"
name = "Data Breach"

  [fields.options.metadata]
  description = "Risk of personal or confidential information leakage"
  color = "red"

[[fields.options]]
id = "system-failure"
name = "System Failure"

  [fields.options.metadata]
  description = "Risk of system or service downtime and failures"
  color = "orange"

# Likelihood field: select with scoring
[[fields]]
id = "likelihood"
name = "Likelihood"
type = "select"
required = true

[[fields.options]]
id = "low"
name = "Low"

  [fields.options.metadata]
  description = "Unlikely to occur (a few times per year)"
  score = 2

[[fields.options]]
id = "high"
name = "High"

  [fields.options.metadata]
  description = "Likely to occur (about once per week)"
  score = 4

# Simple fields without options
[[fields]]
id = "specific-impact"
name = "Specific Impact"
type = "text"

[[fields]]
id = "deadline"
name = "Deadline"
type = "date"

[[fields]]
id = "reference-url"
name = "Reference URL"
type = "url"
```

---

## See Also

- [Authentication Guide](./auth.md) - Slack OAuth setup and no-auth development mode
- [Slack Integration Guide](./slack.md) - Events API, webhooks, and channel management
- [Example Configuration](../examples/config.toml) - Complete example for security risk management
