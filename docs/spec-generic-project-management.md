# Development Specification: Generic Project Management Abstraction

## 1. Overview

### 1.1 Purpose

This specification defines the transformation of Hecatoncheires from a risk management system into a generic project/case management platform. The core idea is to replace domain-specific (risk management) data models with a flexible, configuration-driven architecture where field definitions are customizable via a TOML configuration file.

### 1.2 Goals

1. **Customizable fields** -- Entity fields are defined in a TOML configuration file, not hardcoded in source code.
2. **Risk-specific features become custom fields** -- Fields like `categoryIDs`, `likelihoodID`, `impactID`, `responseTeamIDs`, `specificImpact`, and `detectionIndicators` are no longer built-in; they are defined as custom field configurations.
3. **Generic naming** -- Risk-specific names (`Risk`, `Response`, `RiskResponse`, `RiskConfig`, etc.) are replaced with generic equivalents.

### 1.3 Non-Goals

- Multi-tenant support (one configuration per deployment remains).
- Runtime schema modification via API (configuration is file-based, loaded at startup).
- Changes to Source, Knowledge, Slack, or Auth subsystems beyond renaming foreign key references.

---

## 2. Naming Changes

All risk-specific terminology is replaced with generic equivalents. The table below shows the complete mapping.

### 2.1 Domain Model Naming

| Current | New | Rationale |
|---------|-----|-----------|
| `Risk` | `Case` | Generic unit of work / case |
| `Response` | `Action` | Task or action item linked to a case |
| `RiskResponse` | `CaseAction` | Many-to-many join between Case and Action |
| `RiskConfig` | `FieldSchema` | Configuration describing available custom fields |
| `RiskUseCase` | `CaseUseCase` | Use case layer |
| `ResponseUseCase` | `ActionUseCase` | Use case layer |
| `RiskRepository` | `CaseRepository` | Repository interface |
| `ResponseRepository` | `ActionRepository` | Repository interface |
| `RiskResponseRepository` | `CaseActionRepository` | Repository interface |
| `ResponseStatus` | `ActionStatus` | Status enum type |

### 2.2 GraphQL Naming

| Current | New |
|---------|-----|
| `type Risk` | `type Case` |
| `type Response` | `type Action` |
| `type RiskConfiguration` | `type FieldConfiguration` |
| `enum ResponseStatus` | `enum ActionStatus` |
| `input CreateRiskInput` | `input CreateCaseInput` |
| `input UpdateRiskInput` | `input UpdateCaseInput` |
| `input CreateResponseInput` | `input CreateActionInput` |
| `input UpdateResponseInput` | `input UpdateActionInput` |
| Query `risks` | Query `cases` |
| Query `risk(id)` | Query `case(id)` |
| Query `riskConfiguration` | Query `fieldConfiguration` |
| Query `responses` | Query `actions` |
| Query `response(id)` | Query `action(id)` |
| Query `responsesByRisk(riskID)` | Query `actionsByCase(caseID)` |
| Mutation `createRisk` | Mutation `createCase` |
| Mutation `updateRisk` | Mutation `updateCase` |
| Mutation `deleteRisk` | Mutation `deleteCase` |
| Mutation `createResponse` | Mutation `createAction` |
| Mutation `updateResponse` | Mutation `updateAction` |
| Mutation `deleteResponse` | Mutation `deleteAction` |
| Mutation `linkResponseToRisk` | Mutation `linkActionToCase` |
| Mutation `unlinkResponseFromRisk` | Mutation `unlinkActionFromCase` |

### 2.3 Frontend Naming

| Current | New |
|---------|-----|
| `RiskList.tsx` | `CaseList.tsx` |
| `RiskDetail.tsx` | `CaseDetail.tsx` |
| `RiskForm.tsx` | `CaseForm.tsx` |
| `RiskDeleteDialog.tsx` | `CaseDeleteDialog.tsx` |
| `ResponseList.tsx` | `ActionList.tsx` |
| `ResponseDetail.tsx` | `ActionDetail.tsx` |
| `ResponseForm.tsx` | `ActionForm.tsx` |
| `ResponseDeleteDialog.tsx` | `ActionDeleteDialog.tsx` |
| `graphql/risk.ts` | `graphql/case.ts` |
| `graphql/response.ts` | `graphql/action.ts` |
| URL `/risks` | URL `/cases` |
| URL `/risks/:id` | URL `/cases/:id` |
| URL `/responses` | URL `/actions` |
| URL `/responses/:id` | URL `/actions/:id` |

### 2.4 Firestore Collection Naming

| Current | New |
|---------|-----|
| `risks` | `cases` |
| `responses` | `actions` |
| `risk_responses` | `case_actions` |
| *(risk fields embedded in document)* | `case_field_values` (new collection) |
| *(N/A)* | `action_field_values` (new collection) |

### 2.5 CLI Flag and Environment Variable Naming

| Current | New |
|---------|-----|
| `--slack-channel-prefix` (default: `"risk"`) | `--slack-channel-prefix` (default: `"case"`) |

---

## 3. Custom Field System

### 3.1 Field Type Definitions

The following field types are supported:

| Type | Description | Storage Type | GraphQL Value Type |
|------|-------------|--------------|-------------------|
| `text` | Free-form text | `string` | `String` |
| `number` | Numeric value | `float64` | `Float` |
| `select` | Single selection from predefined options | `string` (option ID) | `String` |
| `multi-select` | Multiple selections from predefined options | `[]string` (option IDs) | `[String!]` |
| `user` | Single user reference (Slack user ID) | `string` | `String` |
| `multi-user` | Multiple user references | `[]string` | `[String!]` |
| `date` | Date value | `string` (RFC 3339) | `Time` |
| `url` | URL string | `string` | `String` |
| `scored-select` | Single selection with an associated numeric score | `string` (option ID) | `String` |

### 3.2 Field Option Definition

For `select`, `multi-select`, and `scored-select` types, options are defined as:

