# Deployment

This guide covers a production deployment of Hecatoncheires. It is an overview
that points to the detailed references rather than duplicating them: flag and
environment-variable details live in [CLI Reference](cli.md), the config file is
in [Configuration](configuration.md), and Slack App setup is in
[Slack Integration](slack.md).

## Required services

| Service | Purpose | Notes |
|---|---|---|
| Google Cloud Firestore | Primary persistent store (Cases, Actions, Knowledge, …) | Set `--repository-backend=firestore` and `--firestore-project-id`. New Firestore indexes are avoided by policy — see [Operations](operations.md). |
| Google Cloud Storage | Agent thread session History/Trace blobs | Required when Slack/agent features are wired. Set `--cloud-storage-bucket`. |
| LLM provider | AI assist, agent sessions, agent Jobs | OpenAI, Anthropic Claude, or Google Gemini. See below. |
| Slack App | Slack integration (OAuth, Events, Interactivity, Slash) | See [Slack Integration](slack.md). |

## 1. Firestore

Create (or choose) a Google Cloud project and provision a Firestore database.
Run the server with:

```bash
hecatoncheires serve \
  --repository-backend=firestore \
  --firestore-project-id=YOUR_PROJECT_ID \
  --firestore-database-id=YOUR_DATABASE_ID   # optional; defaults to the project default DB
```

Application Default Credentials must be authorized for the project.

## 2. Cloud Storage (agent sessions)

Agent thread sessions persist their History and Trace artifacts to a Cloud
Storage bucket so sessions survive across instances and turns:

```bash
  --cloud-storage-bucket=YOUR_BUCKET \
  --cloud-storage-prefix=optional/key/prefix
```

The object layout and required IAM are documented in
[Architecture → Agent thread session](develop/architecture.md#agent-thread-session-internals).

## 3. LLM models

The models a deployment may use are declared in a global config file, one
`[[llm_model]]` entry each, with their prices. `--llm-model` then names which of
them is the default; AI features are disabled unless it is set.

```toml
# global.toml — pass via --global-config

[[llm_model]]
alias    = "main"
provider = "gemini"                 # openai | claude | gemini
model    = "gemini-3.7-flash"
input_usd_per_mtok  = 0.75          # USD per 1M tokens, from the provider's price page
output_usd_per_mtok = 3.75
```

Prices are required because an agent run's budget is denominated in money (see
[CLI → Agent runtime budgets](cli.md#agent-runtime-budgets)). The full field
reference is in
[Configuration → Model definitions](configuration.md#model-definitions-llm_model).

Credentials stay on the command line, and which ones are required follows the
entry's `provider`:

| Provider | Required credentials |
|---|---|
| `openai` | `--llm-openai-api-key` |
| `claude` | either `--llm-claude-api-key` (direct Anthropic) **or** `--llm-gemini-project-id` (Claude via Vertex AI) — mutually exclusive |
| `gemini` | `--llm-gemini-project-id` and `--llm-gemini-location` |

A client is built at startup for the default model and for every model an enabled
Job names, so those credentials must be present then.

An embedding client (Gemini) is also required whenever the LLM is enabled. It is
configured separately via `--embedding-gemini-project-id` /
`--embedding-gemini-location` / `--embedding-model` (default
`gemini-embedding-2`). See [CLI → `serve`](cli.md#serve) for the full flag list
and conditions.

## 4. Slack App

Set up the Slack App (OAuth & Permissions, Events API, Interactivity, Slash
Commands, and — for org-level installs — Enterprise Grid) following
[Slack Integration](slack.md). For Slack-bound deployments you will need at
least the bot token, signing secret, and (for OAuth) client credentials.

## 5. Passing secrets

Every flag has a matching `HECATONCHEIRES_*` environment variable (see
[CLI Reference](cli.md)). In production, inject secrets — API keys, Slack
tokens, signing secret — via environment variables sourced from your platform's
secret manager rather than command-line flags. Do not bake secrets into images
or commit them to the repository.

## 6. Start the server

```bash
hecatoncheires serve \
  --repository-backend=firestore \
  --firestore-project-id=YOUR_PROJECT_ID \
  --cloud-storage-bucket=YOUR_BUCKET \
  --config=./config.toml \
  --global-config=./global.toml \
  --llm-model=main \
  --llm-gemini-project-id=YOUR_PROJECT_ID \
  --embedding-gemini-project-id=YOUR_PROJECT_ID \
  --addr=:8080
```

For OAuth, signing secret, and Slack-specific environment variables, see
[Slack Integration → Environment Variables Reference](slack.md#environment-variables-reference).

## After deployment

- Wire scheduled agent Jobs (`tick`) and operate diagnostics/migrations — see
  [Operations](operations.md).
- Enable error reporting (Sentry) — see
  [Operations → Observability](operations.md#observability-sentry).

## See Also

- [CLI Reference](cli.md)
- [Configuration](configuration.md)
- [Slack Integration](slack.md)
- [Operations](operations.md)
