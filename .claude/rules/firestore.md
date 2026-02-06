---
paths:
  - "pkg/repository/firestore/**/*.go"
---

# Firestore Repository Rules

## CRITICAL PROHIBITIONS

### 1. Firestore Struct Tags Are ABSOLUTELY FORBIDDEN

**NEVER use `firestore:"..."` struct tags in any code.**

❌ **WRONG:**
```go
type document struct {
    ID        int64     `firestore:"id"`
    Title     string    `firestore:"title"`
    CreatedAt time.Time `firestore:"created_at"`
}
```

✅ **CORRECT:**
```go
// No struct tags - use domain model directly
type model.Case struct {
    ID        int64
    Title     string
    CreatedAt time.Time
}
```

### 2. Converter Functions Are ABSOLUTELY FORBIDDEN

**NEVER create converter functions like `modelToMap()` or `mapToModel()`.**

**ALWAYS use domain models directly with Firestore.**

❌ **WRONG:**
```go
func caseToMap(c *model.Case) map[string]interface{} {
    return map[string]interface{}{
        "ID":        c.ID,
        "Title":     c.Title,
        "CreatedAt": c.CreatedAt,
    }
}

func mapToCase(data map[string]interface{}) (*model.Case, error) {
    // ... conversion logic
}

// Usage
_, err := collection.Doc(id).Set(ctx, caseToMap(model))  // ❌ FORBIDDEN
```

✅ **CORRECT:**
```go
// No converter functions at all - use model directly

// Save
_, err := collection.Doc(id).Set(ctx, model)  // ✅ Use model directly

// Get
var c model.Case
doc, err := collection.Doc(id).Get(ctx)
if err := doc.DataTo(&c); err != nil {
    return nil, err
}
```

### 3. Field Names Must Use Go Naming (NOT snake_case)

**Firestore will automatically use Go field names (PascalCase) when no struct tags are present.**

This means Firestore documents will have fields like `ID`, `CreatedAt`, `UpdatedAt` (NOT `id`, `created_at`, `updated_at`).

### 4. Firestore Composite Indexes Are PROHIBITED

**Creating new Firestore composite indexes is PROHIBITED in principle.**

- Use existing single-field indexes only
- For batch operations requiring indexes, use parallel individual queries instead
- If a feature absolutely requires a composite index, **YOU MUST GET APPROVAL FIRST**

❌ **WRONG:**
```go
// Requires composite index on (CaseID, CreatedAt)
iter := client.Collection("items").
    Where("CaseID", "==", caseID).
    OrderBy("CreatedAt", firestore.Desc).  // ❌ Requires composite index
    Documents(ctx)
```

✅ **CORRECT (Option 1 - Remove OrderBy):**
```go
// Uses single-field index only
iter := client.Collection("items").
    Where("CaseID", "==", caseID).         // ✅ Single-field index
    Documents(ctx)
// Sort in memory if needed
```

✅ **CORRECT (Option 2 - Parallel queries):**
```go
// Execute parallel individual queries instead
for _, id := range ids {
    go func(id int64) {
        // Each query uses single-field index
        iter := client.Collection("items").
            Where("CaseID", "==", id).
            Documents(ctx)
    }(id)
}
```

## Correct Pattern: Use Models Directly

**ALWAYS use domain models directly - NO converter functions.**

```go
// Save - use model directly
_, err := collection.Doc(id).Set(ctx, model)

// Get - use DataTo with model
var c model.Case
doc, err := collection.Doc(id).Get(ctx)
if err != nil {
    return nil, err
}
if err := doc.DataTo(&c); err != nil {
    return nil, err
}
```

## Complete Repository Example

```go
func (r *caseRepository) Create(ctx context.Context, c *model.Case) (*model.Case, error) {
    now := time.Now().UTC()
    created := &model.Case{
        ID:          nextID,
        Title:       c.Title,
        Description: c.Description,
        CreatedAt:   now,
        UpdatedAt:   now,
    }

    docID := fmt.Sprintf("%d", created.ID)

    // Use model directly - NO converter function
    _, err = r.client.Collection("cases").Doc(docID).Set(ctx, created)
    if err != nil {
        return nil, goerr.Wrap(err, "failed to create case")
    }

    return created, nil
}

func (r *caseRepository) Get(ctx context.Context, id int64) (*model.Case, error) {
    docID := fmt.Sprintf("%d", id)
    docSnap, err := r.client.Collection("cases").Doc(docID).Get(ctx)
    if err != nil {
        return nil, goerr.Wrap(err, "failed to get case")
    }

    // Use DataTo directly - NO converter function
    var c model.Case
    if err := docSnap.DataTo(&c); err != nil {
        return nil, goerr.Wrap(err, "failed to decode case")
    }

    return &c, nil
}
```

## Rationale

1. **No struct tags**: Ensures explicit control over field mapping and prevents hidden magic behavior
2. **Go field names**: Maintains consistency with Go conventions and avoids unnecessary transformations
3. **No composite indexes**: Prevents index management overhead and keeps queries simple and maintainable