```
Option:
  id:          string   (required, lowercase alphanumeric with hyphens)
  name:        string   (required, display label)
  description: string   (optional)
  score:       int      (required for scored-select only)
  color:       string   (optional, hex color code for UI rendering)
```

### 3.3 Field Target

Each custom field is associated with a target entity:

| Target | Description |
|--------|-------------|
| `case` | Field appears on Case entities |
| `action` | Field appears on Action entities |

### 3.4 Built-in Fields (Not Customizable)

The following fields remain hardcoded and are NOT configurable via custom fields:

**Case built-in fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | `int64` | Auto-generated unique identifier |
| `title` | `string` | Case title (renamed from `name`) |
| `description` | `string` | Case description |
| `assigneeIDs` | `[]string` | Slack user IDs of assignees |
| `slackChannelID` | `string` | Associated Slack channel |
| `createdAt` | `time.Time` | Creation timestamp |
| `updatedAt` | `time.Time` | Last update timestamp |

**Action built-in fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | `int64` | Auto-generated unique identifier |
| `title` | `string` | Action title |
| `description` | `string` | Action description |
| `assigneeIDs` | `[]string` | Slack user IDs of assignees (renamed from `responderIDs`) |
| `url` | `string` | Reference URL |
| `status` | `ActionStatus` | Current status |
| `createdAt` | `time.Time` | Creation timestamp |
| `updatedAt` | `time.Time` | Last update timestamp |

### 3.5 Migration of Current Risk Fields to Custom Fields

The following table shows how each current risk-specific field maps to a custom field definition:

| Current Field | Custom Field ID | Custom Field Type | Target |
|---------------|----------------|-------------------|--------|
| `categoryIDs` | `category` | `multi-select` | `case` |
| `specificImpact` | `specific-impact` | `text` | `case` |
| `likelihoodID` | `likelihood` | `scored-select` | `case` |
| `impactID` | `impact` | `scored-select` | `case` |
| `responseTeamIDs` | `response-team` | `multi-select` | `case` |
| `detectionIndicators` | `detection-indicators` | `text` | `case` |

---

## 4. Configuration File Format

### 4.1 TOML Schema

The configuration file (`config.toml`) defines custom fields and their options.

```toml
# =============================================================================
# Entity Display Labels (optional)
# Customize how entities are displayed in the UI.
# These do NOT affect internal naming (code always uses "case" / "action").
# =============================================================================
[labels]
case = "Risk"         # Default: "Case"
action = "Response"   # Default: "Action"

# =============================================================================
# Custom Field Definitions
# Each [[fields]] block defines one custom field.
# =============================================================================

# --- Example: Multi-select field (replaces categoryIDs) ---
[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true
target = "case"
description = "Classification of the case"

  [[fields.options]]
  id = "data-breach"
  name = "Data Breach"
  description = "Risk of personal or confidential information leakage"
  color = "#E53E3E"

  [[fields.options]]
  id = "system-failure"
  name = "System Failure"
  description = "Risk of system or service downtime and failures"
  color = "#DD6B20"

  [[fields.options]]
  id = "compliance"
  name = "Compliance"
  description = "Risk of regulatory or internal policy violations"
  color = "#3182CE"

# --- Example: Scored-select field (replaces likelihoodID) ---
[[fields]]
id = "likelihood"
name = "Likelihood"
type = "scored-select"
required = true
target = "case"
description = "Probability of occurrence"

  [[fields.options]]
  id = "very-low"
  name = "Very Low"
  description = "Extremely unlikely to occur"
  score = 1

  [[fields.options]]
  id = "low"
  name = "Low"
  description = "Unlikely to occur"
  score = 2

  [[fields.options]]
  id = "medium"
  name = "Medium"
  description = "Moderately likely to occur"
  score = 3

  [[fields.options]]
  id = "high"
  name = "High"
  description = "Likely to occur"
  score = 4

  [[fields.options]]
  id = "very-high"
  name = "Very High"
  description = "Very likely to occur"
  score = 5

# --- Example: Scored-select field (replaces impactID) ---
[[fields]]
id = "impact"
name = "Impact"
type = "scored-select"
required = true
target = "case"
description = "Severity of potential harm"

  [[fields.options]]
  id = "negligible"
  name = "Negligible"
  description = "Little to no impact on business operations"
  score = 1

  [[fields.options]]
  id = "moderate"
  name = "Moderate"
  description = "Some business impact requiring response"
  score = 3

  [[fields.options]]
  id = "critical"
  name = "Critical"
  description = "Critical impact on business continuity"
  score = 5

# --- Example: Multi-select field (replaces responseTeamIDs) ---
[[fields]]
id = "response-team"
name = "Response Team"
type = "multi-select"
required = false
target = "case"

  [[fields.options]]
  id = "security-team"
  name = "Security Team"

  [[fields.options]]
  id = "infrastructure-team"
  name = "Infrastructure Team"

# --- Example: Text field (replaces specificImpact) ---
[[fields]]
id = "specific-impact"
name = "Specific Impact"
type = "text"
required = false
target = "case"
description = "Detailed description of impact for this specific case"

# --- Example: Text field (replaces detectionIndicators) ---
[[fields]]
id = "detection-indicators"
name = "Detection Indicators"
type = "text"
required = false
target = "case"
description = "Triggers and indicators for detection"

# --- Example: Custom field on Action ---
[[fields]]
id = "priority"
name = "Priority"
type = "select"
required = false
target = "action"
description = "Priority level of this action"

  [[fields.options]]
  id = "low"
  name = "Low"

  [[fields.options]]
  id = "medium"
  name = "Medium"

  [[fields.options]]
  id = "high"
  name = "High"

  [[fields.options]]
  id = "urgent"
  name = "Urgent"
```

### 4.2 Configuration Validation Rules

The configuration loader MUST enforce the following rules at startup:

