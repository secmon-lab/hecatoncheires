// Package notiontool contains gollem tools that let the AI agent search Notion
// pages/databases and retrieve their content as Markdown via the Notion
// Markdown Content API (Notion-Version 2026-03-11). The Notion API client and
// types live here too, since they are agent-tool-specific and not used by the
// existing Source/Compile pipelines (which keep using pkg/service/notion).
package notiontool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Deps groups the dependencies needed to register Notion-backed agent tools.
type Deps struct {
	// Client is the Notion API client. nil disables both Notion tools.
	Client Client
}

// New returns the Notion tools when a client is provided. Returns nil when
// deps.Client is nil — the caller can simply append the result to the agent's
// tool list.
func New(deps Deps) []gollem.Tool {
	if deps.Client == nil {
		return nil
	}
	return []gollem.Tool{
		&searchTool{client: deps.Client},
		&getPageTool{client: deps.Client},
		&getDatabaseTool{client: deps.Client},
		&searchDatabaseTool{client: deps.Client},
	}
}

// searchDatabaseToolName is referenced from the other tools' results, which is
// how an agent is told where to search rather than only being told in prose.
const searchDatabaseToolName = "notion__search_database"

// searchTool searches Notion pages and databases by title.
type searchTool struct {
	client Client
}

func (t *searchTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "notion__search",
		Description: "Search Notion pages and databases shared with the integration. Matches titles against the query string — " +
			"Notion cannot search page bodies, and this endpoint takes ONE substring, not several keywords. " +
			"Returns id, type (page or database), title, URL, last edited timestamp, read_tool, and the hit's parent " +
			"(parent_type, parent_id, and parent_database_name when the parent is a database), so it can be told whether a hit " +
			"belongs to the database you care about. " +
			"read_tool names the tool that reads that hit — notion__get_page for a page, notion__get_database for a database. " +
			"Call the tool the hit names; the two are not interchangeable and passing a database id to notion__get_page fails. " +
			"A database hit also carries search_tool: use " + searchDatabaseToolName + " to search that database's rows by " +
			"several keywords or by property value, which this tool cannot do.",
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
			"sort_by_last_edited": {
				Type: gollem.TypeString,
				Description: "Order hits by when they were last edited. Omit for Notion's relevance order. " +
					"Notion cannot sort this search by anything else.",
				Required: false,
				Enum:     []string{directionAscending, directionDescending},
			},
			"start_cursor": {
				Type:        gollem.TypeString,
				Description: "Pagination cursor returned as 'next_cursor' by a previous call.",
				Required:    false,
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
	if s, ok := args["sort_by_last_edited"].(string); ok {
		opts.SortByEdit = s
	}
	if s, ok := args["start_cursor"].(string); ok {
		opts.StartCursor = s
	}

	tool.Update(ctx, fmt.Sprintf("Searching Notion: %q", query))

	res, err := t.client.Search(ctx, query, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search notion",
			goerr.V("query", query),
		)
	}

	names := t.resolveParentNames(ctx, res.Items)

	items := make([]map[string]any, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, map[string]any{
			"id":                   it.ID,
			"type":                 it.Type,
			"title":                it.Title,
			"url":                  it.URL,
			"last_edited":          it.LastEdited.Format(time.RFC3339),
			"read_tool":            readToolFor(it.Type),
			"search_tool":          searchToolFor(it.Type),
			"parent_type":          it.Parent.Type,
			"parent_id":            it.Parent.ID,
			"parent_database_name": names[it.Parent.ID],
		})
	}

	return map[string]any{
		"status":      statusOK,
		"matched":     len(items),
		"items":       items,
		"has_more":    res.HasMore,
		"next_cursor": res.NextCursor,
	}, nil
}

// parentNameResolveMax bounds how many distinct parent databases one search
// resolves the name of.
//
// Notion does not include a parent's title in a search result, so each name is
// one more request, and Notion rate-limits at roughly three per second. The id
// is what decides whether a hit belongs to the database the caller cares about;
// the name is for quoting it. So the ids are always complete and the names run
// out first.
const parentNameResolveMax = 5

