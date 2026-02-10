---
name: gen-hecaton-config
description: Generate a Hecatoncheires config.toml file for your workspace. Use when setting up a new workspace, adding custom fields, or creating configuration from scratch.
disable-model-invocation: true
allowed-tools: Read, Write
argument-hint: "[use-case description]"
---

# Generate Hecatoncheires Configuration

Generate a valid `config.toml` for Hecatoncheires by interacting with the user to understand their requirements.

## Step 1: Gather use case

If `$ARGUMENTS` is provided, use it as the starting description. Otherwise, ask the user:

> What kind of cases will this workspace manage?
> (e.g., security risks, incident tracking, task management, vulnerability management, compliance issues)

Based on the answer, proceed to Step 2.

## Step 2: Propose a configuration plan

From the user's use case, draft a **configuration plan** covering all of the following and present it to the user for review. Do NOT generate the TOML file yet.

The plan must include:

1. **Workspace ID** — a short identifier (lowercase alphanumeric with hyphens, max 63 chars, pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`)
2. **Workspace Name** — human-readable display name
3. **Entity Label** — what "Case" should be called in the UI (e.g., "Risk", "Incident", "Task")
4. **Slack Channel Prefix** — prefix for auto-created Slack channels (defaults to workspace ID if omitted)
5. **Field list** — a table of proposed fields, each with:
   - Field ID, Display Name, Type, Required (yes/no)
   - For select/multi-select: the list of options with IDs and names

Present the plan as a readable summary (not TOML) so the user can review it easily. For example:

```
Workspace: risk (Risk Management)
Entity Label: "Risk"
Slack Channel Prefix: risk

Fields:
  1. category (Category) — multi-select, required
     Options: data-breach, system-failure, compliance
  2. severity (Severity) — select, required
     Options: critical, high, medium, low
  3. assignee (Assignee) — user, optional
  4. deadline (Deadline) — date, optional
  5. reference-url (Reference URL) — url, optional
```

Then ask the user:
- Does this plan look correct?
- Are there fields to add, remove, or change?
- Any options to adjust?

Iterate until the user confirms the plan.

## Step 3: Generate the TOML file

Once the user confirms, generate the `config.toml` following the specification below. Ask the user where to write the file (default: `./config.toml`).

---

## Configuration Specification

### Overall TOML Structure

```toml
[workspace]
id = "workspace-id"
name = "Display Name"

[labels]
case = "Risk"

[slack]
channel_prefix = "risk"

[[fields]]
id = "field-id"
name = "Display Name"
type = "text"
required = true
description = "Help text"
```

### `[workspace]` section (required)

| Key    | Required | Description                                                          |
|--------|----------|----------------------------------------------------------------------|
| `id`   | **Yes**  | Unique identifier. Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`, max 63 chars |
| `name` | No       | Display name. Defaults to the value of `id` if omitted               |

### `[labels]` section (optional)

| Key    | Default  | Description                            |
|--------|----------|----------------------------------------|
| `case` | `"Case"` | Display name for the primary entity    |

### `[slack]` section (optional)

| Key              | Default        | Description                                   |
|------------------|----------------|-----------------------------------------------|
| `channel_prefix` | workspace `id` | Prefix for auto-created Slack channel names   |

### `[[fields]]` array (custom field definitions)

Each element defines a custom field displayed in the case form and detail view.

| Property      | Type    | Required | Description                                 |
|---------------|---------|----------|---------------------------------------------|
| `id`          | string  | **Yes**  | Unique identifier. Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `name`        | string  | **Yes**  | Display name shown in the UI                |
| `type`        | string  | **Yes**  | One of the 8 supported types (see below)    |
| `required`    | boolean | No       | Whether the field is required (default: `false`) |
| `description` | string  | No       | Help text shown in the UI                   |
| `options`     | array   | Conditional | Required for `select` and `multi-select` types |

### Supported field types

| Type           | Description                                | Requires `options` |
|----------------|--------------------------------------------|--------------------|
| `text`         | Single-line text input                     | No                 |
| `number`       | Numeric input                              | No                 |
| `select`       | Single selection from a predefined list    | **Yes** (1+)       |
| `multi-select` | Multiple selections from a predefined list | **Yes** (1+)       |
| `user`         | Single Slack user reference                | No                 |
| `multi-user`   | Multiple Slack user references             | No                 |
| `date`         | Date picker                                | No                 |
| `url`          | URL input with validation                  | No                 |

### Option properties (for `select` / `multi-select`)

Each option is an inline table in the `options` array:

```toml
options = [
  { id = "opt-id", name = "Display Name" },
  { id = "another", name = "Another", description = "Help text", color = "red" },
  { id = "scored", name = "Scored", metadata = { score = 5, escalation = true } },
]
```

| Property      | Type           | Required | Description                                              |
|---------------|----------------|----------|----------------------------------------------------------|
| `id`          | string         | **Yes**  | Unique within the field. Same pattern as field ID        |
| `name`        | string         | **Yes**  | Display name                                             |
| `description` | string         | No       | Description text                                         |
| `color`       | string         | No       | Color name or hex code (e.g., `"red"`, `"#E53E3E"`)     |
| `metadata`    | table          | No       | Arbitrary key-value pairs (string, number, or boolean)   |

Metadata use cases:
- Scoring: `metadata = { score = 5 }` for risk calculation
- Categorization: `metadata = { description = "...", color = "red" }`
- Workflow flags: `metadata = { escalation_required = true }`

### Validation rules

The generated config must satisfy all of the following:

1. `[workspace] id` is present and matches `^[a-z0-9]+(-[a-z0-9]+)*$` (max 63 chars)
2. Every field `id` matches `^[a-z0-9]+(-[a-z0-9]+)*$`
3. Every field `name` is non-empty
4. Every field `type` is one of: `text`, `number`, `select`, `multi-select`, `user`, `multi-user`, `date`, `url`
5. No duplicate field IDs across the entire configuration
6. `select` and `multi-select` fields have at least one option
7. Every option `id` matches the same pattern and is unique within its parent field
8. Every option `name` is non-empty

### Complete example

```toml
[workspace]
id = "risk"
name = "Risk Management"

[labels]
case = "Risk"

[slack]
channel_prefix = "risk"

[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true
options = [
  { id = "data-breach", name = "Data Breach", metadata = { description = "Risk of personal or confidential information leakage", color = "red" } },
  { id = "system-failure", name = "System Failure", metadata = { description = "Risk of system or service downtime and failures", color = "orange" } },
  { id = "compliance", name = "Compliance", metadata = { description = "Risk of regulatory or internal policy violations", color = "yellow" } },
]

[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
options = [
  { id = "critical", name = "Critical", metadata = { score = 5 } },
  { id = "high", name = "High", metadata = { score = 4 } },
  { id = "medium", name = "Medium", metadata = { score = 3 } },
  { id = "low", name = "Low", metadata = { score = 2 } },
  { id = "negligible", name = "Negligible", metadata = { score = 1 } },
]

[[fields]]
id = "assignee"
name = "Assignee"
type = "user"
required = false

[[fields]]
id = "deadline"
name = "Deadline"
type = "date"
required = false

[[fields]]
id = "reference-url"
name = "Reference URL"
type = "url"
required = false
```
