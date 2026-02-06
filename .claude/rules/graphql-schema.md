---
paths:
  - "graphql/**"
  - "pkg/controller/graphql/**"
  - "pkg/domain/model/**"
  - "pkg/repository/**"
  - "pkg/usecase/**"
  - "frontend/src/graphql/**"
  - "frontend/src/pages/**"
  - "frontend/src/components/**"
---

# GraphQL Schema Governance

## Single Source of Truth: `graphql/schema.graphql`

`graphql/schema.graphql` is the **master definition** for the entire application's data contract. All other layers MUST conform to this schema. When in doubt about field names, types, nullability, or structure, always refer to this file first.

### Schema → Each Layer Mapping

```
graphql/schema.graphql          ← MASTER (define here first)
  ↓ gqlgen generate
pkg/domain/model/graphql/       ← Auto-generated Go types (DO NOT edit manually)
  ↓ referenced by
pkg/controller/graphql/         ← Converters, resolvers, dataloaders (conform to schema)
  ↓ data from
pkg/domain/model/               ← Domain models (independent of schema, but converters bridge the gap)
pkg/repository/                 ← Persistence (stores domain models)
pkg/usecase/                    ← Business logic (operates on domain models)
  ↓ consumed by
frontend/src/graphql/           ← GraphQL queries/mutations (MUST match schema field names & types)
frontend/src/pages/             ← Pages consuming GraphQL data
frontend/src/components/        ← Components consuming GraphQL data
```

### Workflow for Schema Changes

1. **Edit `graphql/schema.graphql`** — define or modify the field/type
2. **Run `task graphql`** — regenerate `models_gen.go`
3. **Update converters** (`converter.go`, `source_converter.go`) — bridge domain ↔ GraphQL
4. **Update resolvers** (`schema.resolvers.go`) — implement any new field resolvers
5. **Update dataloaders** (`dataloader.go`) — if the new field requires batch loading
6. **Update frontend queries** (`frontend/src/graphql/`) — match the schema exactly
7. **Update frontend components** — consume the data correctly

### What Each Layer Decides vs. What the Schema Decides

| Concern              | Decided by                    | NOT decided by            |
|----------------------|-------------------------------|---------------------------|
| Field names          | `schema.graphql`              | Frontend, Domain model    |
| Nullability (! or ?) | `schema.graphql`              | Converter, Repository     |
| List element types   | `schema.graphql`              | DataLoader, Resolver      |
| Field existence      | `schema.graphql`              | Frontend query selection  |
| Domain model fields  | `pkg/domain/model/`           | Schema                    |
| Persistence format   | `pkg/repository/`             | Schema                    |
| Business rules       | `pkg/usecase/`                | Schema                    |

## Nullability Rules

### Notation Reference

| Schema Type      | Meaning                              | Go Type (gqlgen)     |
|------------------|--------------------------------------|----------------------|
| `String!`        | Non-null scalar                      | `string`             |
| `String`         | Nullable scalar                      | `*string`            |
| `[String!]!`     | Non-null list with non-null elements | `[]string`           |
| `[String!]`      | Nullable list with non-null elements | `[]string`           |
| `[String]!`      | Non-null list with nullable elements | `[]*string`          |

### Backend (Go) Rules

#### Converter Layer (`converter.go`, `source_converter.go`)

- **Non-null list fields (`[T!]!`)**: MUST ensure the Go slice is never `nil`. If the domain model's slice is `nil`, substitute an empty slice.
  ```go
  // CORRECT
  assigneeIDs := c.AssigneeIDs
  if assigneeIDs == nil {
      assigneeIDs = []string{}
  }

  // WRONG — nil slice causes "null in non-null list" error
  AssigneeIDs: c.AssigneeIDs,
  ```

- **Nullable scalar fields (`String`)**: Use pointer (`*string`). Convert empty strings to `nil` if semantically appropriate.

#### DataLoader Layer (`dataloader.go`)

- **For non-null element lists (`[T!]!`)**: If an ID does not resolve to an entity (e.g., user deleted from Slack), **skip the nil entry** instead of including it.
  ```go
  // CORRECT — filter out missing entries
  result := make([]*T, 0, len(ids))
  for _, id := range ids {
      if item, ok := itemMap[id]; ok {
          result = append(result, item)
      }
  }

  // WRONG — nil elements violate [T!] constraint
  result[i] = itemMap[id]  // may be nil!
  ```

#### Resolver Layer (`schema.resolvers.go`)

- **Non-null list fields**: Always return an empty slice `[]T{}`, never `nil`.
- **Nullable input lists (mutation inputs)**: Convert `nil` to empty slice before passing to use cases.

### Frontend (TypeScript) Rules

- GraphQL query/mutation field names MUST exactly match `schema.graphql`.
- For `[T!]!` fields, always expect an array (never null/undefined). Initialize with `[]` as default.
- For nullable fields (`T` without `!`), handle `null` explicitly in the UI.
- When constructing mutation inputs with optional list fields (`[T!]`), omit the field entirely instead of sending `null`.

## Schema Design Conventions

- **Output type lists**: Prefer `[T!]!` as default. Safest for clients.
- **Input type lists**: Use `[T!]` (nullable list, non-null elements) for optional inputs.
- **Never use `[T]!` or `[T]`** (nullable elements) unless there is a concrete reason.

## Checklist for Adding/Modifying Fields

1. Define in `graphql/schema.graphql` with correct nullability
2. Run `task graphql` to regenerate
3. Verify generated Go types in `models_gen.go`
4. In converters: nil slices → empty slices for `[T!]!`
5. In dataloaders: no nil elements for `[T!]` types
6. In resolvers: empty slices for empty data, never nil
7. In frontend: query field names match schema exactly, handle nullability
