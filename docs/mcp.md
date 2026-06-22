# MCP Server

Hecatoncheires can expose a read-only [Model Context Protocol](https://modelcontextprotocol.io/)
(MCP) endpoint so that AI clients (Claude and other MCP-capable hosts) can read
Workspaces, Cases, and Actions. The endpoint is served over **Streamable HTTP**
on the same HTTP server as the GraphQL API and Slack webhooks, and every tool
call is authenticated and authorized by a **Rego policy**.

## Enabling

The MCP endpoint is disabled by default. Enable it with `--mcp`, and supply at
least one Rego policy with `--policy`:

```bash
hecatoncheires serve \
  --mcp \
  --policy ./policies/mcp.rego \
  --mcp-env MCP_TOKEN
```

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--mcp` | `HECATONCHEIRES_MCP` | `false` | Enable the `/mcp` endpoint |
| `--policy` | `HECATONCHEIRES_POLICY` | - | Rego policy file(s) or directory(ies). Repeatable. **Required** when `--mcp` is set |
| `--mcp-env` | `HECATONCHEIRES_MCP_ENV` | - | Names of environment variables exposed to the policy as `input.env` (allow-list). Repeatable |

> **The endpoint never starts without a policy.** If `--mcp` is set but no
> `--policy` is supplied, `serve` exits with an error instead of exposing an
> unauthenticated data surface.

The endpoint is mounted at `POST/GET /mcp`. Point your MCP client's
Streamable HTTP transport at `https://your-host/mcp`.

## Tools

All tools are read-only and prefixed with `hecaton_` so they stay namespaced in
a client that aggregates several MCP servers.

| Tool | Input | Returns |
|------|-------|---------|
| `hecaton_list_workspaces` | _(none)_ | All workspaces with details: `id`, `name`, `description`, `emoji`, `color`, `case_mode`, `action_statuses`, `case_statuses`, `field_schema` |
| `hecaton_list_cases` | `workspace_id` (required), `status` (optional: `DRAFT`/`OPEN`/`CLOSED`) | Case summaries (`id`, `title`, `status`, `board_status`, `reporter_id`, `assignee_ids`, `created_at`, `updated_at`) |
| `hecaton_get_cases` | `workspace_id` (required), `ids` (required, `[]int`) | Full case details (summary fields plus `description`, `slack_channel_id`, `slack_thread_ts`, `field_values`, `agent_source_ids`) |
| `hecaton_list_actions` | `workspace_id` (required), `case_id` (optional), `include_archived` (optional `bool`) | Action details (`id`, `case_id`, `title`, `description`, `assignee_id`, `status`, `due_date`, `archived_at`, `slack_message_ts`, timestamps) |
| `hecaton_get_actions` | `workspace_id` (required), `ids` (required, `[]int`) | Action details |

### Private cases are never exposed

Private Cases — and **every Action beneath them** — are never returned over
MCP, regardless of channel membership. This is stricter than the Web/Slack
behaviour (where a member can view a private Case). Specifically:

- `hecaton_list_cases` omits private Cases entirely.
- `hecaton_get_cases` silently omits any requested private Case (and any
  non-existent ID), so the response never reveals that such a Case exists.
- `hecaton_list_actions` omits Actions whose parent Case is private; passing a
  private `case_id` returns an empty list.
- `hecaton_get_actions` silently omits Actions whose parent Case is private.

## Authorization (Rego)

Every tool call is evaluated against the Rego entrypoint **`data.auth.mcp`**.
The policy receives an `input` document and must return an object with an
`allow` boolean and an optional `user` string.

### Policy input

```json
{
  "req": {
    "method": "POST",
    "path": "/mcp",
    "header": { "Authorization": ["Bearer ..."] }
  },
  "env": { "MCP_TOKEN": "..." },
  "tool": {
    "name": "hecaton_list_cases",
    "workspace_id": "security",
    "args": { "workspace_id": "security", "status": "OPEN" }
  }
}
```

- `req` — the inbound HTTP request (method, path, headers). The request body is
  not included; the tool call is surfaced separately via `tool`.
- `env` — only the environment variables named with `--mcp-env`.
- `tool` — the tool name, the target `workspace_id` (when the tool takes one),
  and the full argument map, so a policy can authorize per tool / workspace /
  argument.

### Policy output

```json
{ "allow": true, "user": "U0123456789" }
```

- `allow` (boolean, required) — gates the call. When false, the tool returns an
  authorization error and no data is read.
- `user` (string, optional) — the Slack user ID the request acts as. It is
  injected downstream so private-case access control can resolve the caller's
  identity. (Note: because private Cases are never exposed via MCP regardless
  of membership, `user` does not grant access to private data; it is used for
  auditing and any future write tools.)

### Example policy

```rego
package auth.mcp

default allow := false

# Allow when the Authorization header carries the shared MCP token supplied
# via --mcp-env MCP_TOKEN.
allow if {
	input.req.header.Authorization[0] == sprintf("Bearer %s", [input.env.MCP_TOKEN])
}

# Act as a fixed Slack user.
user := "U0123456789" if {
	input.req.header.Authorization[0] == sprintf("Bearer %s", [input.env.MCP_TOKEN])
}

# You can also authorize per tool or workspace, e.g. allow only listing:
# allow if { input.tool.name == "hecaton_list_workspaces" }
```

Policies are compiled once at startup; a malformed policy makes `serve` fail
immediately rather than at the first request.

## Security notes

- **Secrets in logs are redacted.** The `input.env` allow-list is tagged for
  redaction and the `Authorization` header is redacted by the logger's masq
  configuration, so neither appears in logs even at debug level.
- **Stateless transport.** The Streamable HTTP transport runs stateless: each
  request carries its own authorization and no cross-request session state is
  held in process memory, so the endpoint is safe behind a horizontally-scaled
  deployment.
- **Deploy behind your own network controls** as appropriate; the Rego policy
  is the authorization gate, not a network boundary.