1. **Field ID uniqueness**: No two fields may share the same `id` within the same `target`.
2. **Option ID uniqueness**: No two options within a single field may share the same `id`.
3. **ID format**: All `id` values must match `^[a-z0-9]+(-[a-z0-9]+)*$`.
4. **Required name**: `name` is required for all fields and options.
5. **Type validity**: `type` must be one of the defined field types.
6. **Target validity**: `target` must be `"case"` or `"action"`.
7. **Score requirement**: `scored-select` options MUST have a `score` value. Other types MUST NOT.
8. **Options requirement**: `select`, `multi-select`, and `scored-select` types MUST have at least one option. `text`, `number`, `user`, `multi-user`, `date`, `url` types MUST NOT have options.

---

## 5. Domain Model Changes

### 5.1 New Domain Types

#### `pkg/domain/types/field.go`

```go
type FieldID string
type FieldType string

const (
    FieldTypeText         FieldType = "text"
    FieldTypeNumber       FieldType = "number"
    FieldTypeSelect       FieldType = "select"
    FieldTypeMultiSelect  FieldType = "multi-select"
    FieldTypeUser         FieldType = "user"
    FieldTypeMultiUser    FieldType = "multi-user"
    FieldTypeDate         FieldType = "date"
    FieldTypeURL          FieldType = "url"
    FieldTypeScoredSelect FieldType = "scored-select"
)

type FieldTarget string

const (
    FieldTargetCase FieldTarget = "case"
    FieldTargetAction FieldTarget = "action"
)
```

#### `pkg/domain/types/action_status.go` (renamed from `response_status.go`)

```go
type ActionStatus string

const (
    ActionStatusBacklog    ActionStatus = "backlog"
    ActionStatusTodo       ActionStatus = "todo"
    ActionStatusInProgress ActionStatus = "in-progress"
    ActionStatusBlocked    ActionStatus = "blocked"
    ActionStatusCompleted  ActionStatus = "completed"
    ActionStatusAbandoned  ActionStatus = "abandoned"
)
```

### 5.2 New Domain Models

#### `pkg/domain/model/case.go` (replaces `risk.go`)

