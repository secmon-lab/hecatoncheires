---
paths:
  - "**/*.go"
---

# Go conventions

## Visibility

- **Default to unexported.** An identifier may only be capitalised
  when it is *actually* used from another package in non-test code.
  "A test uses it" is not a reason to export — that is what
  `export_test.go` is for.
- Before adding a capitalised name, grep for non-test callers
  (`grep -rn "pkg\." --include='*.go' | grep -v _test.go`). If none
  exist, lowercase it.
- For test-only access to internal identifiers, place a `package
  <pkg>` (NOT `<pkg>_test`) file named `export_test.go` and re-export
  under a `*ForTest` alias / variable / constant. The `ForTest`
  suffix is required so the seam is obvious at the call site.
  Example:

  ```go
  // notion/export_test.go
  package notion

  var BuildToolsForTest = buildTools
  func SetTokenForTest(f *Factory, t string) { f.token = t }
  ```

- Do NOT add capitalised names with comments like `// exported for
  testing` directly in production files. Move them into
  `export_test.go` instead — the compiler then enforces that the seam
  never reaches the production binary.
- Helper / setup files that exist only to support tests must end in
  `_test.go` so they never compile into the production binary.

## String literals

- **Every string literal in Go source MUST be English.** Log
  messages, `goerr.New` / `goerr.Wrap` messages, prompt templates,
  system prompts, error messages — no Japanese or other non-English
  text. The single exception is the i18n layer (`pkg/i18n/`); user-
  facing copy reaches Japanese only through `i18n.T(ctx, key, ...)`.
- When you spot Japanese inside a Go literal while editing nearby
  code, convert it. If it is end-user copy, route it through the
  i18n layer; otherwise just rewrite the literal in English.

## Error handling

- Use `goerr/v2` (`github.com/m-mizutani/goerr/v2`) for wrapping
  errors with context: `goerr.Wrap(err, "load case",
  goerr.V("case_id", id))`. Do not use `fmt.Errorf("%w", ...)` for
  new error sites — the codebase has standardised on goerr.
- Never check error messages with `strings.Contains(err.Error(),
  ...)`. Use `errors.Is` / `errors.As` against typed sentinels.
- Non-fatal errors (errors that don't require rollback or
  propagation) MUST go through `errutil.Handle(ctx, err,
  "description")`. Never use raw `logger.Error` for error reporting,
  and never describe error handling as "log only".

## Logging

- Never call `slog.Info()`, `slog.Error()`, `slog.Debug()`,
  `slog.Warn()` or other global slog logger functions directly.
  Always obtain a logger via `logging.From(ctx)` from
  `pkg/utils/logging/`.
- Attribute constructors (`slog.String()`, `slog.Any()`,
  `slog.Int64()`, etc.) are fine — use them as-is.

## Resource cleanup

- **ALWAYS use `safe.Close(ctx, closer)` from `pkg/utils/safe`** to
  close `io.Closer` resources.
- **NEVER use `_ = x.Close()` or bare `x.Close()`** — use
  `safe.Close` instead for nil-safe, error-logged cleanup.
  - BAD: `defer func() { _ = client.Close() }()`,
    `defer client.Close()`
  - GOOD: `defer safe.Close(ctx, client)`

## Background goroutines

- Background goroutines launch via `pkg/utils/async.Dispatch` /
  `RunParallel`, never raw `go func(){...}()`. Those helpers wrap
  panic recovery, logger context propagation, and error reporting.
- Tests that exercise async tails must call `async.Wait()` before
  asserting on side effects — do not rely on `time.Sleep`.

## Multi-instance safety

The application is designed to run as multiple concurrent instances.
Any design that assumes single-instance will break in production.

- **NEVER hold cross-request state in process memory.** State that
  must survive across separate requests, goroutines that originated
  elsewhere, or instance boundaries MUST be persisted to a shared
  backend (Firestore / GCS / Pub/Sub).
- **Allowed in-memory state**: only within a single continuous
  processing flow (e.g. variables within one HTTP request, one
  goroutine's local variables, one WebSocket connection's live
  buffer for the duration of that connection). As soon as the flow
  ends, the state must be gone or persisted.
- **Forbidden patterns**:
  - In-memory registry/map keyed by ID that other requests look up
    (e.g. `map[SessionID]*Handler` at package level).
  - Singleton caches of business data without a shared backend.
  - Cross-goroutine coordination via channels at package scope.
  - Assuming a WebSocket client is always on the same instance as
    the goroutine publishing to it.
