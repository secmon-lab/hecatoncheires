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

// The values the "status" field of a read tool's result takes.
//
// It exists so that "nothing matched" is distinguishable from "the request was
// not carried out", without reading the prose in "message". A caller whose next
// step is "if there is no evidence, hand this to a person" gets that decision
// wrong in both directions otherwise: it stops on a repairable mistake, or it
// concludes there is no evidence when the search never ran.
//
// A failure to reach Notion at all — no permission, not shared, rate limited —
// is NOT one of these. It is returned as an error, so the agent sees a failed
// tool call rather than a result it could mistake for an empty one.
const (
	statusOK                  = "ok"
	statusInvalidRequest      = "invalid_request"
	statusDataSourceAmbiguous = "data_source_ambiguous"
)

// schemaSummary lists every property's name and type, plus the choices of the
// properties the caller asked about.
//
// Choices are not listed for everything on purpose: a data source in production
// carries dozens of properties, and their choice lists together do not fit in a
// model's context. Names and types do fit, and they are what a filter needs
// first.
func schemaSummary(ds *DataSource, withChoices []PropertySchema) []map[string]any {
	chosen := make(map[string]struct{}, len(withChoices))
	for _, p := range withChoices {
		chosen[p.ID] = struct{}{}
	}

	out := make([]map[string]any, 0, len(ds.Properties))
	for _, p := range ds.Properties {
		entry := map[string]any{"name": p.Name, "type": p.Type}
		if _, ok := chosen[p.ID]; ok && len(p.Options) > 0 {
			entry["options"] = p.Options
			entry["options_truncated"] = p.OptionsTruncated
		}
		out = append(out, entry)
	}
	return out
}

// rejectedResult answers an unusable request as a successful tool result. See
// the rejection type for why this is not an error.
//
// The schema rides along whenever it is already in hand, so the agent can
// repair the call from this one response instead of asking for the schema
// again.
func rejectedResult(out map[string]any, ds *DataSource, err error) (map[string]any, error) {
	message, ok := rejectionMessage(err)
	if !ok {
		return nil, err
	}

	out["status"] = statusInvalidRequest
	out["message"] = message
	out["items"] = []map[string]any{}
	out["matched"] = 0
	if ds != nil {
		out["property_schema"] = schemaSummary(ds, nil)
		out["operators_by_type"] = operatorsByType(ds)
	}
	return out, nil
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
		Description: "Describe a Notion database and list its rows — use this for a notion__search result whose type is \"database\". " +
			"Returns the database title, its rows as id/title/url entries, and 'property_schema': every column's name and type. " +
			"Read a row's own content with notion__get_page. " +
			"A database keeps its rows in one or more data sources: when it has several, no rows are returned and the 'data_sources' " +
			"list is reported instead, so call again with data_source_id set to the one you want. " +
			"'status' says what happened: \"ok\" (the row count is in 'matched'), \"invalid_request\" (fix the arguments and call again), " +
			"or \"data_source_ambiguous\". A row count of 0 with status \"ok\" means the database is empty, not that the call failed.",
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
			"describe_properties": {
				Type: gollem.TypeArray,
				Description: "Property names whose choices you need. Only select, status and multi_select properties have choices. " +
					"Every property's name and type is reported anyway; ask here for the choices of the few you intend to filter on.",
				Required: false,
				Items:    &gollem.Parameter{Type: gollem.TypeString},
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
	dataSourceID, reason, outcome := pickDataSource(db.DataSources, args)
	if dataSourceID == "" {
		out["data_source_id"] = ""
		out["items"] = []map[string]any{}
		out["message"] = reason
		out["status"] = statusForOutcome(outcome)
		out["matched"] = 0
		return out, nil
	}
	out["data_source_id"] = dataSourceID

	describe, err := parseNameList(args, "describe_properties", describePropertiesMax)
	if err != nil {
		return rejectedResult(out, nil, err)
	}

	opts := QueryOptions{}
	if v, err := tool.ExtractInt64(args, "page_size"); err == nil && v > 0 {
		opts.PageSize = int(v)
	}
	if s, ok := args["start_cursor"].(string); ok {
		opts.StartCursor = s
	}

	// The schema is read on the first page of a listing, and whenever choices
	// were asked for. A later page repeats neither: the names and types have not
	// changed between two pages of one listing, and this can be dozens of
	// entries.
	if opts.StartCursor == "" || len(describe) > 0 {
		ds, err := t.client.GetDataSource(ctx, dataSourceID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to fetch notion data source schema",
				goerr.V("database_id", databaseID),
				goerr.V("data_source_id", dataSourceID),
			)
		}
		chosen, err := resolveProperties(ds, describe)
		if err != nil {
			return rejectedResult(out, ds, err)
		}
		out["property_schema"] = schemaSummary(ds, chosen)
		out["operators_by_type"] = operatorsByType(ds)
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

	out["status"] = statusOK
	out["matched"] = len(items)
	out["items"] = items
	out["has_more"] = res.HasMore
	out["next_cursor"] = res.NextCursor
	return out, nil
}

// dataSourceOutcome says why a data source could not be named. The three
// situations call for three different things from the caller, so they cannot
// share one status: an id the database does not hold is an argument to repair, a
// choice not made is a choice to make, and a database with no data sources has
// no rows to find and nothing to fix.
type dataSourceOutcome int

const (
	dataSourceChosen dataSourceOutcome = iota
	dataSourceUnknown
	dataSourceEmpty
	dataSourceAmbiguous
)

// pickDataSource decides which data source of a database to read. It returns an
// empty id, the reason to report and which situation it was whenever the choice
// cannot be made.
func pickDataSource(sources []DataSourceRef, args map[string]any) (string, string, dataSourceOutcome) {
	if requested, _ := args["data_source_id"].(string); requested != "" {
		for _, ds := range sources {
			if ds.ID == requested {
				return requested, "", dataSourceChosen
			}
		}
		return "", "data_source_id is not one of this database's data sources; pick an id listed under data_sources", dataSourceUnknown
	}

	switch len(sources) {
	case 0:
		return "", "this database holds no data sources, so it has no rows to list", dataSourceEmpty
	case 1:
		return sources[0].ID, "", dataSourceChosen
	default:
		return "", "this database holds several data sources; call again with data_source_id set to one of the ids listed under data_sources", dataSourceAmbiguous
	}
}

// statusForOutcome maps a failed data source choice onto what the caller should
// do about it.
func statusForOutcome(outcome dataSourceOutcome) string {
	switch outcome {
	case dataSourceUnknown:
		// The caller named a data source this database does not hold, which is
		// its own argument to correct — not one of the listed ids to choose
		// between.
		return statusInvalidRequest
	case dataSourceEmpty:
		// A database with no data sources has no rows. That is an answer of
		// zero, not a request to fix anything: reported as ambiguous, a caller
		// would keep asking for a data source id that does not exist.
		return statusOK
	}
	return statusDataSourceAmbiguous
}
