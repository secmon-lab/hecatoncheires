# CLI Reference

This document is the complete reference for the `hecatoncheires` command-line interface: its subcommands, their flags, and the environment variables they read.

All flags can also be set via environment variables. Environment variables use the prefix `HECATONCHEIRES_` with uppercase, underscore-separated names (e.g., `--log-level` becomes `HECATONCHEIRES_LOG_LEVEL`).

CLI flags take precedence over environment variables.

Subcommands:

- [`serve`](#serve) — start the HTTP server (GraphQL API + frontend + Slack webhooks).
- [`assist`](#assist) — run the AI assist agent across open cases.
- [`migrate`](#migrate) — manage Firestore indexes.
- [`validate`](#validate) — validate configuration files and optionally check DB consistency.
- [`diagnosis`](#diagnosis) — one-shot data inspection / repair jobs.
- [`tick`](#tick) — run a single sweep over scheduled Agent Jobs.
- [`eval`](./eval.md) — run offline scenario-based evaluation of LLM workflows (see [eval.md](./eval.md)).

For TOML configuration topics (workspace definitions, field schemas, the `[assist]` section, etc.), see [configuration.md](./configuration.md).

---

## Global Flags

Available for all commands.

| Flag | Alias | Env Var | Default | Description |
|------|-------|---------|---------|-------------|
| `--log-level` | `-l` | `HECATONCHEIRES_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `-f` | `HECATONCHEIRES_LOG_FORMAT` | `console` | Log format: `console`, `json` |
| `--log-output` | `-o` | `HECATONCHEIRES_LOG_OUTPUT` | `stdout` | Log output: `stdout`, `stderr`, `-`, or a file path |
| `--log-quiet` | `-q` | `HECATONCHEIRES_LOG_QUIET` | `false` | Quiet mode (disables all log output) |
| `--log-stacktrace` | `-s` | `HECATONCHEIRES_LOG_STACKTRACE` | `true` | Show stacktrace in console format |

---

## `serve`

The `serve` command (alias: `s`) starts the HTTP server.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--addr` | `HECATONCHEIRES_ADDR` | `:8080` | No | HTTP server address and port |
| `--base-url` | `HECATONCHEIRES_BASE_URL` | - | Yes\* | Application base URL (e.g., `https://your-domain.com`). No trailing slash |
| `--graphiql` | `HECATONCHEIRES_GRAPHIQL` | `true` | No | Enable GraphiQL playground at `/graphiql` |
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | No | Path to TOML configuration file |
| `--global-config` | `HECATONCHEIRES_GLOBAL_CONFIG` | - | No | Paths to deployment-wide config files/directories (TOML) holding `[[workspace_group]]` definitions. Unset leaves workspace groups dormant. See [configuration.md](./configuration.md#global-configuration-workspace-groups) |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Yes | Google Cloud Firestore project ID |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| `--notion-api-token` | `HECATONCHEIRES_NOTION_API_TOKEN` | - | No | Notion API token for Source integration |
| `--no-auth` | `HECATONCHEIRES_NO_AUTH` | - | No | Slack user ID for no-auth mode (development only) |
| `--slack-client-id` | `HECATONCHEIRES_SLACK_CLIENT_ID` | - | Yes\* | Slack OAuth client ID |
| `--slack-client-secret` | `HECATONCHEIRES_SLACK_CLIENT_SECRET` | - | Yes\* | Slack OAuth client secret |
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | No\*\* | Slack Bot User OAuth Token (`xoxb-...`) |
| `--slack-user-oauth-token` | `HECATONCHEIRES_SLACK_USER_OAUTH_TOKEN` | - | No | Slack User OAuth Token for admin API (`xoxp-...`, required for cross-workspace channel connect in Enterprise Grid) |
| `--slack-signing-secret` | `HECATONCHEIRES_SLACK_SIGNING_SECRET` | - | No\*\*\* | Slack signing secret for webhook verification |
| `--slack-notification-slot-duration` | `HECATONCHEIRES_NOTIFICATION_SLOT_DURATION` | `1h` | No | Rolling window during which Action/Step change notifications are aggregated into a single editable channel message. Set `0` to disable aggregation (legacy `reply_broadcast` per event). See [user_guide.md](./user_guide.md) |
| `--github-app-id` | `HECATONCHEIRES_GITHUB_APP_ID` | - | No | GitHub App ID for GitHub Source integration |
| `--github-app-installation-id` | `HECATONCHEIRES_GITHUB_APP_INSTALLATION_ID` | - | No | GitHub App Installation ID |
| `--github-app-private-key` | `HECATONCHEIRES_GITHUB_APP_PRIVATE_KEY` | - | No | GitHub App private key (PEM string or file path) |
| `--webfetch-enabled` | `HECATONCHEIRES_WEBFETCH_ENABLED` | `true` | No | Enable the agent `webfetch` tool. Built only when an LLM client is also configured (the LLM screens fetched content for indirect prompt injection). Connections to non-public IPs are blocked (SSRF guard) |
| `--webfetch-timeout` | `HECATONCHEIRES_WEBFETCH_TIMEOUT` | `10` | No | `webfetch` HTTP request timeout in seconds |
| `--webfetch-max-size` | `HECATONCHEIRES_WEBFETCH_MAX_SIZE` | `262144` | No | `webfetch` maximum response body size in bytes (excess is truncated). Default 256 KiB keeps a single fetch within model context / cost limits |
| `--llm-provider` | `HECATONCHEIRES_LLM_PROVIDER` | - | No\*\*\*\* | LLM provider: `openai`, `claude`, or `gemini`. Empty disables AI features |
| `--llm-model` | `HECATONCHEIRES_LLM_MODEL` | - | No | LLM model name (provider default if empty) |
| `--llm-openai-api-key` | `HECATONCHEIRES_LLM_OPENAI_API_KEY` | - | No\*\*\*\* | OpenAI API key (required when `--llm-provider=openai`) |
| `--llm-claude-api-key` | `HECATONCHEIRES_LLM_CLAUDE_API_KEY` | - | No\*\*\*\* | Anthropic Claude API key (used with direct Anthropic access) |
| `--llm-gemini-project-id` | `HECATONCHEIRES_LLM_GEMINI_PROJECT_ID` | - | No\*\*\*\* | Google Cloud project ID (Gemini, or Claude via Vertex AI) |
| `--llm-gemini-location` | `HECATONCHEIRES_LLM_GEMINI_LOCATION` | `global` | No | Google Cloud location for Gemini / Claude on Vertex AI |
| `--embedding-gemini-project-id` | `HECATONCHEIRES_EMBEDDING_GEMINI_PROJECT_ID` | - | Cond. | Google Cloud project ID for the Gemini embedding client. Required whenever `--llm-provider` is set |
| `--embedding-gemini-location` | `HECATONCHEIRES_EMBEDDING_GEMINI_LOCATION` | `global` | No | Google Cloud location for the Gemini embedding client |
| `--embedding-model` | `HECATONCHEIRES_EMBEDDING_MODEL` | `gemini-embedding-2` | No | Gemini embedding model name |
| `--cloud-storage-bucket` | `HECATONCHEIRES_CLOUD_STORAGE_BUCKET` | - | Yes\*\*\*\*\* | Cloud Storage bucket holding agent thread session History/Trace blobs. See [develop/architecture.md](./develop/architecture.md#agent-thread-session-internals) |
| `--cloud-storage-prefix` | `HECATONCHEIRES_CLOUD_STORAGE_PREFIX` | - | No | Optional object key prefix within the Cloud Storage bucket |
| `--sentry-dsn` | `HECATONCHEIRES_SENTRY_DSN` | - | No | Sentry DSN. Setting a non-empty value enables Sentry error reporting via `errutil.Handle`. See [operations.md](./operations.md) |
| `--sentry-env` | `HECATONCHEIRES_SENTRY_ENV` | - | No | Sentry environment tag (e.g., `production`, `staging`) |
| `--sentry-release` | `HECATONCHEIRES_SENTRY_RELEASE` | - | No | Sentry release identifier (e.g., commit SHA) |
| `--mcp` | `HECATONCHEIRES_MCP` | `false` | No | Enable the MCP (Model Context Protocol) endpoint at `/mcp`. Requires `--policy`. See [mcp.md](./mcp.md) |
| `--policy` | `HECATONCHEIRES_POLICY` | - | Cond. | Path(s) to Rego policy files or directories used to authorize MCP requests (`data.auth.mcp`). Repeatable. **Required** when `--mcp` is set |
| `--mcp-env` | `HECATONCHEIRES_MCP_ENV` | - | No | Names of environment variables to expose to the Rego policy as `input.env` (allow-list). Repeatable |

\* Required for OAuth mode. Alternatively, use `--no-auth` with `--slack-bot-token` for development.

\*\* Required when using `--no-auth`. Also enables user avatar display and Slack user refresh worker.

\*\*\* Required only to enable Slack webhook integration. Without this, webhook endpoints are not registered.

\*\*\*\* `--llm-provider` is optional for `serve` (AI features will be disabled if unset). When set, the matching provider credentials become required:
- `openai` → `--llm-openai-api-key`
- `claude` → either `--llm-claude-api-key` (direct Anthropic API) **or** `--llm-gemini-project-id` (Vertex AI). The two are mutually exclusive.
- `gemini` → `--llm-gemini-project-id` and `--llm-gemini-location`

The embedding client is configured separately from `--llm-provider` and is **required whenever LLM is enabled** (`--llm-provider` set on `serve`, or always for `assist`). It is reserved for upcoming similarity-search features; the wiring is preserved so callers can keep the same flags through the redesign. The default model is `gemini-embedding-2`; the dimension is fixed at 768. Application Default Credentials must be authorized for the project. Without `--llm-provider`, `serve` runs in a degraded mode that does not need the embedder either.

\*\*\*\*\* Required whenever `--slack-bot-token` is configured. The agent that responds to Slack mentions persists per-thread conversation History and execution Trace into the bucket so follow-up mentions can resume the session. The service account needs **Storage Object Admin** on the bucket.

The prefix for auto-created Slack channel names is not a CLI flag: it is configured per workspace via the `[slack] channel_prefix` key in the TOML configuration file, and defaults to the workspace ID when unset. See [configuration.md](./configuration.md#slack-section).

See [Authentication Modes](#authentication-modes) below for the two supported authentication configurations.

---

## `assist`

The `assist` command (alias: `a`) runs the AI assist agent for all open cases across workspaces. It requires an LLM provider, an embedding client, and a Slack bot token.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | Yes | Slack Bot Token for sending notifications |
| `--workspace` | `HECATONCHEIRES_ASSIST_WORKSPACE` | - | No | Target workspace ID (if empty, process all workspaces) |
| `--log-count` | `HECATONCHEIRES_ASSIST_LOG_COUNT` | `7` | No | Number of recent assist logs to include in system prompt |
| `--message-count` | `HECATONCHEIRES_ASSIST_MESSAGE_COUNT` | `50` | No | Number of recent Slack messages to include in system prompt |
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | No | Paths to configuration files or directories (TOML). Can be specified multiple times |
| `--repository-backend` | `HECATONCHEIRES_REPOSITORY_BACKEND` | `firestore` | No | Repository backend type (`firestore` or `memory`) |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Cond. | Firestore Project ID (required when using firestore backend) |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | - | No | Firestore Database ID |
| `--llm-provider` | `HECATONCHEIRES_LLM_PROVIDER` | - | Yes | LLM provider: `openai`, `claude`, or `gemini` (empty disables AI features). Required for `assist` |
| `--llm-model` | `HECATONCHEIRES_LLM_MODEL` | - | No | LLM model name (provider default if empty) |
| `--llm-openai-api-key` | `HECATONCHEIRES_LLM_OPENAI_API_KEY` | - | Cond. | OpenAI API key (required when `--llm-provider=openai`) |
| `--llm-claude-api-key` | `HECATONCHEIRES_LLM_CLAUDE_API_KEY` | - | Cond. | Anthropic Claude API key (used when `--llm-provider=claude` with direct Anthropic access) |
| `--llm-gemini-project-id` | `HECATONCHEIRES_LLM_GEMINI_PROJECT_ID` | - | Cond. | Google Cloud project ID (Gemini, or Claude via Vertex AI) |
| `--llm-gemini-location` | `HECATONCHEIRES_LLM_GEMINI_LOCATION` | `global` | No | Google Cloud location for Gemini / Claude on Vertex AI (e.g. `global`, `us-central1`) |
| `--embedding-gemini-project-id` | `HECATONCHEIRES_EMBEDDING_GEMINI_PROJECT_ID` | - | Yes | Google Cloud project ID for the Gemini embedding client |
| `--embedding-gemini-location` | `HECATONCHEIRES_EMBEDDING_GEMINI_LOCATION` | `global` | No | Google Cloud location for the Gemini embedding client (e.g. `global`, `us-central1`) |
| `--embedding-model` | `HECATONCHEIRES_EMBEDDING_MODEL` | `gemini-embedding-2` | No | Gemini embedding model name |

The `[assist]` TOML section (prompt, language) is documented in [configuration.md](./configuration.md).

---

## `migrate`

The `migrate` command (alias: `m`) manages Firestore indexes.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Yes | Google Cloud Firestore project ID |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| `--dry-run` | - | `false` | No | Preview migration changes without applying |

Operational depth (when to run a migration, emulator usage, index policy) lives in [operations.md](./operations.md).

---

## `validate`

The `validate` command (alias: `v`) validates configuration files and optionally checks DB consistency.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | No | Paths to configuration files or directories (TOML). Can be specified multiple times |
| `--global-config` | `HECATONCHEIRES_GLOBAL_CONFIG` | - | No | Paths to deployment-wide config files/directories (TOML). Validated (including that group members reference known workspaces) when present |
| `--repository-backend` | `HECATONCHEIRES_REPOSITORY_BACKEND` | `firestore` | No | Repository backend type (`firestore` or `memory`) |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Cond. | Firestore Project ID (required when using firestore backend) |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | - | No | Firestore Database ID |
| `--check-db` | `HECATONCHEIRES_CHECK_DB` | `false` | No | Perform database consistency check |

When `--check-db` is not specified, only the configuration files are validated and the DB consistency check is skipped.

Configuration validation parses every Go `text/template` prompt the config supplies — each `[[job]]` `prompt` / `prompt_file` and every Slack `welcome_messages` entry — with the same template dialect the runtime renders with. A malformed template (an unbalanced `{{ ... }}` action, an unknown function) fails `validate` up-front instead of only erroring the first time the Job runs or a case is created. This is a parse check: it proves the template compiles, not that a specific field reference resolves against a live case (that is exercised at render time).

---

## `diagnosis`

The `diagnosis` command (alias: `d`) groups one-shot data inspection / repair jobs. Each
sub-subcommand is a self-contained job; the umbrella itself takes no flags.

Operational runbook depth for these jobs lives in [operations.md](./operations.md).

### `diagnosis fix-unsent-action`

Re-posts Slack messages for Actions whose initial Slack post never reached
Slack. The job sweeps every workspace in the registry, finds Actions with an
empty `SlackMessageTS`, and replays the post via the unified
`ActionUseCase.PostSlackMessageToAction` entry point. Repeat runs are safe:
already-posted Actions are skipped.

```bash
hecatoncheires diagnosis fix-unsent-action \
  --config=./config.toml \
  --slack-bot-token=xoxb-... \
  --firestore-project-id=...
```

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | Yes | Workspace configuration file |
| `--base-url` | `HECATONCHEIRES_BASE_URL` | - | No | Base URL used to render the action's WebUI link inside the Slack message |
| `--default-lang` | `HECATONCHEIRES_DEFAULT_LANG` | `en` | No | Default language for the Slack message text (`en`, `ja`) |
| `--slack-bot-token` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | - | Yes | Slack Bot Token used to post the recovery messages |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Cond. | Required when using the Firestore backend |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | `(default)` | No | Firestore database ID |
| Sentry flags | see [operations.md](./operations.md) | - | No | Same flags as `serve` for error reporting |

The job logs a final summary line:

```
fix-unsent-action complete total=N fixed=X skipped=Y failed=Z
```

- `Total` — Actions found with an empty `SlackMessageTS`
- `Fixed` — Successfully posted; timestamp persisted
- `Skipped` — Documented skip conditions (parent Case has no Slack channel,
  the Action was already posted by a concurrent run, or the row was deleted
  during the sweep)
- `Failed` — Unexpected errors. Each is reported via `errutil.Handle` so it
  reaches the configured error sink (Sentry / log); the sweep continues
  past failures so a single bad row never blocks the rest

---

## `tick`

The `tick` command runs a single sweep over scheduled Agent Jobs and dispatches due ones. The same logic backs `POST /hooks/tick`; wire it to Cloud Scheduler (or any cron). The command exits when the sweep and in-flight async dispatches finish.

| Flag | Env Var | Default | Required | Description |
|------|---------|---------|----------|-------------|
| `--config` | `HECATONCHEIRES_CONFIG` | `./config.toml` | No | Paths to configuration files or directories (TOML). Can be specified multiple times |
| `--repository-backend` | `HECATONCHEIRES_REPOSITORY_BACKEND` | `firestore` | No | Repository backend type (`firestore` or `memory`) |
| `--firestore-project-id` | `HECATONCHEIRES_FIRESTORE_PROJECT_ID` | - | Cond. | Firestore Project ID (required when using firestore backend) |
| `--firestore-database-id` | `HECATONCHEIRES_FIRESTORE_DATABASE_ID` | - | No | Firestore Database ID |
| `--llm-provider` | `HECATONCHEIRES_LLM_PROVIDER` | - | No | LLM provider: `openai`, `claude`, or `gemini` (empty disables AI features) |
| `--llm-model` | `HECATONCHEIRES_LLM_MODEL` | - | No | LLM model name (provider default if empty) |
| `--llm-openai-api-key` | `HECATONCHEIRES_LLM_OPENAI_API_KEY` | - | Cond. | OpenAI API key (required when `--llm-provider=openai`) |
| `--llm-claude-api-key` | `HECATONCHEIRES_LLM_CLAUDE_API_KEY` | - | Cond. | Anthropic Claude API key (used when `--llm-provider=claude` with direct Anthropic access) |
| `--llm-gemini-project-id` | `HECATONCHEIRES_LLM_GEMINI_PROJECT_ID` | - | Cond. | Google Cloud project ID (Gemini, or Claude via Vertex AI) |
| `--llm-gemini-location` | `HECATONCHEIRES_LLM_GEMINI_LOCATION` | `global` | No | Google Cloud location for Gemini / Claude on Vertex AI (e.g. `global`, `us-central1`) |

Operational depth (scheduling cadence, relationship to `POST /hooks/tick`) lives in [operations.md](./operations.md).

---

## Authentication Modes

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

## See Also

- [configuration.md](./configuration.md) — TOML configuration (workspaces, field schemas, the `[assist]` section).
- [deployment.md](./deployment.md) — deployment topology and runtime requirements.
- [operations.md](./operations.md) — operational runbooks for `migrate`, `diagnosis`, and `tick`, plus Sentry / observability.
- [integrations.md](./integrations.md) — GitHub and Notion source integrations.
- [slack.md](./slack.md) — Slack app setup and OAuth scopes.