// resolveParentNames looks up the titles of the distinct parent databases among
// the hits, keyed by database id.
//
// A lookup that fails leaves the name absent and the search successful: the hit
// is still usable through its id, and failing the whole search because one
// parent could not be read would be a worse answer than a missing label.
func (t *searchTool) resolveParentNames(ctx context.Context, items []SearchItem) map[string]string {
	names := make(map[string]string)
	for _, it := range items {
		if it.Parent.Type != parentTypeDatabase || it.Parent.ID == "" {
			continue
		}
		if _, seen := names[it.Parent.ID]; seen {
			continue
		}
		if len(names) == parentNameResolveMax {
			break
		}
		// Recorded before the call so a failed lookup is not retried for the
		// next hit that shares the same parent.
		names[it.Parent.ID] = ""

		db, err := t.client.GetDatabase(ctx, it.Parent.ID)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to resolve notion parent database name",
				goerr.V("parent_database_id", it.Parent.ID),
			), "failed to resolve notion parent database name")
			continue
		}
		names[it.Parent.ID] = db.Title
	}
	return names
}

// searchToolFor names the tool that searches inside a search hit. Only a
// database has rows to search; a page is read whole.
func searchToolFor(itemType string) string {
	if itemType == "database" {
		return searchDatabaseToolName
	}
	return ""
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
			"It lists the rows unfiltered and in Notion's own order: to search the rows by keyword or property value, " +
			"call " + searchDatabaseToolName + " (named in this result as 'search_tool') with the property names reported here. " +
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
		"database_id": db.ID,
		"title":       db.Title,
		"url":         db.URL,
		// Which tool searches these rows is stated as data, not only in this
		// tool's prose: the same routing carried as data on each search hit is
		// what stopped a database id from being sent to the page tool.
		"search_tool":  searchDatabaseToolName,
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
		out["status"] = statusDataSourceAmbiguous
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

// searchDatabaseTool searches the rows of one database by keyword and by
// property value.
//
// It is a separate tool from getDatabaseTool rather than more arguments on it
// because the name is what an agent picks from: "get_database" does not read as
// somewhere to search, and choosing the wrong Notion tool is a mistake that has
// already happened in production (ARGUS-91, a database id sent to
// notion__get_page once per hit).
type searchDatabaseTool struct {
	client Client
}

func (t *searchDatabaseTool) Spec() gollem.ToolSpec {
	condition := func() *gollem.Parameter {
		return &gollem.Parameter{
			Type: gollem.TypeObject,
			Properties: map[string]*gollem.Parameter{
				"property": {
					Type:        gollem.TypeString,
					Description: "Property name, exactly as listed in notion__get_database's property_schema.",
					Required:    true,
				},
				"operator": {
					Type: gollem.TypeString,
					Description: "How to compare. Which operators a property accepts depends on its type — " +
						"notion__get_database reports that under operators_by_type.",
					Required: true,
					Enum: []string{
						"equals", "does_not_equal", "contains", "does_not_contain",
						"starts_with", "ends_with", "is_empty", "is_not_empty",
						"greater_than", "greater_than_or_equal_to", "less_than", "less_than_or_equal_to",
						"before", "after", "on_or_before", "on_or_after",
						"past_week", "past_month", "past_year", "this_week",
						"next_week", "next_month", "next_year",
					},
				},
				"value": {
					Type: gollem.TypeString,
					Description: "What to compare against, written as text: a number as \"42\", a checkbox as " +
						"\"true\", a date as \"2026-01-31\" or \"2026-01-31T09:00:00Z\", a person or a related page " +
						"as its id. Leave it out for is_empty, is_not_empty and the past_/this_/next_ operators.",
				},
				"value_type": {
					Type: gollem.TypeString,
					Description: "Required only for a formula or rollup property: what it evaluates to. Notion " +
						"does not report that in the schema, so it cannot be inferred.",
					Enum: valueTypeNames,
				},
				"aggregation": {
					Type: gollem.TypeString,
					Description: "For a rollup over a list only: how many of the rolled-up values must match. " +
						"A rollup that computes a number or a date needs no aggregation.",
					Enum: rollupAggregations,
				},
			},
		}
	}

	return gollem.ToolSpec{
		Name: searchDatabaseToolName,
		Description: "Search the rows of one Notion database. Give it a database id and keywords: only rows " +
			"containing EVERY keyword are returned. It can also filter on property values, order the rows, and " +
			"return the values of the properties you name. " +
			"Call notion__get_database first for the property names and types. " +
			"Notion cannot search page bodies, so a term that appears only in a page's body is not findable — " +
			"name the properties that carry such terms (a summary, a keyword list) in search_properties. " +
			"Use this instead of notion__get_database whenever you need to narrow the rows down. " +
			"'status' says what happened: \"ok\" (the row count is in 'matched'), \"invalid_request\" (the message " +
			"says what to fix; the property schema is attached), or \"data_source_ambiguous\". A row count of 0 " +
			"with status \"ok\" means nothing matched; a failed call means the search did not run at all.",
		Parameters: map[string]*gollem.Parameter{
			"database_id": {
				Type:        gollem.TypeString,
				Description: "The Notion database ID (with or without dashes).",
				Required:    true,
			},
			"data_source_id": {
				Type:        gollem.TypeString,
				Description: "Which data source of the database to search. Omit unless a previous call reported several.",
				Required:    false,
			},
			"query": {
				Type: gollem.TypeString,
				Description: "Keywords separated by spaces. A row must match EVERY keyword to be returned. Each " +
					"keyword is matched against the properties named in search_properties.",
				Required: false,
			},
			"search_properties": {
				Type: gollem.TypeArray,
				Description: "Which properties each keyword is matched against. Defaults to the row title. " +
					"A text property matches a substring; a select, status or multi_select property matches a " +
					"choice name exactly, so pass the choice as it is spelled in the schema.",
				Required: false,
				Items:    &gollem.Parameter{Type: gollem.TypeString},
			},
			"filter": {
				Type: gollem.TypeObject,
				Description: "Narrow the rows by property values, in addition to the keywords. " +
					"Property names and types come from notion__get_database.",
				Required: false,
				Properties: map[string]*gollem.Parameter{
					"operator": {
						Type:        gollem.TypeString,
						Description: "How conditions and groups combine. Defaults to and.",
						Enum:        []string{operatorAnd, operatorOr},
					},
					"conditions": {
						Type:        gollem.TypeArray,
						Description: "Conditions combined by the operator above.",
						Items:       condition(),
					},
					"groups": {
						Type: gollem.TypeArray,
						Description: "Nested groups of conditions. Notion nests only two levels deep, so a group " +
							"holds plain conditions — and a group cannot be combined with 'query' when operator is or.",
						Items: &gollem.Parameter{
							Type: gollem.TypeObject,
							Properties: map[string]*gollem.Parameter{
								"operator": {
									Type:     gollem.TypeString,
									Required: true,
									Enum:     []string{operatorAnd, operatorOr},
								},
								"conditions": {
									Type:     gollem.TypeArray,
									Required: true,
									Items:    condition(),
								},
							},
						},
					},
				},
			},
			"sorts": {
				Type:        gollem.TypeArray,
				Description: "Order the rows. Earlier entries take precedence.",
				Required:    false,
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"property": {
							Type:        gollem.TypeString,
							Description: "Property name to order by. Set either this or timestamp, not both.",
						},
						"timestamp": {
							Type:        gollem.TypeString,
							Description: "Order by when the row was created or last edited.",
							Enum:        []string{propTypeCreatedTime, propTypeLastEditedTime},
						},
						"direction": {
							Type:        gollem.TypeString,
							Description: "Defaults to ascending.",
							Enum:        []string{directionAscending, directionDescending},
						},
					},
				},
			},
			"properties": {
				Type: gollem.TypeArray,
				Description: "Property names whose values to include on each returned row, so the rows can be " +
					"judged without opening each one. Omit to return only id, title, url and last_edited.",
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

func (t *searchDatabaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	databaseID, _ := args["database_id"].(string)
	if databaseID == "" {
		return nil, goerr.New("database_id is required")
	}

	db, err := t.client.GetDatabase(ctx, databaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch notion database", goerr.V("database_id", databaseID))
	}

	out := map[string]any{
		"database_id":    db.ID,
		"database_title": db.Title,
	}

	dataSourceID, reason := pickDataSource(db.DataSources, args)
	if dataSourceID == "" {
		sources := make([]map[string]any, 0, len(db.DataSources))
		for _, ds := range db.DataSources {
			sources = append(sources, map[string]any{"id": ds.ID, "name": ds.Name})
		}
		out["data_source_id"] = ""
		out["data_sources"] = sources
		out["items"] = []map[string]any{}
		out["message"] = reason
		out["status"] = statusDataSourceAmbiguous
		out["matched"] = 0
		return out, nil
	}
	out["data_source_id"] = dataSourceID

	keywords, err := parseSearchQuery(args)
	if err != nil {
		return rejectedResult(out, nil, err)
	}
	filterSpec, err := parseFilterArgs(args)
	if err != nil {
		return rejectedResult(out, nil, err)
	}
	sortSpecs, err := parseSortArgs(args)
	if err != nil {
		return rejectedResult(out, nil, err)
	}
	searchNames, err := parseNameList(args, "search_properties", searchPropertiesMax)
	if err != nil {
		return rejectedResult(out, nil, err)
	}
	returnNames, err := parseNameList(args, "properties", returnPropertiesMax)
	if err != nil {
		return rejectedResult(out, nil, err)
	}

	tool.Update(ctx, searchProgress(db.Title, keywords))

	// The schema is read on every search: the conditions are written against
	// property names and types, and those are not in the database object.
	ds, err := t.client.GetDataSource(ctx, dataSourceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch notion data source schema",
			goerr.V("database_id", databaseID),
			goerr.V("data_source_id", dataSourceID),
		)
	}

	var targets []PropertySchema
	if len(keywords) > 0 || len(searchNames) > 0 {
		targets, err = resolveSearchTargets(ds, searchNames)
		if err != nil {
			return rejectedResult(out, ds, err)
		}
	}

	filter, err := buildFilter(ds, keywords, targets, filterSpec)
	if err != nil {
		return rejectedResult(out, ds, err)
	}
	sorts, err := buildSorts(ds, sortSpecs)
	if err != nil {
		return rejectedResult(out, ds, err)
	}
	returnProps, err := resolveProperties(ds, returnNames)
	if err != nil {
		return rejectedResult(out, ds, err)
	}

	opts := QueryOptions{Filter: filter, Sorts: sorts, Properties: returnProps}
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
			goerr.V("condition_count", conditionCount(keywords, filterSpec)),
		)
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, it := range res.Items {
		item := map[string]any{
			"id":          it.ID,
			"type":        it.Type,
			"title":       it.Title,
			"url":         it.URL,
			"last_edited": it.LastEdited.Format(time.RFC3339),
			"read_tool":   readToolFor(it.Type),
		}
		if len(it.Properties) > 0 {
			item["properties"] = it.Properties
		}
		items = append(items, item)
	}

	out["status"] = statusOK
	out["matched"] = len(items)
	out["items"] = items
	out["has_more"] = res.HasMore
	out["next_cursor"] = res.NextCursor
	return out, nil
}

// searchProgress is the one line the run's progress message shows for this
// call. It names the keywords rather than the ids, which is what a person
// reading the thread can recognise.
func searchProgress(databaseTitle string, keywords []string) string {
	if len(keywords) == 0 {
		return fmt.Sprintf("Searching the Notion database %q...", databaseTitle)
	}
	return fmt.Sprintf("Searching the Notion database %q for %q...", databaseTitle, strings.Join(keywords, " "))
}

// conditionCount is attached to a failed query so an operator can see how big
// the request was. The conditions themselves are not: a failed tool call's goerr
// values are rendered into the response the model reads and reproduced in the
// Slack thread, so what travels here is a size, never row content.
func conditionCount(keywords []string, spec *filterSpec) int {
	count := len(keywords)
	if spec == nil {
		return count
	}
	count += len(spec.Conditions)
	for _, group := range spec.Groups {
		count += len(group.Conditions)
	}
	return count
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
