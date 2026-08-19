// Package notiontool contains gollem tools that let the AI agent search Notion
// pages/databases and retrieve their content as Markdown via the Notion
// Markdown Content API (Notion-Version 2026-03-11). The Notion API client and
// types live here too, since they are agent-tool-specific and not used by the
// existing Source/Compile pipelines (which keep using pkg/service/notion).
package notiontool

import (
	"context"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// Deps groups the dependencies needed to register Notion-backed agent tools.
type Deps struct {
	// Client is the Notion API client. nil disables both Notion tools.
	Client Client
}

// New returns the Notion tools (search + get_page + get_database) when a client
// is provided. Returns nil when deps.Client is nil — the caller can simply
// append the result to the agent's tool list.
func New(deps Deps) []gollem.Tool {
	if deps.Client == nil {
		return nil
	}
	return []gollem.Tool{
		&searchTool{client: deps.Client},
		&getPageTool{client: deps.Client},
		&getDatabaseTool{client: deps.Client},
	}
}

// searchTool searches Notion pages and databases by title.
type searchTool struct {
	client Client
}

func (t *searchTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "notion__search",
		Description: "Search Notion pages and databases shared with the integration. Matches titles against the query string. " +
			"Returns id, type (page or database), title, URL, last edited timestamp, and read_tool. " +
			"read_tool names the tool that reads that hit — notion__get_page for a page, notion__get_database for a database. " +
			"Call the tool the hit names; the two are not interchangeable and passing a database id to notion__get_page fails.",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "Title substring to search for. Pass an empty string to list all accessible pages/databases.",
				Required:    true,
			},
			"page_size": {
				Type:        gollem.TypeInteger,
				Description: "Number of results per page (1-100, default 20).",
				Required:    false,
			},
			"filter_type": {
				Type:        gollem.TypeString,
				Description: "Limit results to a specific object type. Empty for both pages and databases.",
				Required:    false,
				Enum:        []string{"page", "database"},
			},
		},
	}
}

func (t *searchTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	// query is allowed to be empty per the Notion docs (returns all accessible pages),
	// but the agent must opt into that explicitly by passing the key.
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query is required (pass empty string to list all)")
	}

	opts := SearchOptions{}
	if v, err := tool.ExtractInt64(args, "page_size"); err == nil && v > 0 {
		opts.PageSize = int(v)
	}
	if s, ok := args["filter_type"].(string); ok {
		opts.FilterType = s
	}

	tool.Update(ctx, fmt.Sprintf("Searching Notion: %q", query))

	res, err := t.client.Search(ctx, query, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search notion",
			goerr.V("query", query),
		)
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, map[string]any{
			"id":          it.ID,
			"type":        it.Type,
			"title":       it.Title,
			"url":         it.URL,
			"last_edited": it.LastEdited.Format(time.RFC3339),
			"read_tool":   readToolFor(it.Type),
		})
	}

	return map[string]any{
		"items":       items,
		"has_more":    res.HasMore,
		"next_cursor": res.NextCursor,
	}, nil
}

// readToolFor names the tool that reads a search hit of the given type. It is
// carried on every item because the type alone did not stop the agent from
// sending a database id to notion__get_page (ARGUS-91): the routing is stated
// as data the model can follow, not only as prose in the tool descriptions.
// An unrecognised type gets no name rather than a guess.
func readToolFor(itemType string) string {
	switch itemType {
	case "page":
		return "notion__get_page"
	case "database":
		return "notion__get_database"
	default:
		return ""
	}
}

// getPageTool retrieves a Notion page rendered as Notion-flavored Markdown.
type getPageTool struct {
	client Client
}

func (t *getPageTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "notion__get_page",
		Description: "Retrieve a Notion page's full content as Notion-flavored Markdown. Accepts a page id only: a notion__search result whose type is \"database\" is not a page and must go to notion__get_database instead. The integration must have access to the page. Returns the markdown body and a 'truncated' flag (true when the page exceeds Notion's render limits).",
		Parameters: map[string]*gollem.Parameter{
			"page_id": {
				Type:        gollem.TypeString,
				Description: "The Notion page ID (with or without dashes).",
				Required:    true,
			},
		},
	}
}

