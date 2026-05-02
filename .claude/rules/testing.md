---
paths:
  - "**/*_test.go"
---

# Testing Rules

## Use gt Library for Assertions

**ALWAYS use `github.com/m-mizutani/gt` for test assertions. NEVER use raw `t.Errorf`, `t.Fatalf`, `t.Error`, or `t.Fatal`.**

### Basic Patterns

```go
import "github.com/m-mizutani/gt"

// Error checking
gt.NoError(t, err).Required()        // t.Fatalf equivalent (stops test on failure)
gt.NoError(t, err)                   // t.Errorf equivalent (continues on failure)
gt.Error(t, err).Is(targetErr)       // errors.Is check
gt.Value(t, err).NotNil()            // err != nil check
gt.Value(t, err).Nil()               // err == nil check

// Value comparison
gt.Value(t, got).Equal(expected)
gt.Value(t, got).NotEqual(unexpected)
gt.Value(t, ptr).Nil()
gt.Value(t, ptr).NotNil()

// String
gt.String(t, s).Equal("expected")
gt.String(t, s).NotEqual("")
gt.String(t, s).Contains("substring")

// Number
gt.Number(t, n).Equal(42)
gt.Number(t, n).GreaterOrEqual(1)
gt.Number(t, n).LessOrEqual(100)

// Boolean
gt.Bool(t, b).True()
gt.Bool(t, b).False()

// Array/Slice
gt.Array(t, slice).Length(3)
gt.Array(t, slice).Length(0)           // empty check
gt.Array(t, slice).Length(1).Required() // stop test if wrong length

// Map
gt.Map(t, m).HasKey("key")
```

### Important Notes

- Use `.Required()` when the test cannot continue if the assertion fails (equivalent to `t.Fatalf`)
- Omit `.Required()` when the test should continue after failure (equivalent to `t.Errorf`)
- For custom types (e.g., `model.SourceID`), use `gt.Value` instead of `gt.String`:
  ```go
  gt.Value(t, source.ID).NotEqual(model.SourceID(""))  // custom string type
  ```
- For `time.Time` equality, use `gt.Bool(t, t1.Equal(t2)).True()` to handle monotonic clock differences

## Test File Naming Convention

**STRICT: Test files MUST follow the `xyz.go` → `xyz_test.go` naming pattern. No exceptions.**

- Every test file must correspond to exactly one source file
- Test file name must match the source file name with `_test.go` suffix
- **NEVER create test files with other naming patterns** (e.g., `xyz_e2e_test.go`, `xyz_integration_test.go`, `xyz_unit_test.go`)
- All tests for a source file must be in ONE test file
- Test package must be `package {name}_test` (external test package)

### Examples

```
# CORRECT
client.go       → client_test.go
usecase.go      → usecase_test.go
case.go         → case_test.go

# WRONG - DO NOT DO THIS
client.go       → client_unit_test.go      ❌
client.go       → client_e2e_test.go       ❌
client.go       → client_integration_test.go ❌
(no source)     → helpers_test.go          ❌
```

### Repository Tests Location

- **NEVER** create test files in `pkg/repository/firestore/` or `pkg/repository/memory/` subdirectories
- ALL repository tests MUST be placed directly in `pkg/repository/*_test.go`

## String literals in tests

- **Every string literal in `_test.go` MUST be English.** Test fixtures (JSON payloads, struct field values, prompts), assertion messages, expected values in `gt.Value(...).Equal(...)`, table-test names — all of it. No Japanese or other non-English text in test code.
- This applies even when the production code under test produces Japanese user-facing copy. The contract you assert against is the i18n key path or the structural shape, not the rendered Japanese sentence — the latter changes whenever a translator tweaks a word and silently breaks tests.
- If you genuinely need to verify Japanese output (e.g. an i18n smoke test that the `ja` translation file resolves), pull the expected string from the same translation source the production code reads, do not hardcode it inline.

## Usecase-level tests

