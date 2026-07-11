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

### Repository tests and the Firestore emulator

The repository tests include a `TestXxxRepository_Firestore` variant for every
repository. These exercise the real Firestore read/write path (the memory
backend copies structs wholesale and can hide field-drop bugs, so the Firestore
path is the one that actually guards persistence).

**These tests always run — they never skip on a missing env var.** A silent
skip is exactly what let a dropped-field bug through, so a missing backend must
fail loudly instead of passing as a no-op. Concretely, a bare `go test ./...`
targets a local Firestore emulator at `127.0.0.1:8080` by default, and **fails
with a connection error if no emulator is running there**. So run the suite
through the task below (or have an emulator listening on that port).

The simplest way is against the Firestore emulator in Docker — no GCP project,
credentials, or local Java install required (the only prerequisite is Docker):

```bash
task test:firestore
```

This starts the emulator container, points the tests at it, runs the full
suite, and removes the container afterwards.

> **Apple Silicon note.** `google/cloud-sdk:emulators` is an amd64-only image.
> On an arm64 Mac it runs under emulation, and the bundled JVM is only stable
> when your container runtime uses Rosetta (Docker Desktop: *Use Rosetta for
> x86/amd64 emulation*; Rancher Desktop: *Virtual Machine → Emulation →
> Rosetta*). Without Rosetta the emulator JVM may crash. CI runs on amd64
> Linux, where the image runs natively.

Under the hood the task sets one test-only environment variable — the Firestore
client connects to the emulator automatically when `FIRESTORE_EMULATOR_HOST` is
present, and the test helper defaults the project/database ids:

| Variable | Value | Purpose |
|---|---|---|
| `FIRESTORE_EMULATOR_HOST` | `127.0.0.1:8080` | Routes the client to the emulator (insecure, no auth). Unset → the helper defaults it, so the tests still target a local emulator |
| `TEST_FIRESTORE_PROJECT_ID` | *(optional)* | Set to a real project id (with ADC, e.g. `zenv task test`) to run the tests against real Firestore instead of the emulator. Unset → defaults to `test-project` |
| `TEST_FIRESTORE_DATABASE_ID` | *(optional)* | Firestore database id. Unset → defaults to `(default)` |

CI runs the same emulator setup in `.github/workflows/test.yml`, so these tests
run on every push.

> **Emulator caveat — composite indexes.** The emulator does not enforce
> Firestore composite index requirements, so a query that would need a new
> composite index in production still passes here. That gap is covered by the
> real-Firestore path (`zenv task test`) and by `hecatoncheires migrate`, which
> manages indexes via the Firestore Admin API (the emulator does not implement
> the Admin API, so `migrate` cannot and does not run against it).

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
