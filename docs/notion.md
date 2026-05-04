# Notion Integration

Hecatoncheires integrates with Notion to:

- Compile knowledge entries from Notion pages and databases (`compile` CLI subcommand and `Source` ingestion pipeline) — backed by `pkg/service/notion`.
- Surface Notion content to the AI agent through tools registered in `pkg/agent/tool/notion/`:
  - `notion__search` — search pages and databases shared with the integration.
  - `notion__get_page` — retrieve a page's content as Notion-flavored Markdown.

This document covers the setup needed for both use cases.

## 1. Create a Notion Internal Integration

1. Open <https://www.notion.so/profile/integrations> (or **Settings → Integrations → Develop your own integrations**).
2. Click **New integration**.
3. Fill in:
   - **Name**: e.g. `Hecatoncheires`.
   - **Associated workspace**: pick the workspace that owns the pages/databases you want to expose.
   - **Type**: **Internal**.
4. Under **Capabilities**, enable:
   - **Read content** — required for both `Search` and the Markdown content endpoint (and for the existing knowledge ingestion pipeline).
   - The other capabilities (Update content / Insert content / etc.) are **not** required for the agent tools.
5. Click **Save**.
6. Copy the **Internal Integration Token** (starts with `secret_…`). This is the value passed via `--notion-api-token` / `HECATONCHEIRES_NOTION_API_TOKEN`.

> Notion's official Markdown Content API works with both *public* and *internal* integrations as long as the integration has the **Read content** capability and the page is shared with it. Internal integrations are the recommended choice for self-hosted deployments because they do not require publishing the integration.

## 2. Share Pages / Databases with the Integration

Notion's permission model is opt-in: a page or database is invisible to the integration until it is explicitly shared.

For each top-level page or database you want the agent (or compile pipeline) to see:

1. Open it in Notion.
2. Click **Share → Add connections** and select the integration you created above.
3. Notion grants the connection access to the page **and all of its descendants**, so it is usually enough to share a small number of root pages.

Pages or child blocks that are **not** shared with the connection will appear as `<unknown>` placeholders in the Markdown output (a documented Notion API limitation).

## 3. Configure the Server

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

in the server logs at startup. The four agent tool registrations (`core__notion_search`, `core__notion_get_page`) light up automatically when the agent runs.

If the token is omitted, the Notion-backed agent tools are silently skipped and the server logs:

```
Notion API token not configured, Source features will be limited
```

## 4. API Surface Used by the Agent Tools

| Tool | Endpoint | Notes |
|------|----------|-------|
| `notion__search` | `POST /v1/search` | Title-substring match across all pages and databases shared with the integration. Pagination via `start_cursor` / `next_cursor`. Capped at 100 results per call. |
| `notion__get_page` | `GET /v1/pages/{page_id}/markdown` | Returns Notion-flavored ("enhanced") Markdown rendered server-side by Notion. Requires `Notion-Version: 2026-03-11` (sent automatically by `pkg/agent/tool/notion/client.go`). |

### About the Markdown Content API

The `GET /v1/pages/{page_id}/markdown` endpoint was introduced by Notion in early 2026 and is the supported way to retrieve a page's full content as a Markdown document in a single call. Hecatoncheires uses it because:

- It is dramatically more compact than the raw block-tree API for LLM consumption.
- File-based blocks (image / file / video / audio / PDF) are returned as pre-signed URLs; those URLs expire after a short period, so consumers must download attachments promptly if they intend to persist them.
- Pages exceeding ~20,000 blocks are returned with `truncated: true`. Hecatoncheires propagates this flag through `core__notion_get_page` so the agent can warn the user.

> The third-party `jomei/notionapi` Go client used elsewhere in the codebase does not expose this endpoint, so `pkg/agent/tool/notion/client.go` calls it directly with `net/http` while reusing the same API token. The Markdown endpoint and the Search endpoint live in the agent tool package because they are exclusively used by the agent — `pkg/service/notion` keeps only the Source/Compile-facing surface.

## 5. Operational Notes

- **Rate limits**: Notion enforces ~3 req/s averaged. The Notion service uses the `notionapi` library's built-in retry-on-429 (3 retries) for `Search` and `QueryUpdatedPages`. The Markdown endpoint does not use the library, but Notion returns standard 429 responses, which surface to the agent as an error string (the agent typically retries with a delay).
- **Token rotation**: rotating the Internal Integration Token requires re-deploying the server with the new token. There is no graceful refresh — the previous token is invalidated immediately.
- **Multi-instance safety**: the Notion client is stateless and safe to instantiate per process; no shared in-memory state is held across instances.

## 6. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Agent never offers `core__notion_search` | `HECATONCHEIRES_NOTION_API_TOKEN` not set, or the value is empty | Set the token; restart the server. |
| `core__notion_get_page` returns `non-2xx` with status 404 | Page is not shared with the integration | Open the page in Notion → **Share → Add connections** → select your integration. |
| `core__notion_search` returns no results despite knowing pages exist | Pages have not been shared with the integration | Same as above — sharing is per-tree, not workspace-wide. |
| Markdown response has `truncated: true` and ends with `<unknown>` blocks | Page exceeds Notion's render limits, or blocks reference unshared child pages | Split the page, or share the referenced child pages with the integration. |
| Agent gets a "validation_error: invalid Notion-Version" | Running against a Notion enterprise tenant that pins a lower API version | This is unlikely under default Notion plans; contact Notion support if the response references a different `notion-version` constraint. |