Usecase tests must verify *observable outcomes*, not just `err == nil`. A test that only asserts no error returned is not a test — it is a smoke check, and it will let regressions slip through. Every new public method on a usecase needs a real test on this bar:

- **Drive every external dependency through an interface** so tests can substitute fakes/mocks. If a usecase still embeds a concrete client, introduce a small interface in the usecase package covering only the methods it actually calls. Concrete production types satisfy the interface implicitly.
- **For LLM-touching code, use the gollem-provided mocks** (`github.com/m-mizutani/gollem/mock`). Set `NewSessionFunc` / `GenerateFunc` to control the conversation, and inspect the recorded `*Calls()` slices to assert the model was actually invoked with the expected input.
- **For Slack-touching code, use a hand-written fake** that records every method call (channel, thread_ts, body) into a slice, then assert the count, ordering, and exact field values. Do not assert "an error did not occur" alone.
- **Persistence is part of the contract**: when a usecase writes to the repo, the test must read it back via the repository interface and assert the stored fields — including foreign keys and timestamps where deterministic. Cover deduplication paths too.
- **Negative paths still need assertions**: when the code is supposed to no-op (LLM not configured, channel unmapped, entity not found), the test must assert that the dependency was *not* called — e.g. `t.Fatalf` inside the mock's `NewSessionFunc`, or `len(fake.calls) == 0` afterwards. "Returns nil error" is not enough.
- **Prompt / output content matters**: when the usecase builds a string for an external system (LLM prompt, Slack reply), assert the rendered content contains the expected context, sanitisation happened, and the downstream call received the model's exact output, not a paraphrase.

## Lifecycle / end-to-end tests

Per-method usecase tests are necessary but not sufficient. They verify each entry point in isolation, but they cannot catch state-machine bugs where the output of one entry point is the input to another. For any feature that spans multiple entry points / events, also write at least one test that drives the *full lifecycle* through the public API in order, with no mid-flight reaching into internals to set up state.

A lifecycle test should:

- **Start from the real entry point.** For Slack-driven flows, that is the `HandleAppMention` / interactivity handler — not a hand-rolled `repo.Case().Create` followed by direct usecase calls. Setting up intermediate state by hand defeats the purpose: the bug is usually in *how* intermediate state gets written.
- **Walk every observable transition.** Assert at every hop: domain state after each step, Slack call ordering, and the final entity fields.
- **Drive `async.Dispatch` deterministically.** Background work uses the package-level WaitGroup; tests must `async.Wait()` after each entry point call before asserting on side effects. Do not rely on `time.Sleep`.
- **Sequence the LLM mock by call count.** Use an `atomic.Int32` (or a slice of canned `Response`s) so call N returns one shape, call N+1 returns the next, and any extra call fails the test.
- **Assert on persisted state, not only Slack output.** Slack-only assertions miss persistence regressions.

Place lifecycle tests in the test file paired with the orchestrating source file (e.g. tests for `pkg/usecase/case.go`'s end-to-end flow live in `case_test.go`). The test file naming rule above forbids carved-out files like `lifecycle_test.go` — keep the lifecycle test alongside the per-method tests of the same orchestrating type, prefixed with `TestLifecycle_...` so it is still trivially greppable.

## Test assertion conventions

- Use `github.com/m-mizutani/gt` for assertions. Prefer it over raw `t.Errorf` / `t.Fatalf` calls.
- Use `.Required()` when the test cannot continue if the assertion fails (equivalent to `t.Fatalf`); omit it when the test should continue (equivalent to `t.Errorf`).
- For `time.Time` equality, use `gt.Bool(t, t1.Equal(t2)).True()` to handle monotonic clock differences.
- **NEVER use length-only checks** when individual values are knowable.
  - BAD: `gt.Array(t, toDelete).Length(3)` with the per-ID assertions commented out.
  - GOOD: Check each expected value explicitly.
- **NEVER comment out test assertions** — if a test doesn't work, fix it or delete it.
