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
