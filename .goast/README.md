# goast policy catalog

This directory holds the repository's [goast](https://github.com/m-mizutani/goast)
policies: Rego rules evaluated over the Go AST that mechanically enforce the
coding conventions in `CLAUDE.md` and `.claude/rules/`. They turn "the reviewer
should have caught this" into "CI catches this".

## Layout

```
.goast/
  <name>.rego        # production policies
  <name>_test.rego   # opa unit tests (share package goast)
  testdata/          # Go fixtures for `goast test`
  README.md
../.goast.toml       # goast test cases (policy + source paths)
```

## How it runs

Three verification layers, cheapest first (all wired in
`.github/workflows/goast.yml`):

1. **`opa test .goast`** — unit-tests each rule against hand-written AST-JSON
   fixtures in the adjacent `*_test.rego` files. Fast and hermetic.
2. **`goast test`** — runs every rule through goast's real parse → walk → eval
   pipeline over the Go fixtures under `testdata/`, so the AST shapes the rules
   assume are checked against what the Go parser actually emits. Cases are
   declared in `../.goast.toml`.
3. **`goast eval -p .goast --fail ./pkg`** — applies the policies to the whole
   production tree as the CI gate. `-p` points at the whole `.goast` directory
   so every policy is enrolled automatically; a new rule can never be silently
   omitted from CI. The `*_test.rego` files get compiled too, but they only
   define `test_*` rules and helpers that `goast eval` never queries (it reads
   `data.goast.fail`), so they contribute neither findings nor measurable cost.

## Authoring notes (learned the hard way)

- **Anchor every rule on `input.Kind`** first, or a deep pattern matches the
  same code once per ancestor node and produces duplicate findings.
- **`goast dump` normalises an empty slice to `[]`, but live `goast eval`
  marshals a nil slice as JSON `null`.** `count(x)` on a null is undefined and
  silently kills the rule. Avoid `count()` on possibly-empty AST slices (see
  `no_strings_contains_error.rego`) — match on presence, not length.
- **All `*_test.rego` files share `package goast`,** so fixture-helper names
  must be globally unique across files (`doc_func_decl`, not `func_decl`).
- **Never name a helper `test_*`** — OPA treats any `test_`-prefixed rule as a
  test case, not a function.

## Catalog

| Policy | Bans | Scope | Exemptions |
|--------|------|-------|------------|
| `no_fmt_print` | `fmt.Print*`, builtin `print`/`println` | `./pkg` | eval CLI (`pkg/cli/eval.go`), eval harness (`pkg/usecase/eval/**`) |
| `usecase_context_first` | exported use-case funcs without `context.Context` first | `pkg/usecase` | `New*`/`With*`/`Is*`/`Has*`/`Can*`/`Parse*`, pure getters (see `exempt_name`), `pkg/usecase/eval/**`, `_test.go` |
| `no_slog_global` | direct `slog.Debug/Info/Warn/Error/Log`, `*Context`/`LogAttrs`, and `slog.With` | `./pkg` | attribute constructors and `slog.New*` are not matched by construction |
| `no_strings_contains_error` | `strings.Contains/HasPrefix/HasSuffix/EqualFold` on `err.Error()` | production | `_test.go` (assertions on third-party error text have no typed sentinel) |
| `no_firestore_struct_tags` | `firestore:"..."` struct field tags | `./pkg` | none (a firestore mention in a comment is not a Tag, so never matched) |
| `repo_no_doc_converter` | `toXxxDoc`/`fromXxxDoc` funcs and `XxxDoc` types | `pkg/repository` | none |
| `no_discarded_close` | `_ = x.Close()`, bare `x.Close()`, bare `defer x.Close()` | production | `_test.go` (httptest teardown), `pkg/utils/safe`; `safe.Close(...)` is the sanctioned form |
| `test_file_conventions` | (a) `_test.go` using an internal package; (b) `*_e2e_test.go`/`*_integration_test.go` filenames | `./pkg` | `export_test.go` (the sanctioned internal-access seam); `package main` (not importable, so `main_test.go` must stay internal) |

## Deliberately NOT enforced by goast

Two conventions are intentionally left to review rather than goast, because a
single-node, type-free AST match cannot express their real intent without
false positives:

- **Raw `go func(){}` outside `pkg/utils/async`.** The remaining production
  goroutines (server lifecycle, worker loops, parallel eval workers) are
  legitimate long-lived goroutines, not the fire-and-forget tasks
  `async.Dispatch` wraps. goast cannot tell the two apart.
- **`time.Now()` in repository write methods.** The rule targets *write
  methods*, but goast cannot identify a write method; a package-wide ban would
  flag legitimate clock-injection defaults and pure `IsExpired(time.Now())`
  comparisons.

## Adding a policy

1. Write a minimal Go sample, then `goast dump --line N sample.go | jq` to learn
   the exact AST shape — never guess field names.
2. Add `<name>.rego`, `<name>_test.rego`, and `testdata/<name>/{bad,good}.go`.
3. Add `[[test.cases]]` entries to `../.goast.toml` for both fixtures.
4. Run `opa test .goast`, `goast test`, and `goast eval -p .goast ./pkg`
   (expect 0 real violations, or fix the code that violates it).
5. Add a row to the catalog above. **No workflow edit is needed** — the CI
   `goast eval -p .goast` picks the new rule up automatically.
