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

### Summary

| Type | Description | Requires Options |
|------|-------------|-----------------|
| `text` | Single-line text input | No |
| `number` | Numeric input | No |
| `select` | Single selection from options | **Yes** |
| `multi-select` | Multiple selections from options | **Yes** |
| `user` | Single Slack user reference | No |
| `multi-user` | Multiple Slack user references | No |
| `date` | Date picker | No |
| `url` | URL input | No |

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

The agent's available tools depend on which optional services are wired up.
Tools are split across three packages: `pkg/agent/tool/core` (case domain
state), `pkg/agent/tool/slack` (Slack-backed tools), and `pkg/agent/tool/notion`
(Notion-backed tools). The table below lists what each one provides and which
configuration enables it.

| Tool | Enabled by | Purpose |
|------|------------|---------|
| `core__list_actions`, `core__get_action`, `core__create_action`, `core__update_action`, `core__update_action_status`, `core__set_action_assignee`, `core__archive_action`, `core__unarchive_action` | Always | Manage the case's action items. `core__list_actions` accepts an optional `include_archived` parameter (default `false`); archived actions are hidden from default views but retained for later restoration via `core__unarchive_action`. There is no destructive delete tool — the archive lifecycle replaces deletion. |
| `slack__post_message` | Assist only (`HECATONCHEIRES_SLACK_BOT_TOKEN`) | Post a message to the case's Slack channel. |
| `slack__get_messages` | `HECATONCHEIRES_SLACK_BOT_TOKEN` | Bulk fetch of one or more Slack messages and their thread context (max 10 per call, parallel, partial failure tolerated). |
| `slack__search_messages` | `HECATONCHEIRES_SLACK_USER_OAUTH_TOKEN` with `search:read` scope | Workspace-wide Slack message search. See [docs/slack.md](slack.md#user-token-scopes). |
| `notion__search`, `notion__get_page` | `HECATONCHEIRES_NOTION_API_TOKEN` | Notion title search and Markdown content retrieval. See [docs/integrations.md](integrations.md). |

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
id = "summarize-on-create"
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
id = "daily-digest"
events.scheduled = { cron = "0 9 * * *" }  # 09:00 UTC every day
prompt = "Post a status digest to the case Slack channel."
```

### Fields

| Field         | Type     | Required | Notes |
|---------------|----------|----------|-------|
| `id`          | string   | yes      | Workspace-unique, kebab-case. |
| `name`        | string   | no       | Human-readable label for logs. |
| `description` | string   | no       | Free-form description for operators. |
| `prompt`      | string   | yes      | Go `text/template`. Has access to `.Case`, `.Workspace`, `.Event`. |
| `disabled`    | bool     | no       | Defaults to `false` (= active). Set `true` to temporarily disable. |
| `strategy`    | string   | no       | `"simple"` (default) or `"planexec"`. See *Execution strategy* below. |
| `events.case` | table    | (\*)     | `on = ["created" \| "closed", ...]`. Always an array. |
| `events.scheduled` | table | (\*)   | Exactly one of `every = "1h"` or `cron = "0 9 * * *"`. |

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
id = "deep-investigation-on-create"
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

### System prompt

The runtime constructs a structured system prompt every invocation. The
contents are fixed by the runtime — Job authors only control
`prompt` (the user message).

| Section                  | Content |
|--------------------------|---------|
| Role                     | Agent role and tone (fixed text). |
| Workspace                | `id`, `name`, `description`, and the custom field schema. Each field's `id`, `name`, `type`, `required` flag (when true), `description` (when set), and — for `select` / `multi-select` — every option's `id`, `name`, `description`, and freeform metadata pairs are emitted so the agent knows the field's constraints and the meaning of each option ID. |
| Case                     | All persisted fields of the current case. For `field_values`, `select` / `multi-select` entries are rendered as `id: <raw> (<option name>, ...)` so the agent can map raw option IDs back to their human-readable label without re-consulting the schema. Unknown option IDs fall back to the raw value. |
| Per-case operator notes  | Rendered only when the Case has `AgentAdditionalPrompt` set (see below). |
| Actions                  | Existing non-archived actions, for de-duplication. |
| Trigger condition        | The Job's declared subscription (events.case / events.scheduled). |
| Trigger reason           | The concrete event that fired this invocation (timestamps, actor, lifecycle / cron tick). |
| Guardrails               | Fixed restrictions: no auto-close, no delete, channel-scoped Slack, etc. |
| Tools                    | gollem auto-injects the resolved tool list. |

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
