# Configuration Guide

Hecatoncheires is configured through a combination of a TOML configuration file and CLI flags (or environment variables).

## Table of Contents

1. [Configuration File (config.toml)](#configuration-file-configtoml)
2. [CLI Flags & Environment Variables](#cli-flags--environment-variables)
3. [Workspace Section](#workspace-section)
4. [Labels](#labels)
5. [Field Definitions](#field-definitions)
6. [Field Types](#field-types)
7. [Options (for select / multi-select)](#options-for-select--multi-select)
8. [Slack Section](#slack-section)
9. [Compile Section](#compile-section)
10. [Assist Section](#assist-section)
11. [Action Section](#action-section)
12. [Validation Rules](#validation-rules)
13. [Complete Example](#complete-example)

---

## Configuration File (config.toml)

The application requires a TOML configuration file at startup. This file defines custom fields for cases and display labels for entities.

- Default path: `./config.toml`
- Override with `--config` flag or `HECATONCHEIRES_CONFIG` environment variable
- The file **must exist** at startup; a missing file causes an error

### Basic Structure

```toml
# Workspace configuration (required)
[workspace]
id = "risk"
name = "Risk Management"

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

# Slack integration (optional)
[slack]
channel_prefix = "risk"

# AI compile configuration (optional)
[compile]
prompt = "Extract key information from the source material."

# AI assist configuration (optional)
[assist]
prompt = "Check action deadlines and follow up on pending items."
language = "Japanese"
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
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Yes | Google Cloud Firestore project ID |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| `--notion-api-token` | `HECATONCHEIRES_NOTION_API_TOKEN` | - | No | Notion API token for Source integration |
| `--no-auth` | `HECATONCHEIRES_NO_AUTH` | - | No | Slack user ID for no-auth mode (development only) |
| `--slack-client-id` | `HECATONCHEIRES_SLACK_CLIENT_ID` | - | Yes\* | Slack OAuth client ID |
| `--slack-client-secret` | `HECATONCHEIRES_SLACK_CLIENT_SECRET` | - | Yes\* | Slack OAuth client secret |
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | No\*\* | Slack Bot User OAuth Token (`xoxb-...`) |
| `--slack-signing-secret` | `HECATONCHEIRES_SLACK_SIGNING_SECRET` | - | No\*\*\* | Slack signing secret for webhook verification |
| `--slack-channel-prefix` | `HECATONCHEIRES_SLACK_CHANNEL_PREFIX` | `risk` | No | Prefix for auto-created Slack channel names |
| `--github-app-id` | `HECATONCHEIRES_GITHUB_APP_ID` | - | No | GitHub App ID for GitHub Source integration |
| `--github-app-installation-id` | `HECATONCHEIRES_GITHUB_APP_INSTALLATION_ID` | - | No | GitHub App Installation ID |
| `--github-app-private-key` | `HECATONCHEIRES_GITHUB_APP_PRIVATE_KEY` | - | No | GitHub App private key (PEM string or file path) |
| `--llm-provider` | `HECATONCHEIRES_LLM_PROVIDER` | - | No\*\*\*\* | LLM provider: `openai`, `claude`, or `gemini`. Empty disables AI features |
| `--llm-model` | `HECATONCHEIRES_LLM_MODEL` | - | No | LLM model name (provider default if empty) |
| `--llm-openai-api-key` | `HECATONCHEIRES_LLM_OPENAI_API_KEY` | - | No\*\*\*\* | OpenAI API key (required when `--llm-provider=openai`) |
| `--llm-claude-api-key` | `HECATONCHEIRES_LLM_CLAUDE_API_KEY` | - | No\*\*\*\* | Anthropic Claude API key (used with direct Anthropic access) |
| `--llm-gemini-project-id` | `HECATONCHEIRES_LLM_GEMINI_PROJECT_ID` | - | No\*\*\*\* | Google Cloud project ID (Gemini, or Claude via Vertex AI) |
| `--llm-gemini-location` | `HECATONCHEIRES_LLM_GEMINI_LOCATION` | `global` | No | Google Cloud location for Gemini / Claude on Vertex AI |
| `--cloud-storage-bucket` | `HECATONCHEIRES_CLOUD_STORAGE_BUCKET` | - | Yes\*\*\*\*\* | Cloud Storage bucket holding agent thread session History/Trace blobs. See [agent-session.md](./agent-session.md) |
| `--cloud-storage-prefix` | `HECATONCHEIRES_CLOUD_STORAGE_PREFIX` | - | No | Optional object key prefix within the Cloud Storage bucket |

\* Required for OAuth mode. Alternatively, use `--no-auth` with `--slack-bot-token` for development.

\*\* Required when using `--no-auth`. Also enables user avatar display and Slack user refresh worker.

\*\*\* Required only to enable Slack webhook integration. Without this, webhook endpoints are not registered.

\*\*\*\* `--llm-provider` is optional for `serve` (AI features will be disabled if unset). When set, the matching provider credentials become required:
- `openai` → `--llm-openai-api-key`
- `claude` → either `--llm-claude-api-key` (direct Anthropic API) **or** `--llm-gemini-project-id` (Vertex AI). The two are mutually exclusive.
- `gemini` → `--llm-gemini-project-id` and `--llm-gemini-location`

\*\*\*\*\* Required whenever `--slack-bot-token` is configured. The agent that responds to Slack mentions persists per-thread conversation History and execution Trace into the bucket so follow-up mentions can resume the session. The service account needs **Storage Object Admin** on the bucket.

### Compile Command Flags

The `compile` command (alias: `c`) extracts knowledge from external sources using LLM.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--notion-api-token` | `HECATONCHEIRES_NOTION_API_TOKEN` | - | Yes | Notion API token for Source integration |
| `--since` | `HECATONCHEIRES_COMPILE_SINCE` | 24h ago | No | Process pages updated since this time (RFC3339 format) |
| `--workspace` | `HECATONCHEIRES_COMPILE_WORKSPACE` | - | No | Target workspace ID (if empty, process all workspaces) |
| `--base-url` | `HECATONCHEIRES_BASE_URL` | - | No | Base URL for the application |
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | No | Slack Bot Token for sending notifications |
| `--github-app-id` | `HECATONCHEIRES_GITHUB_APP_ID` | - | No | GitHub App ID for GitHub Source integration |
| `--github-app-installation-id` | `HECATONCHEIRES_GITHUB_APP_INSTALLATION_ID` | - | No | GitHub App Installation ID |
| `--github-app-private-key` | `HECATONCHEIRES_GITHUB_APP_PRIVATE_KEY` | - | No | GitHub App private key (PEM string or file path) |
| `--llm-provider` | `HECATONCHEIRES_LLM_PROVIDER` | - | Yes | LLM provider: `openai`, `claude`, or `gemini` |
| `--llm-model` | `HECATONCHEIRES_LLM_MODEL` | - | No | LLM model name (provider default if empty) |
| `--llm-openai-api-key` | `HECATONCHEIRES_LLM_OPENAI_API_KEY` | - | Cond. | OpenAI API key (required for `openai`) |
| `--llm-claude-api-key` | `HECATONCHEIRES_LLM_CLAUDE_API_KEY` | - | Cond. | Anthropic Claude API key (for `claude` direct API) |
| `--llm-gemini-project-id` | `HECATONCHEIRES_LLM_GEMINI_PROJECT_ID` | - | Cond. | Google Cloud project ID (for `gemini` or `claude` on Vertex AI) |
| `--llm-gemini-location` | `HECATONCHEIRES_LLM_GEMINI_LOCATION` | `global` | No | Google Cloud location |

### Migrate Command Flags

The `migrate` command (alias: `m`) manages Firestore indexes.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Yes | Google Cloud Firestore project ID |
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

## Workspace Section

The `[workspace]` section defines the workspace's identity and is **required** in each configuration file.

```toml
[workspace]
id = "risk"
name = "Risk Management"
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique workspace identifier. Must match `^[a-z0-9]+(-[a-z0-9]+)*$` and be at most 63 characters |
| `name` | string | No | Display name for the workspace. Defaults to `id` if omitted |

### Workspace ID Format

Workspace IDs must follow the same format as field IDs:

- Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Maximum length: 63 characters
- Must be unique across all workspaces

| Example | Valid | Reason |
|---------|-------|--------|
| `risk` | Yes | Simple lowercase |
| `security-team` | Yes | Hyphen-separated |
| `workspace-123` | Yes | With numbers |
| `MyWorkspace` | **No** | Uppercase not allowed |
| `workspace_1` | **No** | Underscores not allowed |
| `workspace.name` | **No** | Dots not allowed |

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
| `id` | string | **Yes** | Unique identifier. Must match `^[a-z][a-z0-9_]*$` |
| `name` | string | **Yes** | Display name shown in the UI |
| `type` | string | **Yes** | Field type (see [Field Types](#field-types)) |
| `required` | boolean | No | Whether the field is required (default: `false`) |
| `description` | string | No | Help text shown in the UI |

### Field ID Format

Field IDs must start with a lowercase letter and consist of lowercase letters,
digits, and underscores:

- Pattern: `^[a-z][a-z0-9_]*$`
- Must be unique across all fields in the configuration

> **Breaking change (Apr 2026)**: Field IDs and Option IDs no longer accept
> hyphens (`-`). The new pattern aligns with Go identifier rules so that
> Slack welcome message templates can reference custom field values via dot
> notation (e.g., `{{.Fields.risk_level}}`). Existing deployments that store
> hyphenated FieldIDs in Firestore must migrate the data themselves; the
> application provides no automatic migration. Workspace IDs are unaffected
> and continue to follow the legacy hyphen-separated format.

| Example | Valid | Reason |
|---------|-------|--------|
| `category` | Yes | Simple lowercase |
| `risk_level` | Yes | Underscore-separated |
| `my_field_123` | Yes | With numbers |
| `risk-level` | **No** | Hyphens are no longer allowed |
| `MyField` | **No** | Uppercase not allowed |
| `1category` | **No** | Cannot start with a digit |
| `field.name` | **No** | Dots not allowed |
| `_leading` | **No** | Cannot start with underscore |

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
  { id = "review_needed", name = "Review Needed" },
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

Option IDs must be unique within their parent field and follow the same format as field IDs (`^[a-z][a-z0-9_]*$`).

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

## Slack Section

The `[slack]` section customizes Slack integration settings. This section is optional.

```toml
[slack]
channel_prefix = "risk"
```

### Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `channel_prefix` | string | No | workspace ID | Prefix for auto-created Slack channel names |

When a case is created, Hecatoncheires can automatically create a Slack channel with the naming pattern: `{channel_prefix}-{case_number}`.

If `channel_prefix` is not specified, the workspace ID is used as the default prefix.

**Note:** The `--slack-channel-prefix` CLI flag can override this configuration for the entire serve command.

### Auto-Invite (`[slack.invite]`)

The `[slack.invite]` subsection configures automatic invitation of users and user group members to Slack channels when a case is created.

```toml
[slack.invite]
users = ["U12345678", "U87654321"]
groups = ["S0614TZR7", "@security-response"]
```

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `users` | string[] | No | `[]` | Slack user IDs to automatically invite to case channels |
| `groups` | string[] | No | `[]` | Slack user group IDs or `@`-prefixed handle names to resolve and invite |

**Group resolution:**
- If a group identifier starts with `@` (e.g., `@security-response`), it is treated as a handle name and resolved to a group ID via the `usergroups.list` API
- Otherwise (e.g., `S0614TZR7`), it is treated as a Slack user group ID and used directly
- Members of each group are fetched via `usergroups.users.list` and added to the invite list
- Group resolution failures are logged but do not prevent case creation
- Duplicate users (across direct users, group members, creator, and assignees) are automatically deduplicated

**Required bot scope:** `usergroups:read` (in addition to existing scopes)

### Welcome Messages (`welcome_messages`)

After the Slack channel is created and initial members are invited, Hecatoncheires can post a sequence of welcome messages to the channel. Each entry is a Go [`text/template`](https://pkg.go.dev/text/template) string that is rendered against the newly created Case.

```toml
[slack]
welcome_messages = [
  "<@{{.Case.ReporterID}}> Created Case `{{.Case.Title}}`.",
  """\
:rotating_light: Highlights
- Status: {{.Case.Status}}
- Severity: {{.Fields.severity.name}} ({{.Fields.severity.id}})
- Tags: {{range $i, $t := .Fields.tags.items}}{{if $i}}, {{end}}{{$t.name}}{{end}}
- Reporter: <@{{.Case.ReporterID}}>
- Assignees: {{range $i, $a := .Case.AssigneeIDs}}{{if $i}}, {{end}}<@{{$a}}>{{end}}
- Detail: {{.URL}}""",
]
```

**Behavior**

- Messages are sent in the order they appear in the array, after channel creation, member invitation, and bookmark addition.
- Templates are parsed at startup; an invalid template aborts the application start with a configuration error.
- A template that produces an empty string at runtime is skipped (useful for conditional messages with `{{if ...}}...{{end}}`).
- A failure to render or post a single message is logged via `errutil.Handle` and does **not** roll back case creation; subsequent messages still attempt to post.

**Available template variables**

| Variable | Type | Notes |
|----------|------|-------|
| `.Case.ID` | int64 | Case sequence number |
| `.Case.Title` | string | Case title |
| `.Case.Description` | string | Case description |
| `.Case.Status` | CaseStatus | Normalized status string |
| `.Case.ReporterID` | string | Slack user ID of the reporter |
| `.Case.AssigneeIDs` | []string | Slack user IDs of assignees |
| `.Case.SlackChannelID` | string | The freshly-created channel ID |
| `.Case.IsPrivate` | bool | Whether the case is private |
| `.Case.CreatedAt` | time.Time | Creation timestamp (UTC) |
| `.Workspace.ID` | string | Workspace identifier |
| `.Workspace.Name` | string | Workspace display name |
| `.Fields` | map[string]map[string]any | Custom field values keyed by Field ID — each entry exposes `id` and `name` (and `items` for multi-select) |
| `.URL` | string | Web UI Case URL when `--base-url` is configured (otherwise empty) |

**Field value structure**

Each Field is exposed as a sub-map with the following sub-keys:

| Sub-key | Description |
|---------|-------------|
| `id` | Raw stored value. For `select`, the option ID. For `multi-select`, an array of option IDs. For `text`/`number`/`date`/`url`/`user`/`multi-user`, the raw stored value. |
| `name` | Display name. For `select`/`multi-select`, the Option `name` from the schema (falls back to the ID when not found). For other field types, mirrors `id`. |
| `items` | `multi-select` only. Slice of `{id, name}` maps for iteration in templates. |

Examples:

```text
Severity: {{.Fields.severity.name}}                        ← "High"
Severity ID: {{.Fields.severity.id}}                        ← "high"
Tags: {{range $i, $t := .Fields.tags.items}}{{if $i}}, {{end}}{{$t.name}}{{end}}
Note: {{.Fields.note.id}}                                   ← bare text value
```

Slack mrkdwn syntax such as `<@USER_ID>` and `<#CHANNEL_ID>` is rendered as-is and expanded by Slack at delivery time.

### Private Case Channels

When a case is created with the **Private** flag enabled, the associated Slack channel is created as a **private channel** instead of a public one. This ensures that only invited members can view the channel content.

Private cases also track channel membership:
- Channel member IDs are stored on the case and used for access control — only channel members can view the case details, actions, knowledges, and assist logs associated with a private case.
- Members can be synced from the Slack channel via the **Sync** button on the case detail page or through the `syncCaseChannelUsers` GraphQL mutation.
- Bot users are automatically filtered out from the member list.

---

## Compile Section

The `[compile]` section configures the AI-powered knowledge compilation feature. This section is optional.

```toml
[compile]
prompt = "Extract key information from the source material and summarize it."
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `prompt` | string | No | Custom prompt for the LLM when compiling knowledge from external sources |

The `compile` command processes external data sources (Notion pages, GitHub PRs/Issues, etc.) to extract structured knowledge. The `prompt` field allows you to customize how the LLM should interpret and extract information from these sources.

If omitted, the system uses a default prompt optimized for general knowledge extraction.

---

## Assist Section

The `[assist]` section configures the AI-powered case assistance feature. This section is optional.

```toml
[assist]
prompt = "Check action deadlines and follow up on pending items."
language = "Japanese"
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `prompt` | string | No | Custom prompt for the AI assistant when analyzing cases |
| `language` | string | No | Preferred language for AI responses (e.g., "Japanese", "English") |

The `assist` command runs an AI agent that periodically reviews open cases and provides insights, reminders, and suggestions via Slack.

**Important:** If the `prompt` field is empty or the `[assist]` section is omitted, the workspace will be skipped during assist command execution.

### Usage

The assist feature requires:
- An LLM provider (via `--llm-provider` and the matching credential flags). Supported providers: `openai`, `claude` (direct Anthropic API or Vertex AI), `gemini`.
- Slack Bot Token (via `--slack-bot-token`)

Run the assist command (Gemini on Vertex AI example):

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=gemini \
  --llm-gemini-project-id=YOUR_GCP_PROJECT \
  --llm-gemini-location=global \
  --workspace=risk
```

Or with OpenAI:

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=openai \
  --llm-openai-api-key=sk-YOUR_OPENAI_KEY \
  --workspace=risk
```

Or with Claude on Vertex AI:

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=claude \
  --llm-gemini-project-id=YOUR_GCP_PROJECT \
  --llm-gemini-location=us-east5 \
  --workspace=risk
```

The AI agent will:
1. Retrieve recent assist logs and Slack messages for context
2. Analyze open cases in the specified workspace
3. Execute the custom prompt to generate insights
4. Post findings to the case's Slack channel

### Agent tool registry (Slack mention & assist)

The agent's available tools depend on which optional services are wired up.
Tools are split across three packages: `pkg/agent/tool/core` (case domain
state), `pkg/agent/tool/slack` (Slack-backed tools), and `pkg/agent/tool/notion`
(Notion-backed tools). The table below lists what each one provides and which
configuration enables it.

| Tool | Enabled by | Purpose |
|------|------------|---------|
| `core__list_actions`, `core__get_action`, `core__create_action`, `core__update_action`, `core__update_action_status`, `core__set_action_assignee` | Always | Manage the case's action items. |
| `core__search_knowledge`, `core__get_knowledge` | Always (LLM client required) | Semantic search and retrieval over knowledge entries. |
| `core__create_knowledge`, `core__update_knowledge` | Assist only | Persist new knowledge produced by the assist agent. |
| `core__create_memory`, `core__delete_memory`, `core__search_memory`, `core__list_memories` | Assist only | Manage per-case agent memory. |
| `slack__post_message` | Assist only (`HECATONCHEIRES_SLACK_BOT_TOKEN`) | Post a message to the case's Slack channel. |
| `slack__get_messages` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | Bulk fetch of one or more Slack messages and their thread context (max 10 per call, parallel, partial failure tolerated). |
| `slack__search_messages` | `HECATONCHEIRES_SLACK_USER_OAUTH_TOKEN` with `search:read` scope | Workspace-wide Slack message search. See [docs/slack.md](slack.md#user-token-scopes). |
| `notion__search`, `notion__get_page` | `HECATONCHEIRES_NOTION_API_TOKEN` | Notion title search and Markdown content retrieval. See [docs/notion.md](notion.md). |

---

## Action Section

The `[action]` section is **optional**. When omitted, the workspace inherits a built-in default set of action statuses (`BACKLOG`, `TODO`, `IN_PROGRESS`, `BLOCKED`, `COMPLETED`) so that data written before configurable statuses keeps working unchanged. Define this section to tailor the action workflow to your team.

```toml
[action]
initial = "queued"            # Required when [action] is present
closed  = ["done", "cancelled"]   # Optional, defaults to []

[[action.status]]
id = "queued"
name = "Queued"
description = "Awaiting triage"
color = "idle"                 # See "Color values" below
emoji = "📋"

[[action.status]]
id = "doing"
name = "Doing"
color = "active"
emoji = "▶️"

[[action.status]]
id = "done"
name = "Done"
color = "success"
emoji = "✅"
```

### `[action]` keys

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `initial` | string | **Yes** | ID of the status assigned to newly created actions. Must match one of the `[[action.status]]` IDs. |
| `closed` | string[] | No | IDs treated as "closed" for filtering / completion checks. Each ID must match a defined status. Defaults to `[]`. |

### `[[action.status]]` keys

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `id` | string | **Yes** | Stable identifier. Must match `^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$` and be unique within the workspace. Stored in the database as the action's status. |
| `name` | string | **Yes** | Display label. Write it in whichever language fits the workspace — there is no localization layer. |
| `description` | string | No | Free-form description (currently unused in the UI; reserved for tooltips). |
| `color` | string | No | UI color. See "Color values" below. Defaults to the neutral `idle` preset. |
| `emoji` | string | No | Single emoji used in Slack messages and web UI badges. |

### Color values

The `color` field accepts **two and only two** kinds of value. Any other string fails validation at startup.

#### 1. Semantic preset name (recommended)

Each preset expresses the **kind** of state, not a specific named status. Pick the preset that matches your status's intent — preset names are matched case-insensitively.

| Preset | Intended meaning | Suggested for statuses like |
|--------|------------------|-----------------------------|
| `idle` | Not started yet, queued, reserved | `backlog`, `queued`, `new` |
| `active` | Work in progress, someone is on it | `in_progress`, `doing`, `wip` |
| `waiting` | Waiting on someone else (review / external response / dependency) | `in_review`, `waiting_for_user`, `qa` |
| `paused` | Intentionally on hold | `on_hold`, `parked` |
| `attention` | Needs attention / something off-track | `needs_input`, `flagged` |
| `blocked` | Cannot proceed; something is preventing progress | `blocked`, `dependency_failed` |
| `success` | Completed in the desired way | `done`, `resolved`, `shipped` |
| `neutral_done` | Closed without value judgment (won't fix, expired, withdrawn) | `cancelled`, `wont_fix`, `expired` |
| `failure` | Closed in an undesired way (failed, rejected, abandoned) | `failed`, `rejected`, `lost` |

Presets resolve to CSS variables (`--action-status-<preset>`) on the frontend and follow the active light/dark theme.

#### 2. Hex color code

When no preset captures what you need, supply an absolute color as `#RRGGBB` or `#RGB`. Example: `"#5EAEDC"`, `"#abc"`. Hex codes do **not** follow the theme, so pick a color that reads on both light and dark backgrounds.

#### Not supported (will fail validation)

- CSS variable references like `"var(--ok)"` — the supported entry point is the preset list above.
- CSS color keywords like `"red"`, `"blue"`.
- Function notations: `rgb(...)`, `rgba(...)`, `hsl(...)`, `oklch(...)`.
- Any string that is neither a preset name nor `#RRGGBB`/`#RGB`.

#### Examples

| Value | Result |
|-------|--------|
| `"success"` | ✅ Preset, theme-aware green |
| `"WAITING"` | ✅ Case-insensitive preset match |
| `"#5EAEDC"` | ✅ Absolute hex |
| `"#abc"` | ✅ Short hex |
| (omitted) | ✅ Falls back to `idle` |
| `"var(--ok)"` | ❌ Validation error |
| `"red"` | ❌ Validation error |
| `"rgb(255,0,0)"` | ❌ Validation error |

---

## Validation Rules

The configuration file is validated at startup. The following rules are enforced:

| Rule | Error |
|------|-------|
| Configuration file must exist at the specified path | `ErrConfigNotFound` |
| All field IDs must match `^[a-z][a-z0-9_]*$` | `ErrInvalidFieldID` |
| All field names must be non-empty | `ErrMissingName` |
| Field type must be one of the 8 supported types | `ErrInvalidFieldType` |
| Field IDs must be unique across the entire configuration | `ErrDuplicateFieldID` |
| `select` and `multi-select` fields must have at least one option | `ErrMissingOptions` |
| All option IDs must match the same pattern as field IDs | `ErrInvalidFieldID` |
| All option names must be non-empty | `ErrMissingName` |
| Option IDs must be unique within their parent field | `ErrDuplicateOptionID` |
| Slack welcome message templates must parse | `ErrInvalidWelcomeMessage` |
| Action status IDs must match `^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$`, be at most 32 characters, and be unique within the workspace | (action status validation) |
| `[action] initial` must reference a defined `[[action.status]] id` | (action status validation) |
| Each entry in `[action] closed` must reference a defined `[[action.status]] id` | (action status validation) |
| `[[action.status]] color` must be a preset name or `#RRGGBB` / `#RGB` | (action status validation) |

If any validation fails, the application exits with a descriptive error message including the field ID and context.

---

## Complete Example

The following example configures Hecatoncheires as a security risk management system. See also [examples/config.toml](../examples/config.toml).

```toml
# Workspace configuration (required)
[workspace]
id = "risk"
name = "Risk Management"

# Customize entity labels to match your domain
[labels]
case = "Risk"

# Slack integration (optional)
[slack]
channel_prefix = "risk"

# Auto-invite users and groups to case channels (optional)
[slack.invite]
users = ["U12345678"]
groups = ["@security-response"]

# AI compile configuration (optional)
[compile]
prompt = "Extract security risk information, focusing on likelihood, impact, and mitigation strategies."

# AI assist configuration (optional)
[assist]
prompt = "Check action deadlines and follow up on pending items. Remind the team of overdue tasks."
language = "Japanese"

# Category field: multi-select with metadata
[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true
options = [
  { id = "data_breach", name = "Data Breach", metadata = { description = "Risk of personal or confidential information leakage", color = "red" } },
  { id = "system_failure", name = "System Failure", metadata = { description = "Risk of system or service downtime and failures", color = "orange" } },
]

# Likelihood field: select with scoring
[[fields]]
id = "likelihood"
name = "Likelihood"
type = "select"
required = true
options = [
  { id = "low", name = "Low", metadata = { description = "Unlikely to occur (a few times per year)", score = 2 } },
  { id = "high", name = "High", metadata = { description = "Likely to occur (about once per week)", score = 4 } },
]

# Simple fields without options
[[fields]]
id = "specific_impact"
name = "Specific Impact"
type = "text"

[[fields]]
id = "deadline"
name = "Deadline"
type = "date"

[[fields]]
id = "reference_url"
name = "Reference URL"
type = "url"
```

---

## GitHub Source Integration

Hecatoncheires can fetch Pull Requests, Issues, and comments from GitHub repositories as external data sources for knowledge extraction. This requires a GitHub App with appropriate permissions.

### GitHub App Setup

1. Create a GitHub App at `https://github.com/settings/apps/new`
2. Grant the following permissions:
   - **Repository permissions**: Issues (Read), Pull Requests (Read), Contents (Read)
3. Install the App on the target organization or repositories
4. Note the App ID, Installation ID, and download the private key

### Configuration

All three flags (`--github-app-id`, `--github-app-installation-id`, `--github-app-private-key`) must be set to enable GitHub Source features. If any flag is missing, GitHub features are gracefully disabled and the application continues to run normally with other source types.

```bash
hecatoncheires serve \
  --github-app-id=12345 \
  --github-app-installation-id=67890 \
  --github-app-private-key=/path/to/private-key.pem \
  ...
```

The `--github-app-private-key` accepts either a file path to a PEM file or the PEM content directly as a string.

### Source Management

GitHub Sources are managed via the GraphQL API:

- `createGitHubSource` - Create a new GitHub source with repository list
- `updateGitHubSource` - Update an existing GitHub source
- `validateGitHubRepo` - Validate access to a repository before adding it

Repositories can be specified in `owner/repo` format or as full GitHub URLs (e.g., `https://github.com/owner/repo`).

---

## See Also

- [Authentication Guide](./auth.md) - Slack OAuth setup and no-auth development mode
- [Slack Integration Guide](./slack.md) - Events API, webhooks, and channel management
- [Example Configuration](../examples/config.toml) - Complete example for security risk management
