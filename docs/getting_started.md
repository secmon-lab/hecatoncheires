# Getting Started

This guide gets Hecatoncheires running locally with the in-memory backend and
no authentication — the fastest path for evaluation and development. For a
production deployment (Firestore, Cloud Storage, Slack App, secrets), see
[Deployment](deployment.md).

## Prerequisites

- Go 1.21+
- Node.js 18+ and Corepack-managed pnpm (for building the frontend; the version
  is pinned via the `packageManager` field in `frontend/package.json`)

Enable Corepack once so the pinned pnpm is used automatically:

```bash
corepack enable
```

Do NOT install pnpm globally with `npm install -g pnpm` — that bypasses the pin.

## 1. Write a minimal `config.toml`

The configuration file defines your Workspace and its custom Fields. A complete,
annotated example lives at [`examples/config.toml`](../examples/config.toml);
the full reference is in [Configuration](configuration.md). A minimal file looks
like:

```toml
[workspace]
id = "default"
name = "My Workspace"

[[fields]]
id = "summary"
name = "Summary"
type = "text"
```

## 2. Run the server (memory backend, no auth)

```bash
go run . serve \
  --repository-backend=memory \
  --config=config.toml \
  --no-auth=U000000000 \
  --addr=:8080
```

- `--repository-backend=memory` keeps everything in process — no Firestore
  required. Data is lost on restart.
- `--no-auth=<slack-user-id>` runs without OAuth and treats every request as the
  given user. **Development only.** See
  [CLI → Authentication Modes](cli.md#authentication-modes).
- AI features stay disabled unless you also pass `--llm-provider`; see
  [Deployment → LLM provider](deployment.md#3-llm-provider).

## 3. Open the web UI

Visit <http://localhost:8080>. The GraphiQL playground (when enabled) is at
`/graphiql`.

## 4. Run the test suite (optional)

```bash
go test ./...
```

Frontend end-to-end tests use Playwright against a memory-backed server; see the
root [README](../README.md) for the e2e workflow.

## Where to go next

| You want to… | Read |
|---|---|
| Understand the vocabulary | [Concepts](concepts.md) |
| Deploy to production | [Deployment](deployment.md) |
| Customize fields & behavior | [Configuration](configuration.md) |
| Wire up Slack | [Slack Integration](slack.md) |
| Learn the day-to-day workflow | [User Guide](user_guide.md) |
