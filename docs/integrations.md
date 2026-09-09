# Integrations

Hecatoncheires can wire up external services to surface their content to the AI agent and to feed the Source ingestion pipeline. This document covers the integrations you can enable: Notion, GitHub, and Jira.

> **Scope note.** This page is about *enabling* the Notion, GitHub, and Jira
> services. It is **not** the complete agent-tool list — that lives in
> [Agent Tools](agent_tools.md). The **Notion** and **Jira** tools are wired
> into the interactive mention / investigation agents **and into unattended
> [Jobs](configuration.md#job-definitions-job)** (both modes). The **GitHub**
> tools remain interactive / investigation only — they are **not** available to
> Jobs. If you are writing a Job prompt, check
> [Agent Tools → Tools available by context](agent_tools.md#tools-available-by-context)
> for what a Job can actually call.

## Notion

Hecatoncheires integrates with Notion to surface Notion content to the AI agent through tools registered in `pkg/agent/tool/notion/`:

- `notion__search` — search pages and databases shared with the integration.
- `notion__get_page` — retrieve a page's content as Notion-flavored Markdown.
- `notion__get_database` — describe a database's columns and list the pages (rows) it holds.
- `notion__search_database` — search one database's rows by keyword and property value.

This document covers the Notion setup needed for those tools.

### 1. Create a Notion Internal Integration

1. Open <https://www.notion.so/profile/integrations> (or **Settings → Integrations → Develop your own integrations**).
2. Click **New integration**.
3. Fill in:
   - **Name**: e.g. `Hecatoncheires`.
   - **Associated workspace**: pick the workspace that owns the pages/databases you want to expose.
   - **Type**: **Internal**.
4. Under **Capabilities**, enable:
   - **Read content** — required for both `Search` and the Markdown content endpoint.
   - The other capabilities (Update content / Insert content / etc.) are **not** required for the agent tools.
5. Click **Save**.
6. Copy the **Internal Integration Token** (starts with `secret_…`). This is the value passed via `--notion-api-token` / `HECATONCHEIRES_NOTION_API_TOKEN`.

> Notion's official Markdown Content API works with both *public* and *internal* integrations as long as the integration has the **Read content** capability and the page is shared with it. Internal integrations are the recommended choice for self-hosted deployments because they do not require publishing the integration.

### 2. Share Pages / Databases with the Integration

Notion's permission model is opt-in: a page or database is invisible to the integration until it is explicitly shared.

For each top-level page or database you want the agent to see:

1. Open it in Notion.
2. Click **Share → Add connections** and select the integration you created above.
3. Notion grants the connection access to the page **and all of its descendants**, so it is usually enough to share a small number of root pages.

Pages or child blocks that are **not** shared with the connection will appear as `<unknown>` placeholders in the Markdown output (a documented Notion API limitation).

### 3. Configure the Server

Set the API token via flag or environment variable:

```bash
export HECATONCHEIRES_NOTION_API_TOKEN="secret_…"
./hecatoncheires serve
# or:
./hecatoncheires serve --notion-api-token secret_…
```

When the token is configured, you should see:

```
Notion service enabled
```

in the server logs at startup. The four agent tool registrations (`notion__search`, `notion__get_page`, `notion__get_database`, `notion__search_database`) light up automatically when the agent runs.

If the token is omitted, the Notion-backed agent tools are silently skipped and the server logs:

```
Notion API token not configured, Source features will be limited
```

### 4. API Surface Used by the Agent Tools

| Tool | Endpoint | Notes |
|------|----------|-------|
| `notion__search` | `POST /v1/search`, plus up to 5 `GET /v1/databases/{id}` to name the parents | Title-substring match across all pages and databases shared with the integration — one substring, not several keywords, and never page bodies. Each hit carries `read_tool`, naming the tool that reads it (`notion__get_page` or `notion__get_database`), `search_tool` on a database hit, and its parent. Ordering by `last_edited_time` and pagination via `start_cursor` / `next_cursor`. Capped at 100 results per call. |
| `notion__get_page` | `GET /v1/pages/{page_id}/markdown` | Returns Notion-flavored ("enhanced") Markdown rendered server-side by Notion. Requires `Notion-Version: 2026-03-11` (sent automatically by `pkg/agent/tool/notion/client.go`). |
| `notion__get_database` | `GET /v1/databases/{database_id}`, `GET /v1/data_sources/{data_source_id}`, then `POST /v1/data_sources/{data_source_id}/query` | Three calls, because Notion's 2025-09-03 API split moved a database's rows into data sources: the first reports the data sources, the second reports the column schema, the third lists one data source's rows. The schema call is skipped when paging through a listing (`start_cursor` set) unless `describe_properties` asks for choices. All send `Notion-Version: 2026-03-11`. |
| `notion__search_database` | The same three, with `filter` / `sorts` in the query body and `filter_properties` in its query string | Notion has no parent-scoped search endpoint — its own documentation says to use the data source query for that — so searching one database means filtering its rows here. The schema is read on every call, because the conditions are written against property names and types. |

#### About databases and data sources

`notion__search` reports databases alongside pages, but `notion__get_page` reads pages only — Notion answers a database id there with `400 validation_error: … is a database, not a page`. `notion__get_database` is what closes that gap: it returns the database's rows as `id` / `title` / `url` entries, and the agent then opens whichever row it needs with `notion__get_page`. Each search hit also carries `read_tool` naming the tool that reads it, so the routing is data the agent can follow rather than only prose in the tool descriptions.

Since Notion's 2025-09-03 API version, a database does not hold its rows directly; it holds one or more **data sources** that do. Almost every database has exactly one, and the tool queries it without being asked. When a database has several, the tool returns no rows and reports the `data_sources` list instead, so the agent can call again with `data_source_id` set to the one it wants.

#### Which database a search hit came from

`POST /v1/search` takes no parent filter — Notion's documentation says to use the data source query for that — so a workspace-wide search returns hits from everywhere the integration can see. To make those hits usable when only one database counts as evidence, each one carries:

- `parent_type` — `database`, `page`, `block`, `workspace`, or empty when Notion reported a kind this code does not model.
- `parent_id` — the parent's id, always present except for a workspace-level page. This is what an agent compares against the database it cares about.
- `parent_database_name` — the parent database's title, when the parent is a database.

Notion does not include the parent's title in a search result, so each name is one extra `GET /v1/databases/{id}`. Distinct parents are resolved once each and **capped at 5 per search**, because Notion rate-limits at roughly three requests a second: past the cap, and for a parent whose read fails, the id is still reported and the name is left empty. A name that cannot be read is logged (through `errutil.Handle`) and never fails the search — the hit remains usable through its id.

The search endpoint stays pinned to `Notion-Version: 2022-06-28`, which is the response shape the decoder is written against. That is also why a row's parent arrives as `database_id` rather than the `data_source_id` of Notion's 2025-09-03 split.

#### Searching one database's rows

`notion__search_database` narrows the rows of a single database. It takes the same `database_id` / `data_source_id` pair as `notion__get_database`, plus:

- `query` — keywords separated by whitespace (the ideographic space counts). A row must match **every** keyword. Each keyword is matched against the properties named in `search_properties`, defaulting to the row title.
- `search_properties` — which columns the keywords are matched against. A text column (`title`, `rich_text`, `url`, `email`, `phone_number`) matches a **substring**; a `select` or `status` column matches a **choice name exactly**, and `multi_select` matches "holds this choice exactly". Notion has no substring match for a choice column, so pass the choice as it is spelled in the schema. Any other column type is refused rather than matched loosely.
- `filter` — property-value conditions: `operator` (`and` / `or`), `conditions`, and `groups` of conditions. Each condition is `{property, operator, value}`, plus `value_type` for a formula or rollup and `aggregation` for a rollup over a list. The tool resolves the property name to its id, checks the operator against that column's type, and coerces the value (a number from `"42"`, a checkbox from `"true"`, a date from ISO 8601), so a wrong argument comes back named rather than as a Notion 400.
- `sorts` — ordering keys, each naming either a `property` or a `timestamp` (`created_time` / `last_edited_time`) plus a `direction`. Earlier entries take precedence; the direction defaults to ascending, as Notion's does.
- `properties` — the columns whose values each returned row carries, rendered as one line of text each (capped at 500 characters, lists at 10 entries). This is what lets an agent judge a row without opening it.

**Keywords and conditions share Notion's two-level nesting limit.** Keywords become an `and` of one part per keyword, and a part is an `or` across the search properties when several are named. A caller's own conditions merge into that same `and`; an `or` filter with no groups is wrapped into it. The one combination that is refused is a keyword search alongside an `or` filter that also carries groups — that would be a third level. The rejection says to write the keywords as conditions instead.

**Notion cannot search page bodies.** `POST /v1/search` matches titles only, and a data source query matches property values only, so a term that appears only in a page's body is not findable through the API at all. Naming the columns that carry such terms — a summary, a keyword list — in `search_properties` is how that gap is covered.

#### Reading a database's column schema

A database object reports only the id and name of each data source, never the columns. `notion__get_database` therefore also reads `GET /v1/data_sources/{id}` and returns:

- `property_schema` — every column's `name` and `type`, sorted by name. Notion returns the columns as a JSON object, whose order is not meaningful, so sorting makes two descriptions of the same data source identical.
- `operators_by_type` — the filter operators each of those types accepts. Only the types this data source actually uses are listed.
- `options` on a column, when `describe_properties` named it. Only `select`, `status` and `multi_select` columns have choices, and they are reported per request rather than for everything: a database in production can carry dozens of columns, and their choice lists together do not fit in an agent's context. One property's choices are capped at 50, with `options_truncated` saying so.

A `formula` or `rollup` column is reported with the union of the operators its possible result types accept: Notion's schema carries a formula's expression and a rollup's aggregation but never their result type, so which operators apply is not knowable until the caller declares it.

#### Telling "nothing matched" apart from "the call did not happen"

Every read tool's result carries `status` and `matched`:

| `status` | Meaning | What the agent should do |
|---|---|---|
| `ok` | The call ran. `matched` is the number of rows or hits in this response (Notion reports no total; `has_more` says whether more exist). | `matched` of 0 means nothing matched. |
| `invalid_request` | The arguments could not be turned into a Notion request — an unknown property name, an operator the column's type does not accept, a value of the wrong type. `message` says which, and `property_schema` is attached so the call can be repaired without asking again. | Fix the arguments and call again. Do not conclude that nothing matched. |
| `data_source_ambiguous` | The database holds several data sources and none was named. `data_sources` lists them. | Call again with `data_source_id`. |

A failure to reach Notion at all — no permission, page not shared, rate limited, Notion down — is **not** one of these. It is returned as an error, so the agent sees a failed tool call. That distinction is what lets a workflow with the rule "if there is no evidence in this database, hand the request to a person" behave correctly: an empty result and an unreachable database must not look the same.

#### About the Markdown Content API

The `GET /v1/pages/{page_id}/markdown` endpoint was introduced by Notion in early 2026 and is the supported way to retrieve a page's full content as a Markdown document in a single call. Hecatoncheires uses it because:

- It is dramatically more compact than the raw block-tree API for LLM consumption.
- File-based blocks (image / file / video / audio / PDF) are returned as pre-signed URLs; those URLs expire after a short period, so consumers must download attachments promptly if they intend to persist them.
- Pages exceeding ~20,000 blocks are returned with `truncated: true`. Hecatoncheires propagates this flag through `notion__get_page` so the agent can warn the user.

> The third-party `jomei/notionapi` Go client used elsewhere in the codebase does not expose this endpoint, so `pkg/agent/tool/notion/client.go` calls it directly with `net/http` while reusing the same API token. The Markdown endpoint and the Search endpoint live in the agent tool package because they are exclusively used by the agent — `pkg/service/notion` keeps only the Source-facing surface.

### 5. Operational Notes

- **Rate limits**: Notion enforces ~3 req/s averaged. The Notion service uses the `notionapi` library's built-in retry-on-429 (3 retries) for `Search` and `QueryUpdatedPages`. The Markdown endpoint does not use the library, but Notion returns standard 429 responses, which surface to the agent as an error string (the agent typically retries with a delay).
- **Token rotation**: rotating the Internal Integration Token requires re-deploying the server with the new token. There is no graceful refresh — the previous token is invalidated immediately.
- **Multi-instance safety**: the Notion client is stateless and safe to instantiate per process; no shared in-memory state is held across instances.

### 6. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Agent never offers `notion__search` | `HECATONCHEIRES_NOTION_API_TOKEN` not set, or the value is empty | Set the token; restart the server. |
| `notion__get_page` returns `non-2xx` with status 404 | Page is not shared with the integration | Open the page in Notion → **Share → Add connections** → select your integration. |
| `notion__search` returns no results despite knowing pages exist | Pages have not been shared with the integration | Same as above — sharing is per-tree, not workspace-wide. |
| `notion__get_page` returns `400 validation_error: … is a database, not a page` | A database id from `notion__search` was passed to the page tool | Pass it to `notion__get_database` instead; the tool descriptions steer the agent there. |
| `notion__get_database` returns no rows and a `message` about `data_source_id` | The database holds several data sources | Call again with `data_source_id` set to one of the ids listed under `data_sources`. |
| Markdown response has `truncated: true` and ends with `<unknown>` blocks | Page exceeds Notion's render limits, or blocks reference unshared child pages | Split the page, or share the referenced child pages with the integration. |
| Agent gets a "validation_error: invalid Notion-Version" | Running against a Notion enterprise tenant that pins a lower API version | This is unlikely under default Notion plans; contact Notion support if the response references a different `notion-version` constraint. |

## GitHub

Hecatoncheires uses a single GitHub App to power both the Source pipeline (PR/Issue ingestion) and the agent's GitHub tools (search, get_issue, get_pull_request, get_file, list_commits). Wiring up the App enables both at once — there is no separate flag for the agent tools.

### GitHub App Setup

1. Create a GitHub App at `https://github.com/settings/apps/new`
2. Grant the following permissions:
   - **Repository permissions**: Issues (Read), Pull Requests (Read), Contents (Read)
3. Install the App on the target organization or repositories
4. Note the App ID, Installation ID, and download the private key

### Configuration

All three flags (`--github-app-id`, `--github-app-installation-id`, `--github-app-private-key`) must be set to enable GitHub features (Source pipeline + agent tools). If any flag is missing, GitHub features are gracefully disabled and the application continues to run normally with other source types.

```bash
hecatoncheires serve \
  --github-app-id=12345 \
  --github-app-installation-id=67890 \
  --github-app-private-key=/path/to/private-key.pem \
  ...
```

The `--github-app-private-key` accepts either a file path to a PEM file or the PEM content directly as a string.

### Source Management

GitHub Sources are managed via the GraphQL API:

- `createGitHubSource` - Create a new GitHub source with repository list
- `updateGitHubSource` - Update an existing GitHub source
- `validateGitHubRepo` - Validate access to a repository before adding it

Repositories can be specified in `owner/repo` format or as full GitHub URLs (e.g., `https://github.com/owner/repo`).

### Agent Tools

When the GitHub App is configured, the Slack mention agent and the assist flow gain the following gollem tools:

| Tool | Purpose |
| --- | --- |
| `github__search` | Search issues and pull requests using GitHub search syntax (`repo:`, `is:open`, `author:`, `label:`, etc.). Up to 50 hits per call. |
| `github__get_issue` | Fetch a single issue (not PR) with its body, labels, and full comment thread. |
| `github__get_pull_request` | Fetch a single PR with body, labels, comments, and reviews. Optional `include_files=true` adds the diff (per-file patches truncated at 20 KB). |
| `github__get_file` | Fetch a file's content at any branch/tag/SHA. UTF-8 text only; binaries return `is_binary=true` with empty content. Capped at 1 MB. |
| `github__list_commits` | List commits with optional `path`, `author`, `since`, `until` filters. Up to 50 commits per call. |

The tools operate within whatever scope the GitHub App's installation grants — there is no per-repository allowlist on the application side.

## Jira

Hecatoncheires uses [`github.com/gollem-dev/tools/jira`](https://github.com/gollem-dev/tools) — a read-only Jira Cloud integration — for the agent's Jira tools (list projects, search issues, fetch issues). Unlike Notion and GitHub, Jira does not currently feed the Source ingestion pipeline; it is agent-tools only.

### 1. Generate a Jira API Token

1. Sign in to Jira Cloud with the account whose access you want the agent to use.
2. Open <https://id.atlassian.com/manage-profile/security/api-tokens>.
3. Click **Create API token**, name it (e.g. `hecatoncheires`), and copy the generated token. It is shown only once.

The agent's permissions are exactly the permissions of this account within your Jira site — there is no separate app-level scope to grant.

### 2. Configure the Server

All three flags are required together; if any is missing, Jira features are gracefully disabled and the server continues to run normally with other integrations.

```bash
hecatoncheires serve \
  --jira-base-url=https://your-domain.atlassian.net \
  --jira-email=you@example.com \
  --jira-api-token=your-api-token \
  ...
```

Or via environment variables:

```bash
export HECATONCHEIRES_JIRA_BASE_URL="https://your-domain.atlassian.net"
export HECATONCHEIRES_JIRA_EMAIL="you@example.com"
export HECATONCHEIRES_JIRA_API_TOKEN="your-api-token"
```

When all three are configured, you should see `Jira service enabled` in the server logs at startup. If any is omitted, the server logs `Jira not configured, Jira agent tools will be disabled` and the Jira-backed agent tools are silently skipped.

### 3. Agent Tools

When Jira is configured, every agent context (interactive mention agent, assist, investigation sub-agents, and Jobs — see [Agent Tools → Tools available by context](agent_tools.md#tools-available-by-context)) gains the following gollem tools:

| Tool | Purpose |
| --- | --- |
| `jira_list_projects` | List projects accessible to the account (id, key, name, type, lead), with pagination. |
| `jira_search_issues` | Search issues with JQL syntax; a `project` argument is spliced into the JQL via AND for convenience. Returns key, summary, status, type, assignee, priority, and updated time. |
| `jira_get_issues` | Fetch one or more issues by key/id in a single batch (up to 100); descriptions and optional comments are rendered from Jira's Atlassian Document Format to Markdown. |

Investigation sub-agents (proposal case-draft, thread-mode investigation) only receive these tools when the planner explicitly selects the `jira` ToolSet for a task — see [Agent Tools](agent_tools.md) for the ToolSet-selection mechanism.

## See Also

- [Agent Tools](agent_tools.md) — the full agent-tool catalogue and the per-context availability matrix (which tools Jobs get vs. the interactive agent).
- [Configuration](configuration.md) — CLI flags and environment variables, including `--notion-api-token`, the `--github-app-*` flags, and the `--jira-*` flags.
- [Slack](slack.md) — Slack app setup and authentication, which power the mention agent and assist flow that consume these integration tools.
