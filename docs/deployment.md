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

## 3. LLM provider

AI features are disabled unless `--llm-provider` is set. When set, the matching
provider credentials become required. An embedding client (Gemini) is also
required whenever the LLM is enabled.

| Provider | Required credentials |
|---|---|
| `openai` | `--llm-openai-api-key` |
| `claude` | either `--llm-claude-api-key` (direct Anthropic) **or** `--llm-gemini-project-id` (Claude via Vertex AI) — mutually exclusive |
| `gemini` | `--llm-gemini-project-id` and `--llm-gemini-location` |

The embedding client is configured separately via
`--embedding-gemini-project-id` / `--embedding-gemini-location` /
`--embedding-model` (default `gemini-embedding-2`). See
[CLI → `serve`](cli.md#serve) for the full flag list and conditions.

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
  --llm-provider=gemini \
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