func (t *getPageTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	pageID, _ := args["page_id"].(string)
	if pageID == "" {
		return nil, fmt.Errorf("page_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Fetching Notion page %s...", pageID))

	res, err := t.client.GetPageMarkdown(ctx, pageID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch notion page markdown",
			goerr.V("page_id", pageID),
		)
	}

	return map[string]any{
		"page_id":   res.PageID,
		"markdown":  res.Markdown,
		"truncated": res.Truncated,
	}, nil
}

// getDatabaseTool lists the pages held by a Notion database.
//
// It exists because notion__search reports databases as well as pages while
// notion__get_page reads pages only. With no tool for the database half, the
// agent fed each database id it had found to notion__get_page and Notion
// answered "is a database, not a page" once per hit (ARGUS-91).
type getDatabaseTool struct {
	client Client
}

func (t *getDatabaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "notion__get_database",
		Description: "List the pages held by a Notion database — use this for a notion__search result whose type is \"database\". " +
			"Returns the database title and its rows as id/title/url entries; read a row's own content with notion__get_page. " +
			"A database keeps its rows in one or more data sources: when it has several, no rows are returned and the 'data_sources' " +
			"list is reported instead, so call again with data_source_id set to the one you want.",
		Parameters: map[string]*gollem.Parameter{
			"database_id": {
				Type:        gollem.TypeString,
				Description: "The Notion database ID (with or without dashes).",
				Required:    true,
			},
			"data_source_id": {
				Type:        gollem.TypeString,
				Description: "Which data source of the database to list. Omit unless a previous call reported several.",
				Required:    false,
			},
			"page_size": {
				Type:        gollem.TypeInteger,
				Description: "Number of rows per page (1-100, default 20).",
				Required:    false,
			},
			"start_cursor": {
				Type:        gollem.TypeString,
				Description: "Pagination cursor returned as 'next_cursor' by a previous call.",
				Required:    false,
			},
		},
	}
}

func (t *getDatabaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	databaseID, _ := args["database_id"].(string)
	if databaseID == "" {
		return nil, goerr.New("database_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Reading Notion database %s...", databaseID))

	db, err := t.client.GetDatabase(ctx, databaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch notion database",
			goerr.V("database_id", databaseID),
		)
	}

	sources := make([]map[string]any, 0, len(db.DataSources))
	for _, ds := range db.DataSources {
		sources = append(sources, map[string]any{"id": ds.ID, "name": ds.Name})
	}

	out := map[string]any{
		"database_id":  db.ID,
		"title":        db.Title,
		"url":          db.URL,
		"data_sources": sources,
	}

	// An unresolved data source is reported as a result rather than an error: it
	// is something the model can act on by calling again, and a returned error
	// would also be filed as a tool failure by the strategies that report them.
	dataSourceID, reason := pickDataSource(db.DataSources, args)
	if dataSourceID == "" {
		out["data_source_id"] = ""
		out["items"] = []map[string]any{}
		out["message"] = reason
		return out, nil
	}

	opts := QueryOptions{}
	if v, err := tool.ExtractInt64(args, "page_size"); err == nil && v > 0 {
		opts.PageSize = int(v)
	}
	if s, ok := args["start_cursor"].(string); ok {
		opts.StartCursor = s
	}

	res, err := t.client.QueryDataSource(ctx, dataSourceID, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to query notion data source",
			goerr.V("database_id", databaseID),
			goerr.V("data_source_id", dataSourceID),
		)
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, map[string]any{
			"id":          it.ID,
			"type":        it.Type,
			"title":       it.Title,
			"url":         it.URL,
			"last_edited": it.LastEdited.Format(time.RFC3339),
		})
	}

	out["data_source_id"] = dataSourceID
	out["items"] = items
	out["has_more"] = res.HasMore
	out["next_cursor"] = res.NextCursor
	return out, nil
}

// pickDataSource decides which data source of a database to list. It returns an
// empty id plus the reason to report when the choice cannot be made: an id the
// database does not hold, no data sources at all, or several with none named.
func pickDataSource(sources []DataSourceRef, args map[string]any) (string, string) {
	if requested, _ := args["data_source_id"].(string); requested != "" {
		for _, ds := range sources {
			if ds.ID == requested {
				return requested, ""
			}
		}
		return "", "data_source_id is not one of this database's data sources; pick an id listed under data_sources"
	}

	switch len(sources) {
	case 0:
		return "", "this database holds no data sources, so it has no rows to list"
	case 1:
		return sources[0].ID, ""
	default:
		return "", "this database holds several data sources; call again with data_source_id set to one of the ids listed under data_sources"
	}
}