```go
type Case struct {
    ID             int64
    Title          string              // renamed from Name
    Description    string
    AssigneeIDs    []string
    SlackChannelID string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

Note: Custom field values are NOT stored on the Case struct. They are stored in a dedicated collection and loaded separately (see Section 5.3).

#### `pkg/domain/model/action.go` (replaces `response.go`)

```go
type Action struct {
    ID          int64
    Title       string
    Description string
    AssigneeIDs []string             // renamed from ResponderIDs
    URL         string
    Status      types.ActionStatus   // renamed from ResponseStatus
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

Note: Same as Case -- custom field values are stored in a dedicated collection.

#### `pkg/domain/model/case_action.go` (replaces `risk_response.go`)

```go
type CaseAction struct {
    CaseID  int64
    ActionID  int64
    CreatedAt time.Time
}
```

#### `pkg/domain/model/field_value.go` (new)

```go
// FieldValue represents a single custom field value attached to an entity.
// Stored in a dedicated Firestore collection, not embedded in the parent document.
type FieldValue struct {
    EntityID  int64          // CaseID or Action ID
    FieldID   string         // References FieldDefinition.ID from configuration
    Value     any            // Actual value; Go type depends on field type (see Section 5.3)
    UpdatedAt time.Time
}
```

#### `pkg/domain/model/config/field.go` (replaces `risk.go`)

```go
type FieldOption struct {
    ID          string
    Name        string
    Description string
    Score       int    // only for scored-select
    Color       string // optional hex color
}

type FieldDefinition struct {
    ID          string
    Name        string
    Type        types.FieldType
    Required    bool
    Target      types.FieldTarget
    Description string
    Options     []FieldOption
}

type FieldSchema struct {
    Fields []FieldDefinition
    Labels EntityLabels
}

type EntityLabels struct {
    Case   string // display name for Case (default: "Case")
    Action string // display name for Action (default: "Action")
}
```

### 5.3 Custom Field Value Storage

Custom field values are stored in **dedicated Firestore collections** (`case_field_values` and `action_field_values`), not embedded in the parent Case/Action documents. Each `FieldValue` document represents a single field's value for a single entity.

#### Value Type Mapping

The `Value` field in `FieldValue` holds a Go `any` whose concrete type depends on the field type:

| Field Type | Go Value Type | Firestore Type | Example |
|------------|--------------|----------------|---------|
| `text` | `string` | `string` | `"Some text"` |
| `number` | `float64` | `number` | `42.5` |
| `select` | `string` | `string` | `"option-id"` |
| `multi-select` | `[]string` | `array<string>` | `["opt-a", "opt-b"]` |
| `user` | `string` | `string` | `"U12345"` |
| `multi-user` | `[]string` | `array<string>` | `["U12345", "U67890"]` |
| `date` | `string` (RFC 3339) | `string` | `"2025-01-15T00:00:00Z"` |
| `url` | `string` | `string` | `"https://example.com"` |
| `scored-select` | `string` | `string` | `"option-id"` |

#### Design Rationale

Storing field values in a dedicated collection rather than a map inside the parent document provides:

1. **Independent lifecycle** -- Field values can be created, updated, and deleted without touching the parent Case/Action document.
2. **Consistent query pattern** -- Follows the same pattern as `case_actions` (join table). Simple equality queries (`Where("entity_id", "==", id)`) require no composite indexes.
3. **Batch operations** -- Parallel per-entity queries for batch loading, consistent with the existing Firestore index policy.
4. **Schema evolution** -- Adding or removing custom fields from configuration does not require modifying existing parent documents. Orphaned field values (whose field definition has been removed from config) are simply ignored at read time.

### 5.4 Custom Field Validation

A new `FieldValidator` is introduced in the usecase layer to validate custom field values against the field schema:

```go
type FieldValidator struct {
    schema *config.FieldSchema
}

func (v *FieldValidator) ValidateCaseFields(fields []*model.FieldValue) error
func (v *FieldValidator) ValidateActionFields(fields []*model.FieldValue) error
```

Validation rules:
1. Required fields must be present and non-empty.
2. `FieldID` of each `FieldValue` must exist in the schema for the corresponding target.
3. Unknown field IDs are rejected.
4. Value types must match the expected Go type for the field type.
5. For `select` / `scored-select` / `multi-select`, values must be valid option IDs.
6. For `user` / `multi-user`, values must be non-empty strings.
7. For `date`, values must be valid RFC 3339 strings.
8. For `url`, values must be valid URL strings.
9. Duplicate field IDs within a single request are rejected.

---

## 6. Repository Layer Changes

### 6.1 Interface Changes

#### `pkg/domain/interfaces/repository.go`

```go
type Repository interface {
    Case()       CaseRepository       // was Risk()
    Action()       ActionRepository       // was Response()
    CaseAction() CaseActionRepository // was RiskResponse()
    CaseField()  FieldValueRepository   // new: custom field values for cases
    ActionField()  FieldValueRepository   // new: custom field values for actions
    Slack()        SlackRepository        // unchanged
    SlackUser()    SlackUserRepository    // unchanged
    Source()       SourceRepository       // unchanged
    Knowledge()    KnowledgeRepository    // unchanged

    PutToken(ctx, token)    error
    GetToken(ctx, tokenID)  (*auth.Token, error)
    DeleteToken(ctx, tokenID) error
}
```

#### `pkg/domain/interfaces/case.go` (replaces `risk.go`)

```go
type CaseRepository interface {
    Create(ctx context.Context, c *model.Case) (*model.Case, error)
    Get(ctx context.Context, id int64) (*model.Case, error)
    List(ctx context.Context) ([]*model.Case, error)
    Update(ctx context.Context, c *model.Case) (*model.Case, error)
    Delete(ctx context.Context, id int64) error
}
```

#### `pkg/domain/interfaces/action.go` (replaces `response.go`)

```go
type ActionRepository interface {
    Create(ctx context.Context, action *model.Action) (*model.Action, error)
    Get(ctx context.Context, id int64) (*model.Action, error)
    List(ctx context.Context) ([]*model.Action, error)
    Update(ctx context.Context, action *model.Action) (*model.Action, error)
    Delete(ctx context.Context, id int64) error
}
```

#### `pkg/domain/interfaces/case_action.go` (replaces `risk_response.go`)

```go
type CaseActionRepository interface {
    Link(ctx, caseID, actionID int64) error
    Unlink(ctx, caseID, actionID int64) error
    GetActionsByCase(ctx, caseID int64) ([]int64, error)
    GetActionsByCases(ctx, caseIDs []int64) (map[int64][]int64, error)
    GetCasesByAction(ctx, actionID int64) ([]int64, error)
    GetCasesByActions(ctx, actionIDs []int64) (map[int64][]int64, error)
    DeleteByAction(ctx, actionID int64) error
    DeleteByCase(ctx, caseID int64) error
}
```

#### `pkg/domain/interfaces/field_value.go` (new)

A single generic interface used for both `CaseField()` and `ActionField()`. The underlying Firestore collection differs, but the interface is identical.

```go
// FieldValueRepository manages custom field values for an entity type.
// Two instances exist: one backed by "case_field_values" collection,
// another by "action_field_values" collection.
type FieldValueRepository interface {
    // GetByEntityID returns all field values for a single entity.
    // Query: Where("entity_id", "==", entityID)
    GetByEntityID(ctx context.Context, entityID int64) ([]*model.FieldValue, error)

    // GetByEntityIDs returns field values for multiple entities.
    // Implemented as parallel per-entity queries to avoid composite indexes.
    GetByEntityIDs(ctx context.Context, entityIDs []int64) (map[int64][]*model.FieldValue, error)

    // Save persists field values for an entity.
    // This is a full replacement: existing field values for the entity are
    // deleted, then the provided values are written.
    // Uses deterministic document IDs ("{entityID}_{fieldID}") for upsert semantics.
    Save(ctx context.Context, entityID int64, fields []*model.FieldValue) error

    // DeleteByEntityID deletes all field values for an entity.
    // Called when the parent Case or Action is deleted.
    DeleteByEntityID(ctx context.Context, entityID int64) error
}
```

### 6.2 Firestore Storage

#### Case Document Structure

```
Collection: "cases"
Document ID: auto-generated int64

{
  "title": "Data breach via API endpoint",
  "description": "...",
  "assignee_ids": ["U12345", "U67890"],
  "slack_channel_id": "C123456",
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

Note: No `fields` map in the document. Custom field values are in the `case_field_values` collection.

#### Action Document Structure

```
Collection: "actions"
Document ID: auto-generated int64

{
  "title": "Patch API endpoint",
  "description": "...",
  "assignee_ids": ["U12345"],
  "url": "https://github.com/...",
  "status": "in-progress",
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

Note: No `fields` map. Custom field values are in the `action_field_values` collection.

#### Case Field Value Document Structure

```
Collection: "case_field_values"
Document ID: "{caseID}_{fieldID}"  (deterministic, enables upsert)

Example document ID: "42_category"
{
  "entity_id": 42,
  "field_id": "category",
  "value": ["data-breach", "compliance"],
  "updated_at": "2025-01-15T10:00:00Z"
}

Example document ID: "42_likelihood"
{
  "entity_id": 42,
  "field_id": "likelihood",
  "value": "high",
  "updated_at": "2025-01-15T10:00:00Z"
}

Example document ID: "42_specific-impact"
{
  "entity_id": 42,
  "field_id": "specific-impact",
  "value": "Customer PII exposed via unprotected API endpoint",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

#### Action Field Value Document Structure

```
Collection: "action_field_values"
Document ID: "{actionID}_{fieldID}"

Example document ID: "7_priority"
{
  "entity_id": 7,
  "field_id": "priority",
  "value": "high",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

#### Query Patterns

| Operation | Query | Index Required |
|-----------|-------|---------------|
| Get all fields for a case | `case_field_values.Where("entity_id", "==", 42)` | No (single equality) |
| Get all fields for multiple cases | Parallel queries per case ID | No |
| Save fields for a case | Batch write with deterministic doc IDs | No |
| Delete all fields for a case | Query by entity_id + batch delete | No |

All queries use single-field equality filters, consistent with the Firestore index policy.

### 6.3 Memory Repository

The memory repository stores field values in a `map[int64][]*model.FieldValue` (keyed by entity ID). The `FieldValueRepository` interface is implemented identically for case and action field values, differing only in the backing map instance.

---

## 7. GraphQL Schema Changes

### 7.1 New Type Definitions

```graphql
# Field type definitions
enum FieldType {
  TEXT
  NUMBER
  SELECT
  MULTI_SELECT
  USER
  MULTI_USER
  DATE
  URL
  SCORED_SELECT
}

enum FieldTarget {
  TICKET
  ACTION
}

type FieldOption {
  id: String!
  name: String!
  description: String
  score: Int
  color: String
}

type FieldDefinition {
  id: String!
  name: String!
  type: FieldType!
  required: Boolean!
  target: FieldTarget!
  description: String
  options: [FieldOption!]
}

type EntityLabels {
  case: String!
  action: String!
}

type FieldConfiguration {
  fields: [FieldDefinition!]!
  labels: EntityLabels!
}
```

### 7.2 Custom Field Value Representation

```graphql
# A single custom field value on a case or action
type FieldValue {
  fieldId: String!
  # Value encoded as JSON. Clients parse based on field type from FieldConfiguration.
  # Examples:
  #   text:         "some text"
  #   number:       42.5
  #   select:       "option-id"
  #   multi-select: ["opt-a", "opt-b"]
  #   user:         "U12345"
  #   multi-user:   ["U12345", "U67890"]
  #   date:         "2025-01-15T00:00:00Z"
  #   url:          "https://example.com"
  #   scored-select: "option-id"
  value: Any!
}

# Input for setting custom field values
input FieldValueInput {
  fieldId: String!
  value: Any!
}

scalar Any
```

### 7.3 Updated Entity Types

```graphql
type Case {
  id: Int!
  title: String!
  description: String!
  assigneeIDs: [String!]!
  assignees: [SlackUser!]!
  slackChannelID: String
  fields: [FieldValue!]!       # Resolved via DataLoader from case_field_values collection
  actions: [Action!]!
  knowledges: [Knowledge!]!
  createdAt: Time!
  updatedAt: Time!
}

type Action {
  id: Int!
  title: String!
  description: String!
  assigneeIDs: [String!]!
  assignees: [SlackUser!]!
  url: String
  status: ActionStatus!
  fields: [FieldValue!]!       # Resolved via DataLoader from action_field_values collection
  cases: [Case!]!
  createdAt: Time!
  updatedAt: Time!
}

enum ActionStatus {
  BACKLOG
  TODO
  IN_PROGRESS
  BLOCKED
  COMPLETED
  ABANDONED
}
```

The `fields` on both `Case` and `Action` are **not** stored in the parent document. They are resolved by the GraphQL resolver via DataLoader, which batch-loads field values from the dedicated collection using `FieldValueRepository.GetByEntityIDs()`.

### 7.4 Updated Inputs

```graphql
input CreateCaseInput {
  title: String!
  description: String!
  assigneeIDs: [String!]
  fields: [FieldValueInput!]
}

input UpdateCaseInput {
  id: Int!
  title: String!
  description: String!
  assigneeIDs: [String!]
  fields: [FieldValueInput!]
}

input CreateActionInput {
  title: String!
  description: String!
  assigneeIDs: [String!]
  url: String
  status: ActionStatus
  caseIDs: [Int!]
  fields: [FieldValueInput!]
}

input UpdateActionInput {
  id: Int!
  title: String
  description: String
  assigneeIDs: [String!]
  url: String
  status: ActionStatus
  caseIDs: [Int!]
  fields: [FieldValueInput!]
}
```

### 7.5 Updated Queries and Mutations

```graphql
type Query {
  health: String!

  # Cases (was risks)
  cases: [Case!]!
  case(id: Int!): Case

  # Actions (was responses)
  actions: [Action!]!
  action(id: Int!): Action
  actionsByCase(caseID: Int!): [Action!]!

  # Configuration
  fieldConfiguration: FieldConfiguration!

  # Unchanged
  slackUsers: [SlackUser!]!
  sources: [Source!]!
  source(id: String!): Source
  slackJoinedChannels: [SlackChannelInfo!]!
  knowledge(id: String!): Knowledge
  knowledges(limit: Int, offset: Int): KnowledgeConnection!
}

type Mutation {
  noop: Boolean

  # Cases (was risks)
  createCase(input: CreateCaseInput!): Case!
  updateCase(input: UpdateCaseInput!): Case!
  deleteCase(id: Int!): Boolean!

  # Actions (was responses)
  createAction(input: CreateActionInput!): Action!
  updateAction(input: UpdateActionInput!): Action!
  deleteAction(id: Int!): Boolean!

  # Case-Action linking (was risk-response)
  linkActionToCase(actionID: Int!, caseID: Int!): Boolean!
  unlinkActionFromCase(actionID: Int!, caseID: Int!): Boolean!

  # Sources (unchanged)
  createNotionDBSource(input: CreateNotionDBSourceInput!): Source!
  createSlackSource(input: CreateSlackSourceInput!): Source!
  updateSource(input: UpdateSourceInput!): Source!
  updateSlackSource(input: UpdateSlackSourceInput!): Source!
  deleteSource(id: String!): Boolean!
  validateNotionDB(databaseID: String!): NotionDBValidationResult!
}
```

---

## 8. Use Case Layer Changes

### 8.1 CaseUseCase (replaces RiskUseCase)

```go
type CaseUseCase struct {
    repo           interfaces.Repository
    fieldSchema    *config.FieldSchema
    fieldValidator *FieldValidator
    slackService   slack.Service
}

func (uc *CaseUseCase) CreateCase(ctx, title, description string, assigneeIDs []string, fields []*model.FieldValue) (*model.Case, error)
func (uc *CaseUseCase) UpdateCase(ctx, id int64, title, description string, assigneeIDs []string, fields []*model.FieldValue) (*model.Case, error)
func (uc *CaseUseCase) DeleteCase(ctx, id int64) error
func (uc *CaseUseCase) GetCase(ctx, id int64) (*model.Case, error)
func (uc *CaseUseCase) GetCaseFieldValues(ctx, caseID int64) ([]*model.FieldValue, error)
func (uc *CaseUseCase) ListCases(ctx) ([]*model.Case, error)
func (uc *CaseUseCase) GetFieldConfiguration() *config.FieldSchema
```

Key changes:
- No more individual `ValidateCategoryID`, `ValidateLikelihoodID`, etc. methods. A single `FieldValidator` handles all validation.
- `CreateCase` validates custom fields via `FieldValidator.ValidateCaseFields(fields)`, then persists the case and its field values in two separate repository calls:
  1. `repo.Case().Create(ctx, c)` -- creates the case document.
  2. `repo.CaseField().Save(ctx, c.ID, fields)` -- saves field values to the dedicated collection.
- `DeleteCase` deletes both the case document and all associated field values:
  1. `repo.CaseField().DeleteByEntityID(ctx, id)` -- deletes field values.
  2. `repo.CaseAction().DeleteByCase(ctx, id)` -- deletes case-action links.
  3. `repo.Case().Delete(ctx, id)` -- deletes the case document.
- Slack channel creation logic remains, using `title` instead of `name`.

### 8.2 ActionUseCase (replaces ResponseUseCase)

```go
type ActionUseCase struct {
    repo           interfaces.Repository
    fieldSchema    *config.FieldSchema
    fieldValidator *FieldValidator
}

func (uc *ActionUseCase) CreateAction(ctx, title, description string, assigneeIDs []string, url string, status types.ActionStatus, caseIDs []int64, fields []*model.FieldValue) (*model.Action, error)
func (uc *ActionUseCase) UpdateAction(ctx, id int64, ..., fields []*model.FieldValue) (*model.Action, error)
func (uc *ActionUseCase) DeleteAction(ctx, id int64) error
func (uc *ActionUseCase) GetAction(ctx, id int64) (*model.Action, error)
func (uc *ActionUseCase) GetActionFieldValues(ctx, actionID int64) ([]*model.FieldValue, error)
func (uc *ActionUseCase) ListActions(ctx) ([]*model.Action, error)
func (uc *ActionUseCase) LinkActionToCase(ctx, actionID, caseID int64) error
func (uc *ActionUseCase) UnlinkActionFromCase(ctx, actionID, caseID int64) error
func (uc *ActionUseCase) GetActionsByCase(ctx, caseID int64) ([]*model.Action, error)
func (uc *ActionUseCase) GetCasesByAction(ctx, actionID int64) ([]*model.Case, error)
```

Same pattern as CaseUseCase: create/update/delete operations handle both the entity document and field values via separate repository calls.

### 8.3 UseCases Struct

```go
type UseCases struct {
    repo             interfaces.Repository
    fieldSchema      *config.FieldSchema
    // ... services ...
    Case          *CaseUseCase   // was Risk
    Action           *ActionUseCase   // was Response
    Auth             AuthUseCaseInterface
    Slack            *SlackUseCases
    Source           *SourceUseCase
    Compile          *CompileUseCase
}

func WithFieldSchema(schema *config.FieldSchema) Option  // was WithRiskConfig
```

---

## 9. Frontend Changes

### 9.1 Dynamic Form Rendering

The current `RiskForm.tsx` has hardcoded form fields for risk-specific attributes. The new `CaseForm.tsx` must render fields dynamically based on the `FieldConfiguration` query.

#### Field Rendering Strategy

Each field type maps to a specific form input component:

| Field Type | Component |
|------------|-----------|
| `text` | `<textarea>` or `<input type="text">` |
| `number` | `<input type="number">` |
| `select` | `<select>` / custom dropdown |
| `multi-select` | Multi-select chip selector (reuse existing `Chip` component) |
| `user` | User picker (reuse existing Slack user selector) |
| `multi-user` | Multi-user picker (reuse existing multi-user selector) |
| `date` | `<input type="date">` |
| `url` | `<input type="url">` |
| `scored-select` | `<select>` with score display |

#### Component Architecture

```
CaseForm
  ├── Built-in fields (title, description, assignees)
  ├── CustomFieldRenderer (iterates fieldConfiguration.fields where target=TICKET)
  │   ├── TextField
  │   ├── NumberField
  │   ├── SelectField
  │   ├── MultiSelectField
  │   ├── UserField
  │   ├── MultiUserField
  │   ├── DateField
  │   ├── URLField
  │   └── ScoredSelectField
  └── Form actions (submit, cancel)
```

### 9.2 Dynamic Detail View

`CaseDetail.tsx` renders custom field values dynamically:

- Iterate over `case.fields` and resolve display names and option labels from `fieldConfiguration`.
- For `select` / `scored-select`, display the option `name` (and optionally score).
- For `multi-select`, display chips for each selected option.
- For `user` / `multi-user`, display resolved Slack user names and avatars.

### 9.3 Dynamic List View

`CaseList.tsx` renders a table with configurable columns:

- Built-in columns: ID, Title, Assignees, Created At.
- Custom field columns are rendered based on field configuration.
- `select` / `multi-select` fields render as chips.

### 9.4 Entity Labels

The frontend fetches `fieldConfiguration.labels` to display the correct entity names:

```typescript
// Instead of hardcoded "Risk" / "Response"
const { data } = useQuery(GET_FIELD_CONFIGURATION);
const caseLabel = data?.fieldConfiguration.labels.case ?? "Case";
const actionLabel = data?.fieldConfiguration.labels.action ?? "Action";
```

All UI text (page titles, button labels, breadcrumbs, empty states) uses these dynamic labels.

---

## 10. Knowledge System Adaptation

### 10.1 Model Change

```go
type Knowledge struct {
    ID        KnowledgeID
    CaseID  int64        // was RiskID
    SourceID  SourceID
    SourceURL string
    Title     string
    Summary   string
    Embedding []float32
    SourcedAt time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 10.2 Repository Interface Change

```go
type KnowledgeRepository interface {
    Create(ctx, knowledge) error
    Get(ctx, id) (*model.Knowledge, error)
    ListByCaseID(ctx, caseID int64) ([]*model.Knowledge, error)      // was ListByRiskID
    ListByCaseIDs(ctx, caseIDs []int64) (map[int64][]*model.Knowledge, error)  // was ListByRiskIDs
    ListBySourceID(ctx, sourceID) ([]*model.Knowledge, error)
    ListWithPagination(ctx, limit, offset int) ([]*model.Knowledge, int, error)
    Delete(ctx, id) error
}
```

### 10.3 GraphQL Change

```graphql
type Knowledge {
  id: String!
  caseID: Int!    # was riskID
  case: Case     # was risk
  sourceID: String!
  sourceURL: String!
  title: String!
  summary: String!
  sourcedAt: Time!
  createdAt: Time!
  updatedAt: Time!
}
```

---

## 11. Types to Remove

The following domain-specific types are no longer needed as standalone types, since their validation is handled generically by the custom field system:

| Type | Replacement |
|------|-------------|
| `types.CategoryID` | Validated as option ID of a `multi-select` field |
| `types.LikelihoodID` | Validated as option ID of a `scored-select` field |
| `types.ImpactID` | Validated as option ID of a `scored-select` field |
| `types.TeamID` | Validated as option ID of a `multi-select` field |
| `config.RiskConfig` | Replaced by `config.FieldSchema` |
| `config.Category` | Replaced by `config.FieldOption` |
| `config.LikelihoodLevel` | Replaced by `config.FieldOption` |
| `config.ImpactLevel` | Replaced by `config.FieldOption` |
| `config.Team` | Replaced by `config.FieldOption` |

---

## 12. Data Migration

### 12.1 Firestore Collection Migration

A migration command (`hecatoncheires migrate generic`) is needed to transform existing data:

1. **Copy and transform `risks` → `cases`**: Copy documents, rename `name` → `title`, remove risk-specific fields from the document.
2. **Extract field values → `case_field_values`**: For each risk document, extract the risk-specific fields (`category_ids`, `likelihood_id`, `impact_id`, `response_team_ids`, `specific_impact`, `detection_indicators`) and create individual `FieldValue` documents in `case_field_values`.
3. **Copy and transform `responses` → `actions`**: Copy documents, rename `responder_ids` → `assignee_ids`.
4. **Copy `risk_responses` → `case_actions`**: Rename `risk_id` → `case_id`, `response_id` → `action_id`.
5. **Update Knowledge references**: Rename `risk_id` field to `case_id` in knowledge documents.
6. **Clean up**: After verification, delete old collections.

### 12.2 Migration Transformation (Case)

```
Before (Risk document in "risks" collection, doc ID: "42"):
{
  "name": "API Data Breach",
  "description": "...",
  "category_ids": ["data-breach"],
  "specific_impact": "Customer PII exposed",
  "likelihood_id": "high",
  "impact_id": "critical",
  "response_team_ids": ["security-team"],
  "assignee_ids": ["U12345"],
  "detection_indicators": "Abnormal access patterns",
  "slack_channel_id": "C123",
  "created_at": "...",
  "updated_at": "..."
}

After -- Case document in "cases" collection (doc ID: "42"):
{
  "title": "API Data Breach",
  "description": "...",
  "assignee_ids": ["U12345"],
  "slack_channel_id": "C123",
  "created_at": "...",
  "updated_at": "..."
}

After -- Field value documents in "case_field_values" collection:

  Document "42_category":
  { "entity_id": 42, "field_id": "category",
    "value": ["data-breach"], "updated_at": "..." }

  Document "42_specific-impact":
  { "entity_id": 42, "field_id": "specific-impact",
    "value": "Customer PII exposed", "updated_at": "..." }

  Document "42_likelihood":
  { "entity_id": 42, "field_id": "likelihood",
    "value": "high", "updated_at": "..." }

  Document "42_impact":
  { "entity_id": 42, "field_id": "impact",
    "value": "critical", "updated_at": "..." }

  Document "42_response-team":
  { "entity_id": 42, "field_id": "response-team",
    "value": ["security-team"], "updated_at": "..." }

  Document "42_detection-indicators":
  { "entity_id": 42, "field_id": "detection-indicators",
    "value": "Abnormal access patterns", "updated_at": "..." }
```

### 12.3 CLI Command

```
hecatoncheires migrate generic \
  --firestore-project-id=PROJECT_ID \
  --firestore-database-id=DATABASE_ID \
  --dry-run     # Preview changes without writing
```

---

## 13. Implementation Phases

### Phase 1: Core Abstraction (Backend)

1. Introduce new domain types (`FieldID`, `FieldType`, `FieldTarget`, `ActionStatus`).
2. Create `FieldSchema`, `FieldDefinition`, and `FieldValue` domain models.
3. Update configuration loader to parse custom fields from TOML.
4. Implement `FieldValidator`.
5. Create `Case` and `Action` domain models (without embedded fields).
6. Create `CaseAction` join model.
7. Update repository interfaces (`CaseRepository`, `ActionRepository`, `CaseActionRepository`, `FieldValueRepository`).
8. Implement memory repository for all new interfaces including `FieldValueRepository`.
9. Implement firestore repository for all new interfaces including `FieldValueRepository` (backed by `case_field_values` and `action_field_values` collections).
10. Write tests for all new code.

### Phase 2: Use Case and GraphQL (Backend)

1. Create `CaseUseCase` and `ActionUseCase`.
2. Update `UseCases` struct and options.
3. Update GraphQL schema (`schema.graphql`).
4. Regenerate GraphQL code (`task graphql`).
5. Implement new resolvers.
6. Update DataLoaders.
7. Write tests for all new code.

### Phase 3: Frontend

1. Create reusable custom field rendering components.
2. Create `CaseForm.tsx` with dynamic field rendering.
3. Create `CaseList.tsx` and `CaseDetail.tsx`.
4. Create `ActionForm.tsx`, `ActionList.tsx`, `ActionDetail.tsx`.
5. Update GraphQL queries and mutations.
6. Update routing and navigation.
7. Implement entity label support.

### Phase 4: Migration and Cleanup

1. Implement `migrate generic` CLI command.
2. Test migration with sample data.
3. Remove old risk-specific types, models, repositories, resolvers, and frontend components.
4. Remove old `config.RiskConfig` and related types.
5. Update `CLAUDE.md` and documentation.
6. Update example configuration file.

---

## 14. Backward Compatibility Considerations

- **No backward compatibility layer is maintained.** This is a breaking change.
- The migration command (Phase 4) handles data transformation.
- The old GraphQL API is fully replaced; clients must update queries.
- The TOML configuration file format changes entirely; old `config.toml` files are incompatible.
- Firestore collection names change; the migration command handles this.

---

## 15. Files Affected (Summary)

### New Files
- `pkg/domain/types/field.go`
- `pkg/domain/types/action_status.go`
- `pkg/domain/model/case.go`
- `pkg/domain/model/action.go`
- `pkg/domain/model/case_action.go`
- `pkg/domain/model/field_value.go`
- `pkg/domain/model/config/field.go`
- `pkg/domain/interfaces/case.go`
- `pkg/domain/interfaces/action.go`
- `pkg/domain/interfaces/case_action.go`
- `pkg/domain/interfaces/field_value.go`
- `pkg/usecase/case.go`
- `pkg/usecase/action.go`
- `pkg/usecase/field_validator.go`
- `pkg/repository/firestore/case.go`
- `pkg/repository/firestore/action.go`
- `pkg/repository/firestore/case_action.go`
- `pkg/repository/firestore/field_value.go`
- `pkg/repository/memory/case.go`
- `pkg/repository/memory/action.go`
- `pkg/repository/memory/case_action.go`
- `pkg/repository/memory/field_value.go`
- `pkg/cli/migrate_generic.go`
- `frontend/src/pages/CaseList.tsx`
- `frontend/src/pages/CaseDetail.tsx`
- `frontend/src/pages/CaseForm.tsx`
- `frontend/src/pages/CaseDeleteDialog.tsx`
- `frontend/src/pages/ActionList.tsx`
- `frontend/src/pages/ActionDetail.tsx`
- `frontend/src/pages/ActionForm.tsx`
- `frontend/src/pages/ActionDeleteDialog.tsx`
- `frontend/src/components/fields/` (custom field rendering components)
- `frontend/src/graphql/case.ts`
- `frontend/src/graphql/action.ts`

### Files to Delete (after migration)
- `pkg/domain/types/category.go`
- `pkg/domain/types/likelihood.go`
- `pkg/domain/types/impact.go`
- `pkg/domain/types/team.go`
- `pkg/domain/types/response_status.go`
- `pkg/domain/model/risk.go`
- `pkg/domain/model/response.go`
- `pkg/domain/model/risk_response.go`
- `pkg/domain/model/config/risk.go`
- `pkg/domain/interfaces/risk.go`
- `pkg/domain/interfaces/response.go`
- `pkg/domain/interfaces/risk_response.go`
- `pkg/usecase/risk.go`
- `pkg/usecase/response.go`
- `pkg/repository/firestore/risk.go`
- `pkg/repository/firestore/response.go`
- `pkg/repository/firestore/risk_response.go`
- `pkg/repository/memory/risk.go`
- `pkg/repository/memory/response.go`
- `pkg/repository/memory/risk_response.go`
- `frontend/src/pages/RiskList.tsx`
- `frontend/src/pages/RiskDetail.tsx`
- `frontend/src/pages/RiskForm.tsx`
- `frontend/src/pages/RiskDeleteDialog.tsx`
- `frontend/src/pages/ResponseList.tsx`
- `frontend/src/pages/ResponseDetail.tsx`
- `frontend/src/pages/ResponseForm.tsx`
- `frontend/src/pages/ResponseDeleteDialog.tsx`
- `frontend/src/graphql/risk.ts`
- `frontend/src/graphql/response.ts`

### Files to Modify
- `graphql/schema.graphql` (complete rewrite of risk/response types)
- `pkg/domain/interfaces/repository.go` (update method names, add FieldValueRepository accessors)
- `pkg/usecase/usecase.go` (update struct and options)
- `pkg/usecase/compile.go` (update Knowledge references)
- `pkg/cli/serve.go` (update configuration loading)
- `pkg/cli/config/config.go` (new TOML schema)
- `pkg/controller/graphql/resolver.go` (update resolver)
- `pkg/controller/graphql/dataloaders.go` (update loaders, add FieldValue DataLoader)
- `frontend/src/App.tsx` (update routes)
- `frontend/src/components/Layout.tsx` (update navigation)
- `frontend/src/components/Sidebar.tsx` (update navigation labels)
- `examples/config.toml` (new format)
- `CLAUDE.md` (update terminology and documentation)


