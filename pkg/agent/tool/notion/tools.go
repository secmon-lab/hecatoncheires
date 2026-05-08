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

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// Deps groups the dependencies needed to register Notion-backed agent tools.
type Deps struct {
	// Client is the Notion API client. nil disables both Notion tools.
	Client Client
}

// New returns the Notion tools (search + get_page) when a client is provided.
// Returns nil when deps.Client is nil — the caller can simply append the result
// to the agent's tool list.
func New(deps Deps) []gollem.Tool {
	if deps.Client == nil {
		return nil
	}
	return []gollem.Tool{
		&searchTool{client: deps.Client},
		&getPageTool{client: deps.Client},
	}
}

// searchTool searches Notion pages and databases by title.
type searchTool struct {
	client Client
}

func (t *searchTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "notion__search",
		Description: "Search Notion pages and databases shared with the integration. Matches titles against the query string. Returns id, type (page or database), title, URL, and last edited timestamp.",
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
		})
	}

	return map[string]any{
		"items":       items,
		"has_more":    res.HasMore,
		"next_cursor": res.NextCursor,
	}, nil
}

// getPageTool retrieves a Notion page rendered as Notion-flavored Markdown.
type getPageTool struct {
	client Client
}

func (t *getPageTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "notion__get_page",
		Description: "Retrieve a Notion page's full content as Notion-flavored Markdown. The integration must have access to the page. Returns the markdown body and a 'truncated' flag (true when the page exceeds Notion's render limits).",
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
