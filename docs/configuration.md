# Configuration Guide

Hecatoncheires is configured through a combination of a TOML configuration file and CLI flags (or environment variables). This guide is the complete reference for the `config.toml` file.

## Configuration File (config.toml)

The application requires a TOML configuration file at startup. This file defines custom fields for cases and display labels for entities.

- Default path: `./config.toml`
- Override with `--config` flag or `HECATONCHEIRES_CONFIG` environment variable
- The file **must exist** at startup; a missing file causes an error

### Basic Structure

```toml
# Workspace configuration (required)
[workspace]
id = "risk"
name = "Risk Management"

# Entity display labels (optional)
[labels]
case = "Risk"

# Custom field definitions
[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
options = [
  { id = "high", name = "High" },
  { id = "low", name = "Low" },
]

# Slack integration (optional)
[slack]
channel_prefix = "risk"

# AI assist configuration (optional)
[assist]
prompt = "Check action deadlines and follow up on pending items."
language = "Japanese"
```

---

## Workspace Section

The `[workspace]` section defines the workspace's identity and is **required** in each configuration file.

```toml
[workspace]
id = "risk"
name = "Risk Management"
emoji = "🛡️"   # optional; mutually exclusive with color
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique workspace identifier. Must match `^[a-z0-9]+(-[a-z0-9]+)*$` and be at most 63 characters |
| `name` | string | No | Display name for the workspace. Defaults to `id` if omitted |
| `emoji` | string | No | Badge glyph shown in the workspace selector and switcher. Mutually exclusive with `color`. Up to 16 runes |
| `color` | string | No | Badge background color as a `#RRGGBB` hex code (6-digit only). Mutually exclusive with `emoji` |

### Workspace Badge (emoji / color)

Each workspace is shown as a small badge in the workspace selector and the
breadcrumb switcher. You can control its appearance with **either** `emoji`
**or** `color` — they are mutually exclusive, and setting both fails validation
at startup.

- **`emoji`** — renders the emoji on a neutral background (e.g. `emoji = "🛡️"`).
- **`color`** — renders the workspace's initials on a gradient derived from the
  given `#RRGGBB` color (e.g. `color = "#2cb38d"`). Only the 6-digit hex form is
  accepted; `#fff` and named colors are rejected.
- **Neither** — the initials are shown on an automatically assigned color that
  is derived deterministically from the workspace `id`. The same workspace
  always gets the same color regardless of how many workspaces exist or their
  order, so the selector and switcher stay consistent.

### Workspace ID Format

Workspace IDs must follow the same format as field IDs:

- Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`
- Maximum length: 63 characters
- Must be unique across all workspaces

| Example | Valid | Reason |
|---------|-------|--------|
| `risk` | Yes | Simple lowercase |
| `security-team` | Yes | Hyphen-separated |
| `workspace-123` | Yes | With numbers |
| `MyWorkspace` | **No** | Uppercase not allowed |
| `workspace_1` | **No** | Underscores not allowed |
| `workspace.name` | **No** | Dots not allowed |

---

## Labels

The `[labels]` section customizes entity display names in the UI:

```toml
[labels]
case = "Risk"        # Default: "Case"
```

| Key | Default | Description |
|-----|---------|-------------|
| `case` | `Case` | Display name for the primary entity |

If omitted or empty, the default values are used.

---

## Field Definitions

Each field is defined as an element of the `[[fields]]` array:

```toml
[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
description = "Overall severity assessment"
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique identifier. Must match `^[a-z][a-z0-9_]*$` |
| `name` | string | **Yes** | Display name shown in the UI |
| `type` | string | **Yes** | Field type (see [Field Types](#field-types)) |
| `required` | boolean | No | Whether the field is required (default: `false`) |
| `description` | string | No | Help text shown in the UI |
| `reference_workspace` | string | Case-reference only | Target workspace ID whose Cases this field references. **Required** for `case_ref` / `multi_case_ref`, and rejected for every other type. Must name a configured workspace (self-reference allowed) |

### Field ID Format

Field IDs must start with a lowercase letter and consist of lowercase letters,
digits, and underscores:

- Pattern: `^[a-z][a-z0-9_]*$`
- Must be unique across all fields in the configuration

> **Breaking change (Apr 2026)**: Field IDs and Option IDs no longer accept
> hyphens (`-`). The new pattern aligns with Go identifier rules so that
> Slack welcome message templates can reference custom field values via dot
> notation (e.g., `{{.Fields.risk_level}}`). Existing deployments that store
> hyphenated FieldIDs in Firestore must migrate the data themselves; the
> application provides no automatic migration. Workspace IDs are unaffected
> and continue to follow the legacy hyphen-separated format.

| Example | Valid | Reason |
|---------|-------|--------|
| `category` | Yes | Simple lowercase |
| `risk_level` | Yes | Underscore-separated |
| `my_field_123` | Yes | With numbers |
| `risk-level` | **No** | Hyphens are no longer allowed |
| `MyField` | **No** | Uppercase not allowed |
| `1category` | **No** | Cannot start with a digit |
| `field.name` | **No** | Dots not allowed |
| `_leading` | **No** | Cannot start with underscore |

---

## Field Types

### `text`

Single-line text input.

```toml
[[fields]]
id = "description"
name = "Description"
type = "text"
```

### `number`

Numeric input.

```toml
[[fields]]
id = "score"
name = "Risk Score"
type = "number"
```

### `select`

Single selection from a predefined list of options. Requires at least one option.

```toml
[[fields]]
id = "priority"
name = "Priority"
type = "select"
required = true
options = [
  { id = "high", name = "High" },
  { id = "medium", name = "Medium" },
  { id = "low", name = "Low" },
]
```

### `multi-select`

Multiple selections from a predefined list of options. Requires at least one option.

```toml
[[fields]]
id = "tags"
name = "Tags"
type = "multi-select"
options = [
  { id = "urgent", name = "Urgent" },
  { id = "review_needed", name = "Review Needed" },
]
```

### `user`

Single Slack user reference. Rendered with user avatar in the UI. Requires Slack integration to be configured.

```toml
[[fields]]
id = "assignee"
name = "Assignee"
type = "user"
```

### `multi-user`

Multiple Slack user references. Requires Slack integration.

```toml
[[fields]]
id = "stakeholders"
name = "Stakeholders"
type = "multi-user"
```

### `date`

Date picker input.

```toml
[[fields]]
id = "deadline"
name = "Deadline"
type = "date"
```

### `url`

URL input with validation.

```toml
[[fields]]
id = "reference"
name = "Reference URL"
type = "url"
```

### `case_ref`

Reference to a single Case in another (or the same) workspace, selected through
a searchable dropdown. The target workspace is fixed by the `reference_workspace`
property (required); only Cases in that workspace can be referenced.

```toml
[[fields]]
id = "related_incident"
name = "Related Incident"
type = "case_ref"
reference_workspace = "incident-response"
```

### `multi_case_ref`

Reference to multiple Cases in the workspace named by `reference_workspace`
(required). Same rules as `case_ref`, but stores a list.

```toml
[[fields]]
id = "related_cases"
name = "Related Cases"
type = "multi_case_ref"
reference_workspace = "incident-response"
```

> **Private Cases are never referenceable.** A private Case in the target
> workspace is excluded from the picker, hidden from the agent search/detail
> tools, and rejected when set as a value — regardless of who is editing. Draft
> Cases are likewise not referenceable. `reference_workspace` may name the
> field's own workspace (self-reference is allowed), and is validated at startup
> against the set of configured workspaces.
>
> **Case-reference fields cannot be `required`.** The Slack case-creation modal
> has no element for a searchable cross-workspace case picker, so a required
> case reference would be un-fillable from Slack. Set them via the Web UI or
> agent tools after the Case exists; config load rejects `required = true` on
> these types.

### Summary

| Type | Description | Requires Options | Requires `reference_workspace` |
|------|-------------|-----------------|-----------------|
| `text` | Single-line text input | No | No |
| `number` | Numeric input | No | No |
| `select` | Single selection from options | **Yes** | No |
| `multi-select` | Multiple selections from options | **Yes** | No |
| `user` | Single Slack user reference | No | No |
| `multi-user` | Multiple Slack user references | No | No |
| `date` | Date picker | No | No |
| `url` | URL input | No | No |
| `case_ref` | Single Case reference in another workspace | No | **Yes** |
| `multi_case_ref` | Multiple Case references in another workspace | No | **Yes** |

---

## Options (for select / multi-select)

Fields of type `select` or `multi-select` must define at least one option using the `options` property.

### Option Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | **Yes** | Unique identifier within the field. Same format as field ID |
| `name` | string | **Yes** | Display name |
| `description` | string | No | Description of this option (surfaced in the UI's `(?)` help popover and option-hover tooltip) |
| `metadata` | table | No | Arbitrary key-value pairs |

Option IDs must be unique within their parent field and follow the same format as field IDs (`^[a-z][a-z0-9_]*$`).

`description` lives at the top level — it is a first-class part of the option contract, surfaced directly in the UI. The previous shape that placed `description` (or `color`) inside `metadata` is no longer supported; migrate to top-level `description`.

### Metadata

The `metadata` property allows attaching arbitrary key-value data to an option. Values can be strings, numbers, or booleans.

```toml
[[fields]]
id = "severity"
name = "Severity"
type = "select"
description = "Incident severity. Score (1–5) lives in metadata."
options = [
  { id = "critical", name = "Critical", description = "Requires immediate executive attention", metadata = { score = 5, escalation_required = true } },
  { id = "low", name = "Low", description = "Minimal impact", metadata = { score = 1 } },
]
```

Use cases for metadata:
- **Scoring**: Attach numeric scores for risk calculation
- **Workflow**: Flag options that trigger specific behaviors

For human-readable text shown in the UI, use top-level `description` — never store it under `metadata`.

---

## Memo Section

The optional `[memo]` section enables **case-scoped memos**: a per-Case memory
where humans and agents record facts, observations, hypotheses, and decisions
while working a Case. A memo always has an `id` and a `title`; everything else
is a custom field defined here, using the exact same field schema as case
`[[fields]]` (same types, options, and validation rules).

When `[memo]` is omitted the memo feature is disabled for the workspace: the
WebUI hides the Memos tab and agents are given no memo tools.

```toml
[memo]
# The "strong definition" of what this memo records. Shown in the WebUI and
# embedded into the agent system prompt so the agent knows what to write down.
description = """
This memo is the investigation memory for an incident case. Record facts,
observations, hypotheses, and decisions so later agent runs build on them.
Mark unverified guesses with a confidence so they are not mistaken for facts.
"""

# Memo custom fields. Identical schema to [[fields]] (id / name / type /
# required / description / options).
[[memo.fields]]
id = "memo_type"
name = "Type"
type = "select"
required = true
description = "Whether this memory is a verified fact or an inference."
options = [
  { id = "fact", name = "Fact", description = "Verified, backed by evidence." },
  { id = "observation", name = "Observation", description = "Read from logs/data." },
  { id = "hypothesis", name = "Hypothesis", description = "Unverified inference." },
  { id = "decision", name = "Decision", description = "A recorded decision." },
]

[[memo.fields]]
id = "body"
name = "Body"
type = "text"
description = "Free-form details, evidence, and next steps."

[[memo.fields]]
id = "evidence"
name = "Evidence"
type = "url"
description = "Link to the primary source (log, dashboard, PR)."
```

### Properties

| Property            | Description                                                            |
|---------------------|------------------------------------------------------------------------|
| `description`       | The strong definition of the memo, embedded into the agent system prompt and shown in the WebUI. |
| `[[memo.fields]]`   | Memo custom field definitions. Same schema and validation as `[[fields]]`. |

### Behavior

- **WebUI**: a "Memos" tab on the Case detail page lets users list, view,
  create, edit, and archive memos. Deletion is a soft delete (archive) and is
  restorable.
- **Agents**: every agent running in a Case context is given memo tools
  (`memo__list_memos`, `memo__get_memo`, `memo__create_memo`,
  `memo__update_memo`, `memo__archive_memo`), scoped to that Case. Create and
  update are validated against the memo field schema exactly like the WebUI
  path (required fields enforced, unknown field ids rejected). `memo__list_memos`
  excludes archived memos by default.
- **System prompt**: the `description` and the memo field schema are injected
  into the agent's system prompt, along with the id + title of up to 20 of the
  Case's active memos (and the total count when there are more). Full content is
  fetched on demand via the memo tools.

---

## Slack Section

The `[slack]` section customizes Slack integration settings. This section is optional.

```toml
[slack]
channel_prefix = "risk"
```

### Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `channel_prefix` | string | No | workspace ID | Prefix for auto-created Slack channel names (channel mode only) |
| `mode` | string | No | `channel` | Case binding mode: `channel` (one Case per dedicated channel) or `thread` (one Case per thread in a monitored channel) |
| `channel` | string | Conditional | — | The monitored Slack channel ID (e.g. `C0123456789`). **Required when `mode = "thread"`.** Use the channel **ID**, not the name |
| `accept_bot` | bool | No | `false` | Thread mode only. When `true`, **bot-authored** channel-root posts (e.g. an intake-form app's relayed request) also start a Case; every bot root post is picked up. When `false`, only human channel-root posts start a Case. |

When a case is created (channel mode), Hecatoncheires can automatically create a Slack channel with the naming pattern: `{channel_prefix}-{case_number}`.

If `channel_prefix` is not specified, the workspace ID is used as the default prefix.

**Thread mode:** When `mode = "thread"`, `channel_prefix`, `[slack.invite]`, and
`welcome_messages` are ignored (no dedicated channel is created). The monitored
`channel` must exist, the bot must be a member of it, and the app must subscribe
to the `message.channels` (and `message.groups` for private channels) events. A
`[case.status]` section is required in thread mode — see
[Case Section](#case-section-thread-mode). See also
[Slack Integration → Thread mode](slack.md#thread-mode-monitored-channel).

**Note:** The `--slack-channel-prefix` CLI flag can override this configuration for the entire serve command.

### Auto-Invite (`[slack.invite]`)

The `[slack.invite]` subsection configures automatic invitation of users and user group members to Slack channels when a case is created.

```toml
[slack.invite]
users = ["U12345678", "U87654321"]
groups = ["S0614TZR7", "@security-response"]
```

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `users` | string[] | No | `[]` | Slack user IDs to automatically invite to case channels |
| `groups` | string[] | No | `[]` | Slack user group IDs or `@`-prefixed handle names to resolve and invite |

**Group resolution:**
- If a group identifier starts with `@` (e.g., `@security-response`), it is treated as a handle name and resolved to a group ID via the `usergroups.list` API
- Otherwise (e.g., `S0614TZR7`), it is treated as a Slack user group ID and used directly
- Members of each group are fetched via `usergroups.users.list` and added to the invite list
- Group resolution failures are logged but do not prevent case creation
- Duplicate users (across direct users, group members, creator, and assignees) are automatically deduplicated

**Required bot scope:** `usergroups:read` (in addition to existing scopes)

### Welcome Messages (`welcome_messages`)

After the Slack channel is created and initial members are invited, Hecatoncheires can post a sequence of welcome messages to the channel. Each entry is a Go [`text/template`](https://pkg.go.dev/text/template) string that is rendered against the newly created Case.

```toml
[slack]
welcome_messages = [
  "<@{{.Case.ReporterID}}> Created Case `{{.Case.Title}}`.",
  """\
:rotating_light: Highlights
- Status: {{.Case.Status}}
- Severity: {{.Fields.severity.name}} ({{.Fields.severity.id}})
- Tags: {{range $i, $t := .Fields.tags.items}}{{if $i}}, {{end}}{{$t.name}}{{end}}
- Reporter: <@{{.Case.ReporterID}}>
- Assignees: {{range $i, $a := .Case.AssigneeIDs}}{{if $i}}, {{end}}<@{{$a}}>{{end}}
- Detail: {{.URL}}""",
]
```

**Behavior**

- Messages are sent in the order they appear in the array, after channel creation, member invitation, and bookmark addition.
- Templates are parsed at startup; an invalid template aborts the application start with a configuration error.
- A template that produces an empty string at runtime is skipped (useful for conditional messages with `{{if ...}}...{{end}}`).
- A failure to render or post a single message is logged via `errutil.Handle` and does **not** roll back case creation; subsequent messages still attempt to post.

**Available template variables**

| Variable | Type | Notes |
|----------|------|-------|
| `.Case.ID` | int64 | Case sequence number |
| `.Case.Title` | string | Case title |
| `.Case.Description` | string | Case description |
| `.Case.Status` | CaseStatus | Normalized status string |
| `.Case.ReporterID` | string | Slack user ID of the reporter |
| `.Case.AssigneeIDs` | []string | Slack user IDs of assignees |
| `.Case.SlackChannelID` | string | The freshly-created channel ID |
| `.Case.IsPrivate` | bool | Whether the case is private |
| `.Case.CreatedAt` | time.Time | Creation timestamp (UTC) |
| `.Workspace.ID` | string | Workspace identifier |
| `.Workspace.Name` | string | Workspace display name |
| `.Fields` | map[string]map[string]any | Custom field values keyed by Field ID — each entry exposes `id` and `name` (and `items` for multi-select) |
| `.URL` | string | Web UI Case URL when `--base-url` is configured (otherwise empty) |

**Field value structure**

Each Field is exposed as a sub-map with the following sub-keys:

| Sub-key | Description |
|---------|-------------|
| `id` | Raw stored value. For `select`, the option ID. For `multi-select`, an array of option IDs. For `text`/`number`/`date`/`url`/`user`/`multi-user`, the raw stored value. |
| `name` | Display name. For `select`/`multi-select`, the Option `name` from the schema (falls back to the ID when not found). For other field types, mirrors `id`. |
| `items` | `multi-select` only. Slice of `{id, name}` maps for iteration in templates. |

Examples:

```text
Severity: {{.Fields.severity.name}}                        ← "High"
Severity ID: {{.Fields.severity.id}}                        ← "high"
Tags: {{range $i, $t := .Fields.tags.items}}{{if $i}}, {{end}}{{$t.name}}{{end}}
Note: {{.Fields.note.id}}                                   ← bare text value
```

Slack mrkdwn syntax such as `<@USER_ID>` and `<#CHANNEL_ID>` is rendered as-is and expanded by Slack at delivery time.

### Private Case Channels

When a case is created with the **Private** flag enabled, the associated Slack channel is created as a **private channel** instead of a public one. This ensures that only invited members can view the channel content.

Private cases also track channel membership:
- Channel member IDs are stored on the case and used for access control — only channel members can view the case details, actions, and assist logs associated with a private case.
- Members can be synced from the Slack channel via the **Sync** button on the case detail page or through the `syncCaseChannelUsers` GraphQL mutation.
- Bot users are automatically filtered out from the member list.

---

## Assist Section

The `[assist]` section configures the AI-powered case assistance feature. This section is optional.

```toml
[assist]
prompt = "Check action deadlines and follow up on pending items."
language = "Japanese"
```

### Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `prompt` | string | No | Custom prompt for the AI assistant when analyzing cases |
| `language` | string | No | Preferred language for AI responses (e.g., "Japanese", "English") |

The `assist` command runs an AI agent that periodically reviews open cases and provides insights, reminders, and suggestions via Slack.

**Important:** If the `prompt` field is empty or the `[assist]` section is omitted, the workspace will be skipped during assist command execution.

### Usage

The assist feature requires:
- An LLM provider (via `--llm-provider` and the matching credential flags). Supported providers: `openai`, `claude` (direct Anthropic API or Vertex AI), `gemini`.
- Slack Bot Token (via `--slack-bot-token`)

Run the assist command (Gemini on Vertex AI example):

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=gemini \
  --llm-gemini-project-id=YOUR_GCP_PROJECT \
  --llm-gemini-location=global \
  --workspace=risk
```

Or with OpenAI:

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=openai \
  --llm-openai-api-key=sk-YOUR_OPENAI_KEY \
  --workspace=risk
```

Or with Claude on Vertex AI:

```bash
hecatoncheires assist \
  --slack-bot-token=xoxb-YOUR_BOT_TOKEN \
  --llm-provider=claude \
  --llm-gemini-project-id=YOUR_GCP_PROJECT \
  --llm-gemini-location=us-east5 \
  --workspace=risk
```

The AI agent will:
1. Retrieve recent assist logs and Slack messages for context
2. Analyze open cases in the specified workspace
3. Execute the custom prompt to generate insights
4. Post findings to the case's Slack channel

### Agent tool registry (Slack mention & assist)

> **The full tool catalogue — every tool name, what it does, and which runtime
> context (Job vs. interactive mention agent) exposes it — lives in
> [Agent Tools](agent_tools.md).** Read it before naming a tool in any prompt;
> the palette a Job gets is narrower than the interactive agent's. The table
> below is only a quick reference for *which optional service enables which
> tool group* for the interactive mention / assist agent.

| Tool group | Enabled by |
|------------|------------|
| `core__*` (actions) + `case__*` (case edits) | Always (a case context exists). |
| `slack__search_messages` | `HECATONCHEIRES_SLACK_USER_OAUTH_TOKEN` with the `search:read` scope. See [docs/slack.md](slack.md#user-token-scopes). |
| `slack__get_messages`, `slack__post_message` | `HECATONCHEIRES_SLACK_BOT_TOKEN`. |
| `notion__search`, `notion__get_page` | `HECATONCHEIRES_NOTION_API_TOKEN`. See [docs/integrations.md](integrations.md). |
| `github__*` | The `--github-app-*` flags. See [docs/integrations.md](integrations.md). |
| `webfetch` | A configured web-fetch client. |
| `knowledge__*` | Always (write is withheld on private cases). |
| `memo__*` | A `[memo]` section with at least one memo field defined. |

---

## Action Section

The `[action]` section is **optional**. When omitted, the workspace inherits a built-in default set of action statuses (`BACKLOG`, `TODO`, `IN_PROGRESS`, `BLOCKED`, `COMPLETED`) so that data written before configurable statuses keeps working unchanged. Define this section to tailor the action workflow to your team.

```toml
[action]
initial = "queued"            # Required when [action] is present
closed  = ["done", "cancelled"]   # Optional, defaults to []

[[action.status]]
id = "queued"
name = "Queued"
description = "Awaiting triage"
color = "idle"                 # See "Color values" below
emoji = "📋"

[[action.status]]
id = "doing"
name = "Doing"
color = "active"
emoji = "▶️"

[[action.status]]
id = "done"
name = "Done"
color = "success"
emoji = "✅"
```

### `[action]` keys

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `initial` | string | **Yes** | ID of the status assigned to newly created actions. Must match one of the `[[action.status]]` IDs. |
| `closed` | string[] | No | IDs treated as "closed" for filtering / completion checks. Each ID must match a defined status. Defaults to `[]`. |

### `[[action.status]]` keys

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `id` | string | **Yes** | Stable identifier. Must match `^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$` and be unique within the workspace. Stored in the database as the action's status. |
| `name` | string | **Yes** | Display label. Write it in whichever language fits the workspace — there is no localization layer. |
| `description` | string | No | Free-form description (currently unused in the UI; reserved for tooltips). |
| `color` | string | No | UI color. See "Color values" below. Defaults to the neutral `idle` preset. |
| `emoji` | string | No | Single emoji used in Slack messages and web UI badges. |

### Color values

The `color` field accepts **two and only two** kinds of value. Any other string fails validation at startup.

#### 1. Semantic preset name (recommended)

Each preset expresses the **kind** of state, not a specific named status. Pick the preset that matches your status's intent — preset names are matched case-insensitively.

| Preset | Intended meaning | Suggested for statuses like |
|--------|------------------|-----------------------------|
| `idle` | Not started yet, queued, reserved | `backlog`, `queued`, `new` |
| `active` | Work in progress, someone is on it | `in_progress`, `doing`, `wip` |
| `waiting` | Waiting on someone else (review / external response / dependency) | `in_review`, `waiting_for_user`, `qa` |
| `paused` | Intentionally on hold | `on_hold`, `parked` |
| `attention` | Needs attention / something off-track | `needs_input`, `flagged` |
| `blocked` | Cannot proceed; something is preventing progress | `blocked`, `dependency_failed` |
| `success` | Completed in the desired way | `done`, `resolved`, `shipped` |
| `neutral_done` | Closed without value judgment (won't fix, expired, withdrawn) | `cancelled`, `wont_fix`, `expired` |
| `failure` | Closed in an undesired way (failed, rejected, abandoned) | `failed`, `rejected`, `lost` |

Presets resolve to CSS variables (`--action-status-<preset>`) on the frontend and follow the active light/dark theme.

#### 2. Hex color code

When no preset captures what you need, supply an absolute color as `#RRGGBB` or `#RGB`. Example: `"#5EAEDC"`, `"#abc"`. Hex codes do **not** follow the theme, so pick a color that reads on both light and dark backgrounds.

#### Not supported (will fail validation)

- CSS variable references like `"var(--ok)"` — the supported entry point is the preset list above.
- CSS color keywords like `"red"`, `"blue"`.
- Function notations: `rgb(...)`, `rgba(...)`, `hsl(...)`, `oklch(...)`.
- Any string that is neither a preset name nor `#RRGGBB`/`#RGB`.

#### Examples

| Value | Result |
|-------|--------|
| `"success"` | ✅ Preset, theme-aware green |
| `"WAITING"` | ✅ Case-insensitive preset match |
| `"#5EAEDC"` | ✅ Absolute hex |
| `"#abc"` | ✅ Short hex |
| (omitted) | ✅ Falls back to `idle` |
| `"var(--ok)"` | ❌ Validation error |
| `"red"` | ❌ Validation error |
| `"rgb(255,0,0)"` | ❌ Validation error |

---

## Case Section (thread mode)

The `[case]` section configures the status set that attaches to **Cases** in
thread mode (`[slack] mode = "thread"`). It is **required** for thread-mode
workspaces and **ignored** (with a startup warning) in channel mode, where the
configurable status set lives on Actions instead (`[action]`).

The shape mirrors `[action]` exactly — the same keys, the same color values, and
the same validation rules apply (see [Action Section](#action-section)).

```toml
[slack]
mode = "thread"
channel = "C0123456789"   # the monitored channel ID

[case]
initial = "triage"            # Required: status assigned to a newly created Case
closed  = ["done", "wontfix"] # IDs treated as closed (closes the Case via lifecycle sync)

[[case.status]]
id = "triage"
name = "Triage"
color = "active"
emoji = "🩺"

[[case.status]]
id = "investigating"
name = "Investigating"
color = "waiting"

[[case.status]]
id = "done"
name = "Done"
color = "success"
emoji = "✅"
```

The Kanban board renders one column per `[[case.status]]` for thread-mode
workspaces; dragging a Case into a `closed` column closes the Case. The
investigation agent can also move a Case to a closed status when a mention
indicates the issue is resolved.

### Case agent prompts (`[case.prompts]`)

Thread-mode case **initialization** is agent-driven and is started **only** by
a post at the channel root. The root post may be authored by a human **or** by
an integration bot — e.g. an intake-form app that relays a request on a person's
behalf. **Bot-authored root posts are opt-in**: they start a Case only when the
workspace sets `[slack] accept_bot = true` (default `false`, so a
channel is not flooded with a Case per bot notification); human root posts always
start a Case. (A mention or a reply inside a thread that has no Case yet is
ignored — only a channel-root post starts a Case.) The Case **reporter** is, as a
rule, the post's author. Only when the root post is bot-authored (so it has no
human author) is the reporter taken from the first Slack user mention in the body
(typically the requester named in the form); if the post names no user, the
reporter is left **empty** — a thread-mode Case is allowed to have no reporter,
so creation still proceeds. When such a root post arrives, the bot does **not**
create a Case immediately.
Instead it runs a plan-and-execute agent that investigates
(all read-only search tools are available), may ask the reporter to clarify
intent (a question form posted to the thread), and only commits a Case once it
can fill a valid title, description, and every **required** custom field. If
validation fails — a required field missing, or a value outside the allowed
options — the agent is told what is wrong and tries again, all bounded by the
planner round budget. A pending question is answered by submitting the question
form (not by a free-text reply). When the agent cannot conclude within budget,
it posts a "couldn't conclude" notice. On success the bot posts a Block Kit
summary of the created Case.

The optional `[case.prompts]` sub-table injects workspace-specific guidance into
this agent. Today only the `create` key (the initialization agent) is consumed;
`mention` / `close` are reserved for future phases.

```toml
[case.prompts]
create = """
For security incidents, always set the severity field and capture the affected
service in the description before creating the case.
"""
```

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `create` | string | No | Appended to the case-initialization agent's system prompt as "Workspace-specific instructions". Capped at 16384 bytes; startup fails if exceeded. |

---

## Job Definitions (`[[job]]`)

Agent Jobs let workspace administrators declaratively wire LLM-powered automation to Case lifecycle events and periodic ticks. Each Job is defined in the workspace TOML, listens to one or more events, and runs the Plan-and-Execute agent runtime with a fixed system-prompt structure and a curated tool palette (read-only + writer).

```toml
# A minimal lifecycle Job.
[[job]]
id = "summarize_on_create"
name = "Auto-summarize on creation"
description = "Summarize a new case and post the summary to Slack."
events.case = { on = ["created"] }
prompt = """
Summarize this case in three lines or fewer and post it to the Slack channel
bound to the case via slack__post_to_case_channel.
"""

# A multi-trigger Job: fires on lifecycle events AND every hour.
[[job]]
id = "watch"
events.case = { on = ["created", "closed"] }
events.scheduled = { every = "1h" }
prompt = "Take any appropriate action..."

# A cron-based scheduled Job.
[[job]]
id = "daily_digest"
events.scheduled = { cron = "0 9 * * *" }  # 09:00 UTC every day
prompt = "Post a status digest to the case Slack channel."
```

### Fields

| Field         | Type     | Required | Notes |
|---------------|----------|----------|-------|
| `id`          | string   | yes      | Workspace-unique, snake_case (`^[a-z0-9]+(_[a-z0-9]+)*$`). |
| `name`        | string   | no       | Human-readable label for logs. |
| `description` | string   | no       | Free-form description for operators. |
| `prompt`      | string   | (\*\*)   | Inline prompt. Go `text/template`. Has access to `.Case`, `.Workspace`, `.Event`. |
| `prompt_file` | string   | (\*\*)   | Path to a file holding the prompt, resolved relative to this config file's directory. Use it when the prompt is too long to inline comfortably. |
| `disabled`    | bool     | no       | Defaults to `false` (= active). Set `true` to temporarily disable. |
| `quiet`       | bool     | no       | Defaults to `false`. Set `true` to suppress the operational Slack session log (see *Session log* below). |
| `strategy`    | string   | no       | `"simple"` (default) or `"planexec"`. See *Execution strategy* below. |
| `events.case` | table    | (\*)     | `on = ["created" \| "closed", ...]`. Always an array. |
| `events.scheduled` | table | (\*)   | Exactly one of `every = "1h"` or `cron = "0 9 * * *"`. |

(\*\*) Exactly one of `prompt` or `prompt_file` must be set; supplying both, or neither, fails at config load time.

### Prompt source (`prompt` vs `prompt_file`)

Provide the prompt either inline via `prompt` or from an external file via
`prompt_file` — never both. `prompt_file` is resolved **relative to the
directory of the config file that declares the Job**, so a prompt can live
next to its workspace TOML and grow without bloating it:

```toml
[[job]]
id = "deep_investigation_on_create"
prompt_file = "prompts/deep_investigation.md"  # relative to this config.toml
events.case = { on = ["created"] }
```

The file's contents are loaded at startup and behave exactly like an inline
`prompt` (Go `text/template` with access to `.Case`, `.Workspace`, `.Event`);
trailing whitespace is trimmed. A missing or empty file fails loudly at load
time.

### Execution strategy

The `strategy` field selects which runtime drives the Job's LLM loop.
Defaults to `simple`, which is the v1 behaviour: a single
`gollem.Agent.Execute` call with the configured tool set. Set it to
`planexec` to opt into the plan-and-execute runtime shared with the
proposal mode — a planner LLM emits a list of parallel sub-agent tasks,
the runtime fans them out, replans with the observations, and finishes
with a structured summary.

```toml
[[job]]
id = "deep_investigation_on_create"
prompt = "Investigate the case from every angle and summarise findings."
strategy = "planexec"
events.case = { on = ["created"] }
```

| Strategy | When to use |
|----------|-------------|
| `simple` (default) | Single-step actions: post a digest, set a status, send a Slack reply. The Job's prompt is a direct instruction the agent executes in one ReAct loop. |
| `planexec` | Multi-step investigations: pull context from several sources, cross-reference, and produce a structured summary. The runtime budgets up to 8 planner rounds and 16 parallel sub-agent tasks per turn (configurable in the binary). |

Notes:

- `planexec` Jobs run **unattended** — they cannot ask the operator a
  question mid-run. Use `simple` if your Job is interactive in spirit
  but happens to need a small amount of planning; the planexec
  Question feature is reserved for the proposal mode.
- `planexec` Jobs surface their per-phase progress through the same
  JobRunLog event trail as `simple`, so the Cases UI shows you the
  plan, sub-agent activity, and final summary in order.
- The JobRunLog `executorKind` field is recorded as `single_loop`
  (simple) or `plan_execute` (planexec). The Cases UI renders the
  Run row with a `planexec` chip when the Job ran under the
  plan-and-execute runtime.

(\*) At least one of `events.case` / `events.scheduled` must be present.

### Events and scheduling

A Job subscribes to one or both event domains, and the runtime fires **one
invocation per `(job, case)` pair** — never one invocation for a whole
workspace. This per-case model is the single most important thing to internalise
when writing a Job prompt: your prompt runs against **one case at a time**, with
that case already loaded into the system prompt as `.Case`. **Do not** write a
prompt that tries to enumerate or loop over cases — the agent has no tool to
list other cases, and it doesn't need one.

**`events.case`** — fires the moment a case lifecycle transition is persisted:

| `on` value | Fires when |
|------------|-----------|
| `created`  | A new (published) case is created. |
| `closed`   | A case is moved to a closed status. |

**`events.scheduled`** — a periodic sweep (driven by `hecatoncheires tick` or
`POST /hooks/tick`) decides which Jobs are due. Exactly one of:

| Key | Format | Meaning |
|-----|--------|---------|
| `every` | Go duration string (`"24h"`, `"6h"`, `"30m"`, `"1h30m"`) | Fire when at least this long has elapsed since the last successful run for that `(job, case)`. |
| `cron`  | 5-field cron expression (`min hour day-of-month month day-of-week`), **UTC** | Fire on the cron schedule. No seconds field and no `@`-descriptors (`@hourly` etc.). |

Key facts that are easy to get wrong:

- **Scheduled Jobs run only against OPEN cases** — cases that are still open and
  published. Closed (`CLOSED`) and draft cases are skipped entirely, so a daily
  digest Job will simply stop firing for a case once it closes.
- **Cron is always UTC.** To fire at 08:30 Asia/Tokyo (JST = UTC+9), write
  `"30 23 * * *"`. There is no per-Job timezone setting.
- **The first scheduled run is always due**, regardless of `every` / `cron`,
  because there is no prior run to measure against.
- A Job may subscribe to **both** domains at once (e.g. react to `created` *and*
  sweep `every = "1h"`); each matching event produces its own invocation.

The operational mechanics of the sweep — concurrency leasing, duplicate
suppression, the `tick` CLI vs. the HTTP hook — are covered in
[Operations → Agent Jobs operations](operations.md#agent-jobs-operations).

### Session log

Each Job run posts a minimal operational log to the Case's Slack channel so
operators can see the agent working in real time. The log is intentionally
sparse — a starting marker, a few progress lines, and a completion / failure
marker — not a play-by-play of every LLM call.

- **Starting marker** — when the run begins, the runtime posts a
  `starting... <job_id>` message. This message roots the run's **session
  thread**: all subsequent log lines for that run are replies to it.
  - *Channel-mode Cases* (the Case owns a dedicated channel): the marker is
    a new channel-root message; progress and completion replies thread under
    it.
  - *Thread-mode Cases* (the Case lives in a Slack thread): the marker is a
    reply in the Case thread, and the Case thread doubles as the session
    thread (Slack nests only one level).
- **Progress lines** — the first time the run executes each distinct tool, a
  single line is posted to the session thread. Repeat calls to the same tool
  stay silent, so the log stays short.
- **Completion / failure marker** — when the run finishes, a success or
  failure marker (with the error text on failure) is posted to the session
  thread.

The agent's own `slack__post_message` tool is **not** part of the session
log: it remains a deliberate agent action, posted where the agent directs
(channel root in channel-mode Cases), and is never suppressed by `quiet`.

Set `quiet = true` to disable the entire session log (starting marker,
progress lines, and completion / failure marker) for a Job — useful for
high-frequency or purely background Jobs whose Slack chatter would be noise.
The run still executes and records its full trace in the Cases UI. The
session log also no-ops automatically when the deployment has no Slack
service wired (e.g. the scheduled-tick CLI) or when the Case has no bound
Slack channel.

### System prompt

The runtime constructs a structured system prompt every invocation. The
contents are fixed by the runtime — Job authors only control
`prompt` (the user message).

| Section                  | Content |
|--------------------------|---------|
| Role                     | Agent role and tone (fixed text). |
| Current time             | The turn's execution start time in UTC RFC3339, so the agent has an absolute "now" to reason about recency and elapsed time. Omitted only when the runtime did not supply a timestamp. |
| Workspace                | `id`, `name`, `description`, and the custom field schema. Each field's `id`, `name`, `type`, `required` flag (when true), `description` (when set), and — for `select` / `multi-select` — every option's `id`, `name`, `description`, and freeform metadata pairs are emitted so the agent knows the field's constraints and the meaning of each option ID. |
| Case                     | All persisted fields of the current case. For `field_values`, `select` / `multi-select` entries are rendered as `id: <raw> (<option name>, ...)` so the agent can map raw option IDs back to their human-readable label without re-consulting the schema. Unknown option IDs fall back to the raw value. |
| Per-case operator notes  | Rendered only when the Case has `AgentAdditionalPrompt` set (see below). |
| Actions                  | Existing non-archived actions, for de-duplication. |
| Trigger condition        | The Job's declared subscription (events.case / events.scheduled). |
| Trigger reason           | The concrete event that fired this invocation (timestamps, actor, lifecycle / cron tick). |
| Guardrails               | Fixed restrictions the runtime injects. See [Guardrails](#guardrails) below. |
| Tools                    | gollem auto-injects the resolved tool list — see [Agent Tools](agent_tools.md). |

### Guardrails

The runtime injects a fixed set of restrictions into every Job's system prompt.
These are **not** configurable from the TOML, and a Job prompt cannot grant
itself more than they allow (a per-case *Additional prompt* cannot either — it
is extra context, not extra capability):

- **A Job will not close a case.** Closing is a human-only decision. The agent
  is told not to move a case to a closed status, even though
  `case__update_case_status` could technically do it. (The interactive,
  human-initiated mention agent *may* close a case — this restriction is
  specific to unattended Jobs.)
- **A Job cannot delete cases, archive actions, or delete action steps.**
- **A Job posts only to the case's own bound Slack channel** — never to an
  arbitrary channel.
- **A Job cannot read its own past run traces;** it judges idempotency from the
  current case state, action list, and Slack history. So a "don't repeat
  yourself" prompt must lean on the visible case state, not on memory of prior
  runs.

Which of these are enforced in code (the tool is absent) versus only by the
prompt (the tool exists but the agent is instructed not to use it that way) is
spelled out in [Agent Tools → Guardrails](agent_tools.md#guardrails). The
distinction matters: a prompt-only guardrail is a firm instruction, so do not
write a Job prompt that tries to argue the agent out of it.

### Per-case agent customisation

The Case Agent page (`/ws/{ws}/cases/{id}/agent`) lets operators attach
two Case-scoped knobs that the runtime applies on top of the TOML Job
definition:

- **Additional prompt**: a Markdown snippet (max 16,384 bytes) that is
  rendered into the `Per-case operator notes` section of the system
  prompt. It does **not** replace the TOML prompt or the Guardrails —
  treat it as extra context, not as a way to grant new capabilities.
- **Source allowlist** (`AgentSourceIDs`): when empty, every enabled
  Workspace Source remains in play. When non-empty, Source-aware tools
  MUST narrow themselves to the listed IDs. Unknown / deleted IDs are
  silently skipped at use time so a Source toggled off later does not
  invalidate the stored selection.

Both values round-trip through the `updateCaseAgentSettings` GraphQL
mutation. Drafts cannot carry agent settings (they have no Slack
channel and no Job runs against them).

---

## Validation Rules

The configuration file is validated at startup. The following rules are enforced:

| Rule | Error |
|------|-------|
| Configuration file must exist at the specified path | `ErrConfigNotFound` |
| All field IDs must match `^[a-z][a-z0-9_]*$` | `ErrInvalidFieldID` |
| All field names must be non-empty | `ErrMissingName` |
| Field type must be one of the 8 supported types | `ErrInvalidFieldType` |
| Field IDs must be unique across the entire configuration | `ErrDuplicateFieldID` |
| `select` and `multi-select` fields must have at least one option | `ErrMissingOptions` |
| All option IDs must match the same pattern as field IDs | `ErrInvalidFieldID` |
| All option names must be non-empty | `ErrMissingName` |
| Option IDs must be unique within their parent field | `ErrDuplicateOptionID` |
| Slack welcome message templates must parse | `ErrInvalidWelcomeMessage` |
| Action status IDs must match `^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$`, be at most 32 characters, and be unique within the workspace | (action status validation) |
| `[action] initial` must reference a defined `[[action.status]] id` | (action status validation) |
| Each entry in `[action] closed` must reference a defined `[[action.status]] id` | (action status validation) |
| `[[action.status]] color` must be a preset name or `#RRGGBB` / `#RGB` | (action status validation) |

If any validation fails, the application exits with a descriptive error message including the field ID and context.

---

## Complete Example

The following example configures Hecatoncheires as a security risk management system. See also [examples/config.toml](../examples/config.toml).

```toml
# Workspace configuration (required)
[workspace]
id = "risk"
name = "Risk Management"

# Customize entity labels to match your domain
[labels]
case = "Risk"

# Slack integration (optional)
[slack]
channel_prefix = "risk"

# Auto-invite users and groups to case channels (optional)
[slack.invite]
users = ["U12345678"]
groups = ["@security-response"]

# AI assist configuration (optional)
[assist]
prompt = "Check action deadlines and follow up on pending items. Remind the team of overdue tasks."
language = "Japanese"

# Category field: multi-select
[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true
description = "Risk classification. Multiple values may apply."
options = [
  { id = "data_breach", name = "Data Breach", description = "Risk of personal or confidential information leakage" },
  { id = "system_failure", name = "System Failure", description = "Risk of system or service downtime and failures" },
]

# Likelihood field: select with scoring (score lives in metadata)
[[fields]]
id = "likelihood"
name = "Likelihood"
type = "select"
required = true
description = "How likely the risk is to materialise."
options = [
  { id = "low", name = "Low", description = "Unlikely to occur (a few times per year)", metadata = { score = 2 } },
  { id = "high", name = "High", description = "Likely to occur (about once per week)", metadata = { score = 4 } },
]

# Simple fields without options
[[fields]]
id = "specific_impact"
name = "Specific Impact"
type = "text"

[[fields]]
id = "deadline"
name = "Deadline"
type = "date"

[[fields]]
id = "reference_url"
name = "Reference URL"
type = "url"
```

---

## See Also

- [CLI Flags & Environment Variables](cli.md) — server flags, environment variables, and the diagnosis command
- [Integrations](integrations.md) — GitHub source integration and Notion tools
- [Operations](operations.md) — observability (Sentry) and operational guidance
- [Slack](slack.md) — Slack app setup, scopes, and authentication
- [User Guide](user_guide.md) — drafts, import, action steps, case-draft, and agent Jobs usage
