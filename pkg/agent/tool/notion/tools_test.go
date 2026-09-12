package notiontool_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
)

// fakeNotionClient records what each tool asked for and answers with canned
// data. Every method is driven from a field so a test can shape one call
// without touching the others.
type fakeNotionClient struct {
	searchResult  *notiontool.SearchResult
	database      *notiontool.Database
	databaseErr   error
	dataSource    *notiontool.DataSource
	dataSourceErr error
	queryResult   *notiontool.QueryResult
	queryErr      error

	gotDatabaseIDs     []string
	gotSchemaDataSrcID []string
	gotDataSourceIDs   []string
	gotQueryOptions    []notiontool.QueryOptions
	gotSearchOptions   []notiontool.SearchOptions
}

func (f *fakeNotionClient) Search(_ context.Context, _ string, opts notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	f.gotSearchOptions = append(f.gotSearchOptions, opts)
	if f.searchResult != nil {
		return f.searchResult, nil
	}
	return &notiontool.SearchResult{}, nil
}

func (f *fakeNotionClient) GetPageMarkdown(_ context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	return &notiontool.PageMarkdown{PageID: pageID}, nil
}

func (f *fakeNotionClient) GetDatabase(_ context.Context, databaseID string) (*notiontool.Database, error) {
	f.gotDatabaseIDs = append(f.gotDatabaseIDs, databaseID)
	if f.databaseErr != nil {
		return nil, f.databaseErr
	}
	return f.database, nil
}

func (f *fakeNotionClient) GetDataSource(_ context.Context, dataSourceID string) (*notiontool.DataSource, error) {
	f.gotSchemaDataSrcID = append(f.gotSchemaDataSrcID, dataSourceID)
	if f.dataSourceErr != nil {
		return nil, f.dataSourceErr
	}
	if f.dataSource != nil {
		return f.dataSource, nil
	}
	return &notiontool.DataSource{ID: dataSourceID}, nil
}

func (f *fakeNotionClient) QueryDataSource(_ context.Context, dataSourceID string, opts notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	f.gotDataSourceIDs = append(f.gotDataSourceIDs, dataSourceID)
	f.gotQueryOptions = append(f.gotQueryOptions, opts)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.queryResult, nil
}

// blockingNotionClient answers a search immediately and then holds every
// database read until the test releases it or the caller's context is done. It
// stands in for a Notion that has stopped responding while a search has already
// succeeded.
type blockingNotionClient struct {
	searchResult *notiontool.SearchResult
	release      chan struct{}

	mu    sync.Mutex
	calls int
}

func (b *blockingNotionClient) Search(context.Context, string, notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	if b.searchResult != nil {
		return b.searchResult, nil
	}
	return &notiontool.SearchResult{}, nil
}

func (b *blockingNotionClient) GetPageMarkdown(_ context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	return &notiontool.PageMarkdown{PageID: pageID}, nil
}

func (b *blockingNotionClient) GetDatabase(ctx context.Context, databaseID string) (*notiontool.Database, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
		return &notiontool.Database{ID: databaseID, Title: "Some database"}, nil
	}
}

func (b *blockingNotionClient) GetDataSource(_ context.Context, dataSourceID string) (*notiontool.DataSource, error) {
	return &notiontool.DataSource{ID: dataSourceID}, nil
}

func (b *blockingNotionClient) QueryDataSource(context.Context, string, notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	return &notiontool.QueryResult{}, nil
}

func (b *blockingNotionClient) databaseCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func findTool(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	var found gollem.Tool
	for _, tl := range tools {
		if tl.Spec().Name == name {
			found = tl
		}
	}
	gt.Value(t, found).NotNil().Required()
	return found
}

func TestNew(t *testing.T) {
	t.Run("returns no tools when the client is nil", func(t *testing.T) {
		gt.Array(t, notiontool.New(notiontool.Deps{})).Length(0)
	})

	t.Run("registers search, get_page, get_database and search_database", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})
		gt.Array(t, tools).Length(4).Required()

		names := make([]string, 0, len(tools))
		for _, tl := range tools {
			names = append(names, tl.Spec().Name)
		}
		gt.Array(t, names).Equal([]string{
			"notion__search", "notion__get_page", "notion__get_database", "notion__search_database",
		})
	})

	// get_database lists the rows unfiltered, so an agent that needs to narrow
	// them has to be sent to the other tool — and told as data, not only in
	// prose, for the reason ARGUS-91 gives below.
	t.Run("get_database points at search_database", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})
		gt.String(t, findTool(t, tools, "notion__get_database").Spec().Description).
			Contains("notion__search_database")
		gt.String(t, findTool(t, tools, "notion__search_database").Spec().Description).
			Contains("notion__get_database")
	})

	// gollem rejects a tool whose object parameter declares no properties and
	// whose array parameter declares no items, and it does so when the agent
	// runs rather than at startup.
	t.Run("every tool definition passes gollem's own validation", func(t *testing.T) {
		for _, tl := range notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}}) {
			spec := tl.Spec()
			gt.NoError(t, spec.Validate())
		}
	})

	// A search hit typed "database" is not readable by notion__get_page, and the
	// model only learns that from these descriptions. Feeding one to the other is
	// what produced a 400 per database found (ARGUS-91).
	t.Run("search and get_page send a database hit to get_database", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})

		search := findTool(t, tools, "notion__search").Spec().Description
		gt.String(t, search).Contains("notion__get_database")
		gt.String(t, search).Contains("notion__get_page")
		gt.String(t, search).Contains("read_tool")

		getPage := findTool(t, tools, "notion__get_page").Spec().Description
		gt.String(t, getPage).Contains("notion__get_database")
	})
}

func TestSearchTool(t *testing.T) {
	// The type of a hit was already reported when the agent sent a database id to
	// notion__get_page anyway (ARGUS-91), so each hit also names the tool that
	// reads it.
	t.Run("names the reading tool on every hit", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{
				{
					ID:         "page-1",
					Type:       "page",
					Title:      "Incident Playbook",
					URL:        "https://www.notion.so/page-1",
					LastEdited: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				},
				{
					ID:         "db-1",
					Type:       "database",
					Title:      "Runbooks",
					URL:        "https://www.notion.so/db-1",
					LastEdited: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC),
				},
			},
			HasMore:    true,
			NextCursor: "cursor-2",
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "runbook"})
		gt.NoError(t, err).Required()

		gt.Value(t, got["has_more"]).Equal(true)
		gt.Value(t, got["next_cursor"]).Equal("cursor-2")

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(2).Required()

		gt.Value(t, items[0]["id"]).Equal("page-1")
		gt.Value(t, items[0]["type"]).Equal("page")
		gt.Value(t, items[0]["title"]).Equal("Incident Playbook")
		gt.Value(t, items[0]["url"]).Equal("https://www.notion.so/page-1")
		gt.Value(t, items[0]["last_edited"]).Equal("2026-04-01T12:00:00Z")
		gt.Value(t, items[0]["read_tool"]).Equal("notion__get_page")

		gt.Value(t, items[1]["id"]).Equal("db-1")
		gt.Value(t, items[1]["type"]).Equal("database")
		gt.Value(t, items[1]["title"]).Equal("Runbooks")
		gt.Value(t, items[1]["url"]).Equal("https://www.notion.so/db-1")
		gt.Value(t, items[1]["last_edited"]).Equal("2026-04-02T09:00:00Z")
		gt.Value(t, items[1]["read_tool"]).Equal("notion__get_database")
	})

	t.Run("names no reading tool for an unrecognised hit type", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{{ID: "x-1", Type: "data_source", Title: "Rows"}},
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "rows"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["read_tool"]).Equal("")
	})

	t.Run("returns error when query is absent", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})
		_, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil()
	})

	t.Run("names the searching tool on a database hit only", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{
				{ID: "page-1", Type: "page"},
				{ID: "db-1", Type: "database"},
			},
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "runbook"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(2).Required()
		gt.Value(t, items[0]["search_tool"]).Equal("")
		gt.Value(t, items[1]["search_tool"]).Equal("notion__search_database")
	})

	t.Run("counts the hits it returned", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{{ID: "page-1", Type: "page"}},
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "runbook"})
		gt.NoError(t, err).Required()
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(1)
	})

	t.Run("passes the ordering and the cursor through", func(t *testing.T) {
		fake := &fakeNotionClient{}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		_, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{
			"query":               "runbook",
			"sort_by_last_edited": "descending",
			"start_cursor":        "cursor-1",
			"page_size":           float64(50),
			"filter_type":         "database",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSearchOptions).Length(1).Required()
		opts := fake.gotSearchOptions[0]
		gt.String(t, opts.SortByEdit).Equal("descending")
		gt.String(t, opts.StartCursor).Equal("cursor-1")
		gt.Number(t, opts.PageSize).Equal(50)
		gt.String(t, opts.FilterType).Equal("database")
	})

	t.Run("asks Notion for no ordering or cursor when neither is given", func(t *testing.T) {
		fake := &fakeNotionClient{}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		_, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "runbook"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSearchOptions).Length(1).Required()
		gt.String(t, fake.gotSearchOptions[0].SortByEdit).Equal("")
		gt.String(t, fake.gotSearchOptions[0].StartCursor).Equal("")
	})
}

func TestSearchToolReportsTheParent(t *testing.T) {
	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__search")
	}

	hit := func(id, parentType, parentID string) notiontool.SearchItem {
		return notiontool.SearchItem{
			ID:     id,
			Type:   "page",
			Parent: notiontool.ParentRef{Type: parentType, ID: parentID},
		}
	}

	// Which database a hit came from is what decides whether it may be used as
	// evidence, and the name is what a citation shows.
	t.Run("reports the parent and resolves its name", func(t *testing.T) {
		fake := &fakeNotionClient{
			searchResult: &notiontool.SearchResult{Items: []notiontool.SearchItem{
				hit("row-1", "database", "db-1"),
				hit("child-1", "page", "page-9"),
				hit("loose-1", "workspace", ""),
			}},
			database: &notiontool.Database{ID: "db-1", Title: "Knowledge Base"},
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(3).Required()

		gt.Value(t, items[0]["parent_type"]).Equal("database")
		gt.Value(t, items[0]["parent_id"]).Equal("db-1")
		gt.Value(t, items[0]["parent_database_name"]).Equal("Knowledge Base")

		// A page parent has an id but no database title to resolve.
		gt.Value(t, items[1]["parent_type"]).Equal("page")
		gt.Value(t, items[1]["parent_id"]).Equal("page-9")
		gt.Value(t, items[1]["parent_database_name"]).Equal("")

		gt.Value(t, items[2]["parent_type"]).Equal("workspace")
		gt.Value(t, items[2]["parent_id"]).Equal("")

		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
	})

	t.Run("resolves a repeated parent once", func(t *testing.T) {
		fake := &fakeNotionClient{
			searchResult: &notiontool.SearchResult{Items: []notiontool.SearchItem{
				hit("row-1", "database", "db-1"),
				hit("row-2", "database", "db-1"),
				hit("row-3", "database", "db-1"),
			}},
			database: &notiontool.Database{ID: "db-1", Title: "Knowledge Base"},
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
		items := gt.Cast[[]map[string]any](t, got["items"])
		for _, item := range items {
			gt.Value(t, item["parent_database_name"]).Equal("Knowledge Base")
		}
	})

	// Notion rate-limits at roughly three requests a second, so the names run
	// out before the ids do.
	t.Run("stops resolving names past the cap but keeps every id", func(t *testing.T) {
		items := make([]notiontool.SearchItem, 0, 7)
		for i := 1; i <= 7; i++ {
			items = append(items, hit(fmt.Sprintf("row-%d", i), "database", fmt.Sprintf("db-%d", i)))
		}
		fake := &fakeNotionClient{
			searchResult: &notiontool.SearchResult{Items: items},
			database:     &notiontool.Database{Title: "Some database"},
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDatabaseIDs).Length(5)

		hits := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, hits).Length(7).Required()
		for i, item := range hits {
			gt.Value(t, item["parent_id"]).Equal(fmt.Sprintf("db-%d", i+1))
			if i < 5 {
				gt.Value(t, item["parent_database_name"]).Equal("Some database")
				continue
			}
			gt.Value(t, item["parent_database_name"]).Equal("")
		}
	})

	// A label that could not be read is not worth failing a search over.
	t.Run("keeps the search successful when a name cannot be read", func(t *testing.T) {
		fake := &fakeNotionClient{
			searchResult: &notiontool.SearchResult{Items: []notiontool.SearchItem{
				hit("row-1", "database", "db-1"),
			}},
			databaseErr: goerr.New("notion database endpoint returned HTTP 404 (object_not_found)"),
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(1)
		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Value(t, items[0]["parent_id"]).Equal("db-1")
		gt.Value(t, items[0]["parent_database_name"]).Equal("")
	})

	t.Run("does not retry a parent whose name already failed", func(t *testing.T) {
		fake := &fakeNotionClient{
			searchResult: &notiontool.SearchResult{Items: []notiontool.SearchItem{
				hit("row-1", "database", "db-1"),
				hit("row-2", "database", "db-1"),
			}},
			databaseErr: goerr.New("notion database endpoint returned HTTP 404 (object_not_found)"),
		}

		_, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()
		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
	})

	// The count bound does not bound the wait: each lookup is an HTTP request
	// under the client's own 30-second timeout. The search has already
	// succeeded by this point, so the labels get a deadline and the answer does
	// not wait past it.
	t.Run("stops naming parents when the time budget runs out", func(t *testing.T) {
		items := make([]notiontool.SearchItem, 0, 3)
		for i := 1; i <= 3; i++ {
			items = append(items, hit(fmt.Sprintf("row-%d", i), "database", fmt.Sprintf("db-%d", i)))
		}
		fake := &blockingNotionClient{
			searchResult: &notiontool.SearchResult{Items: items},
			release:      make(chan struct{}),
		}
		defer close(fake.release)

		tool := notiontool.NewSearchToolWithNameBudgetForTest(fake, 20*time.Millisecond)
		got, err := tool.Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		// The search still answers, with every id and no labels.
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(3)

		hits := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, hits).Length(3).Required()
		for i, item := range hits {
			gt.Value(t, item["parent_id"]).Equal(fmt.Sprintf("db-%d", i+1))
			gt.Value(t, item["parent_database_name"]).Equal("")
		}

		// The deadline stopped the phase rather than letting it work through
		// every parent.
		gt.Number(t, fake.databaseCalls()).LessOrEqual(2)
	})

	t.Run("reports an unknown parent as empty rather than guessing", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{Items: []notiontool.SearchItem{
			{ID: "row-1", Type: "page"},
		}}}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"query": "outage"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Value(t, items[0]["parent_type"]).Equal("")
		gt.Value(t, items[0]["parent_id"]).Equal("")
		gt.Array(t, fake.gotDatabaseIDs).Length(0)
	})
}

func TestGetDatabaseTool(t *testing.T) {
	rows := &notiontool.QueryResult{
		Items: []notiontool.SearchItem{{
			ID:         "row-1",
			Type:       "page",
			Title:      "Restart the ingest worker",
			URL:        "https://www.notion.so/row-1",
			LastEdited: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		}},
		HasMore:    true,
		NextCursor: "cursor-2",
	}

	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__get_database")
	}

	t.Run("lists the rows of the only data source", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				Title:       "Runbooks",
				URL:         "https://www.notion.so/db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"page_size":   float64(50),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-1"})
		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.Number(t, fake.gotQueryOptions[0].PageSize).Equal(50)

		gt.Value(t, got["database_id"]).Equal("db-1")
		gt.Value(t, got["title"]).Equal("Runbooks")
		gt.Value(t, got["url"]).Equal("https://www.notion.so/db-1")
		gt.Value(t, got["data_source_id"]).Equal("ds-1")
		gt.Value(t, got["has_more"]).Equal(true)
		gt.Value(t, got["next_cursor"]).Equal("cursor-2")

		sources := gt.Cast[[]map[string]any](t, got["data_sources"])
		gt.Array(t, sources).Length(1).Required()
		gt.Value(t, sources[0]["id"]).Equal("ds-1")
		gt.Value(t, sources[0]["name"]).Equal("Active")

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["id"]).Equal("row-1")
		gt.Value(t, items[0]["type"]).Equal("page")
		gt.Value(t, items[0]["title"]).Equal("Restart the ingest worker")
		gt.Value(t, items[0]["url"]).Equal("https://www.notion.so/row-1")
		gt.Value(t, items[0]["last_edited"]).Equal("2026-05-01T08:00:00Z")
	})

	t.Run("queries the requested data source when several exist", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-2",
			"start_cursor":   "cursor-1",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-2"})
		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.String(t, fake.gotQueryOptions[0].StartCursor).Equal("cursor-1")
		gt.Value(t, got["data_source_id"]).Equal("ds-2")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(1)
	})

	// The three no-rows cases below are reported as results, not errors: the
	// model can act on each by calling again, and an error would be filed as a
	// tool failure by the strategy that reports them.
	t.Run("reports the choices instead of rows when several data sources exist", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["data_source_id"]).Equal("")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("data_source_id")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["data_sources"])).Length(2)

		// A choice the caller has to make, which is what this status means.
		gt.Value(t, got["status"]).Equal("data_source_ambiguous")
		gt.Value(t, got["matched"]).Equal(0)
	})

	t.Run("reports an unknown data_source_id instead of querying it", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-nope",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["data_source_id"]).Equal("")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("not one of this database's data sources")

		// An id this database does not hold is an argument to correct, not one
		// of the listed ids to choose between — so it is not the ambiguous
		// status, which would send the caller looking through a list its own id
		// is not in.
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.Value(t, got["matched"]).Equal(0)
	})

	t.Run("reports a database that holds no data sources", func(t *testing.T) {
		fake := &fakeNotionClient{
			database:    &notiontool.Database{ID: "db-1", Title: "Empty"},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("no data sources")

		// A database with no data sources has no rows: an answer of zero, not
		// something to fix and not a choice to make. Reported as ambiguous, a
		// caller would keep asking for an id that does not exist.
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
	})

	t.Run("returns error when database_id is missing", func(t *testing.T) {
		fake := &fakeNotionClient{}
		_, err := newTool(fake).Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil().Required()
		gt.Array(t, fake.gotDatabaseIDs).Length(0)
	})

	t.Run("propagates the reason a database read failed", func(t *testing.T) {
		fake := &fakeNotionClient{databaseErr: goerr.New("notion database endpoint returned HTTP 404 (object_not_found)")}
		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("object_not_found")
	})

	t.Run("propagates the reason a data source query failed", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryErr: goerr.New("notion data source query endpoint returned HTTP 403 (restricted_resource)"),
		}
		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("restricted_resource")
		gt.String(t, err.Error()).Contains("failed to query notion data source")
	})
}

// schemaFake builds a client whose database holds one data source with a small
// but type-varied schema.
func schemaFake(rows *notiontool.QueryResult) *fakeNotionClient {
	return &fakeNotionClient{
		database: &notiontool.Database{
			ID:          "db-1",
			Title:       "Knowledge Base",
			URL:         "https://www.notion.so/db-1",
			DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
		},
		dataSource: &notiontool.DataSource{
			ID:   "ds-1",
			Name: "Active",
			// In name order, as GetDataSource returns them.
			Properties: []notiontool.PropertySchema{
				{ID: "aikw", Name: "Keywords", Type: "multi_select", Options: []string{"network", "storage"}},
				{ID: "title", Name: "Name", Type: "title"},
				{ID: "stat", Name: "Status", Type: "select", Options: []string{"Published", "Draft"}, OptionsTruncated: true},
				{ID: "sumr", Name: "Summary", Type: "rich_text"},
			},
		},
		queryResult: rows,
	}
}

func TestGetDatabaseToolDescribesTheSchema(t *testing.T) {
	rows := &notiontool.QueryResult{Items: []notiontool.SearchItem{{ID: "row-1", Type: "page", Title: "Reset a stuck job"}}}

	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__get_database")
	}

	t.Run("reports every property's name and type on the first page", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-1"})
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(1)

		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, schema).Length(4).Required()
		gt.Value(t, schema[0]["name"]).Equal("Keywords")
		gt.Value(t, schema[0]["type"]).Equal("multi_select")
		gt.Value(t, schema[1]["name"]).Equal("Name")
		gt.Value(t, schema[1]["type"]).Equal("title")
		gt.Value(t, schema[2]["name"]).Equal("Status")
		gt.Value(t, schema[2]["type"]).Equal("select")
		gt.Value(t, schema[3]["name"]).Equal("Summary")
		gt.Value(t, schema[3]["type"]).Equal("rich_text")

		// Choices are withheld until asked for: this data source has four
		// properties, a real one has dozens.
		for _, entry := range schema {
			gt.Value(t, entry["options"]).Nil()
		}

		operators := gt.Cast[map[string][]string](t, got["operators_by_type"])
		gt.Map(t, operators).HasKey("multi_select")
		gt.Map(t, operators).HasKey("rich_text")
		gt.Map(t, operators).HasKey("title")
		gt.Map(t, operators).HasKey("select")
		gt.Array(t, operators["multi_select"]).Equal([]string{"contains", "does_not_contain", "is_empty", "is_not_empty"})
		gt.Array(t, operators["select"]).Equal([]string{"equals", "does_not_equal", "is_empty", "is_not_empty"})
	})

	t.Run("repeats no schema on a later page", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":  "db-1",
			"start_cursor": "cursor-2",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["property_schema"]).Nil()
		gt.Value(t, got["operators_by_type"]).Nil()
		gt.Value(t, got["status"]).Equal("ok")
	})

	t.Run("reports the choices of the properties asked about", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": []any{"Status"},
		})
		gt.NoError(t, err).Required()

		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, schema).Length(4).Required()
		gt.Value(t, schema[0]["options"]).Nil()
		gt.Value(t, schema[2]["name"]).Equal("Status")
		gt.Array(t, gt.Cast[[]string](t, schema[2]["options"])).Equal([]string{"Published", "Draft"})
		gt.Value(t, schema[2]["options_truncated"]).Equal(true)
	})

	t.Run("asks for the schema on a later page when choices are requested", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"start_cursor":        "cursor-2",
			"describe_properties": []any{"Keywords"},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-1"})
		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, gt.Cast[[]string](t, schema[0]["options"])).Equal([]string{"network", "storage"})
	})

	// A bad property name is the model's own mistake, so it comes back as a
	// result it can repair from — with the schema attached — rather than as a
	// tool failure that the strategies would report to Sentry.
	t.Run("rejects an unknown property name without querying", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": []any{"Owner"},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)

		message := gt.Cast[string](t, got["message"])
		gt.String(t, message).Contains("Owner")
		gt.String(t, message).Contains("available properties")
		gt.String(t, message).Contains("Summary")

		gt.Array(t, gt.Cast[[]map[string]any](t, got["property_schema"])).Length(4)
	})

	t.Run("rejects a describe_properties that is not an array", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": "Status",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("must be an array")
	})

	// This one is NOT a rejection: the agent cannot repair a Notion outage or a
	// database it may not read, and reporting it as an empty result would let a
	// caller conclude there is no evidence.
	t.Run("propagates the reason a schema read failed", func(t *testing.T) {
		fake := schemaFake(rows)
		fake.dataSourceErr = goerr.New("notion data source endpoint returned HTTP 403 (restricted_resource)")

		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("restricted_resource")
		gt.String(t, err.Error()).Contains("failed to fetch notion data source schema")
		gt.Array(t, fake.gotDataSourceIDs).Length(0)
	})

	t.Run("says the data source is ambiguous rather than empty", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["status"]).Equal("data_source_ambiguous")
		gt.Value(t, got["matched"]).Equal(0)
	})

	t.Run("names the tool that searches these rows", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()
		gt.Value(t, got["search_tool"]).Equal("notion__search_database")
	})

	// An empty database is a real answer, and it must not read like a failure.
	t.Run("reports an empty database as a successful zero", func(t *testing.T) {
		fake := schemaFake(&notiontool.QueryResult{})

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Value(t, got["message"]).Nil()
	})

	// The listing half of this tool is untouched: it narrows nothing, orders
	// nothing and asks for no column values, so Notion receives what it always
	// received.
	t.Run("sends no filter, sorts or column selection", func(t *testing.T) {
		fake := schemaFake(rows)

		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.Value(t, fake.gotQueryOptions[0].Filter).Nil()
		gt.Array(t, fake.gotQueryOptions[0].Sorts).Length(0)
		gt.Array(t, fake.gotQueryOptions[0].Properties).Length(0)
	})
}

func TestSearchDatabaseTool(t *testing.T) {
	rows := &notiontool.QueryResult{
		Items: []notiontool.SearchItem{{
			ID:         "row-1",
			Type:       "page",
			Title:      "Reset a stuck job",
			URL:        "https://www.notion.so/row-1",
			LastEdited: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
			Properties: map[string]string{"Summary": "Steps to clear the queue and retry."},
		}},
		HasMore:    true,
		NextCursor: "cursor-2",
	}

	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__search_database")
	}

	t.Run("searches one database by keyword and returns the named columns", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":       "db-1",
			"query":             "stuck job",
			"search_properties": []any{"Name", "Summary"},
			"properties":        []any{"Summary"},
			"sorts":             []any{map[string]any{"timestamp": "last_edited_time", "direction": "descending"}},
			"page_size":         float64(50),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-1"})
		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-1"})
		gt.Array(t, fake.gotQueryOptions).Length(1).Required()

		opts := fake.gotQueryOptions[0]
		gt.Number(t, opts.PageSize).Equal(50)
		gt.Value(t, opts.Filter).NotNil().Required()
		gt.Array(t, opts.Sorts).Length(1).Required()
		gt.Value(t, opts.Sorts[0]["timestamp"]).Equal("last_edited_time")
		gt.Value(t, opts.Sorts[0]["direction"]).Equal("descending")
		gt.Array(t, opts.Properties).Length(1).Required()
		gt.String(t, opts.Properties[0].Name).Equal("Summary")

		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(1)
		gt.Value(t, got["database_id"]).Equal("db-1")
		gt.Value(t, got["database_title"]).Equal("Knowledge Base")
		gt.Value(t, got["data_source_id"]).Equal("ds-1")
		gt.Value(t, got["has_more"]).Equal(true)
		gt.Value(t, got["next_cursor"]).Equal("cursor-2")

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["id"]).Equal("row-1")
		gt.Value(t, items[0]["title"]).Equal("Reset a stuck job")
		gt.Value(t, items[0]["url"]).Equal("https://www.notion.so/row-1")
		gt.Value(t, items[0]["last_edited"]).Equal("2026-05-01T08:00:00Z")
		gt.Value(t, items[0]["read_tool"]).Equal("notion__get_page")

		values := gt.Cast[map[string]string](t, items[0]["properties"])
		gt.Value(t, values["Summary"]).Equal("Steps to clear the queue and retry.")
	})

	t.Run("omits the column values a row does not carry", func(t *testing.T) {
		fake := schemaFake(&notiontool.QueryResult{
			Items: []notiontool.SearchItem{{ID: "row-1", Type: "page", Title: "Reset a stuck job"}},
		})

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["properties"]).Nil()
	})

	// Narrowing nothing is a legitimate call: it is how an agent asks for the
	// column values of the first page of rows.
	t.Run("searches with no filter when nothing narrows", func(t *testing.T) {
		fake := schemaFake(rows)

		_, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"properties":  []any{"Summary"},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.Value(t, fake.gotQueryOptions[0].Filter).Nil()
		gt.Array(t, fake.gotQueryOptions[0].Sorts).Length(0)
		gt.Array(t, fake.gotQueryOptions[0].Properties).Length(1)
	})

	t.Run("reports zero rows as a successful search", func(t *testing.T) {
		fake := schemaFake(&notiontool.QueryResult{})

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"query":       "nothing here",
		})
		gt.NoError(t, err).Required()

		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Value(t, got["message"]).Nil()
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
	})

	// A repairable mistake comes back as a result with the schema attached, so
	// the agent can fix the call without asking for the schema again — and
	// without the strategies filing a Sentry issue per attempt.
	t.Run("rejects an unusable condition without querying", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"filter": map[string]any{"conditions": []any{
				map[string]any{"property": "Status", "operator": "contains", "value": "Publ"},
			}},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)

		message := gt.Cast[string](t, got["message"])
		gt.String(t, message).Contains("contains")
		gt.String(t, message).Contains("Status")

		gt.Array(t, gt.Cast[[]map[string]any](t, got["property_schema"])).Length(4)
		gt.Map(t, gt.Cast[map[string][]string](t, got["operators_by_type"])).HasKey("select")
	})

	// A value that cannot be encoded as JSON has to be caught here. Sent on, it
	// fails when the request body is marshalled, which reaches the agent as an
	// internal error rather than as an argument it can restate.
	t.Run("rejects a value that could not be sent as JSON", func(t *testing.T) {
		fake := schemaFake(rows)
		fake.dataSource = &notiontool.DataSource{
			ID: "ds-1",
			Properties: []notiontool.PropertySchema{
				{ID: "title", Name: "Name", Type: "title"},
				{ID: "rank", Name: "Rank", Type: "number"},
			},
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"filter": map[string]any{"conditions": []any{
				map[string]any{"property": "Rank", "operator": "greater_than", "value": "NaN"},
			}},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("digits")
	})

	// An argument whose shape is wrong is caught before the schema is even read.
	t.Run("rejects a malformed argument before calling Notion", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"sorts":       "last_edited_time",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("sorts must be an array")
	})

	t.Run("reports an ambiguous data source without reading the schema", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"query":       "stuck",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("data_source_ambiguous")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["data_sources"])).Length(2)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("data_source_id")
	})

	// The three ways a data source cannot be named ask three different things
	// of the caller, so they do not share one status.
	t.Run("separates an unknown data source from an unmade choice", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-nope",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("not one of this database's data sources")
	})

	t.Run("reports a database with no data sources as zero rows", func(t *testing.T) {
		fake := &fakeNotionClient{
			database:    &notiontool.Database{ID: "db-1", Title: "Empty"},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"query":       "stuck",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("no data sources")
	})

	t.Run("searches the requested data source when several exist", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			dataSource: &notiontool.DataSource{
				ID:         "ds-2",
				Properties: []notiontool.PropertySchema{{ID: "title", Name: "Name", Type: "title"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-2",
			"query":          "stuck",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-2"})
		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-2"})
		gt.Value(t, got["data_source_id"]).Equal("ds-2")
		gt.Value(t, got["status"]).Equal("ok")
	})

	t.Run("returns error when database_id is missing", func(t *testing.T) {
		fake := schemaFake(rows)
		_, err := newTool(fake).Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil().Required()
		gt.Array(t, fake.gotDatabaseIDs).Length(0)
	})

	// These are NOT rejections: a caller told "no rows" would conclude there is
	// no evidence, when in fact the search never ran.
	t.Run("propagates the reason a call to Notion failed", func(t *testing.T) {
		t.Run("database read", func(t *testing.T) {
			fake := schemaFake(rows)
			fake.databaseErr = goerr.New("notion database endpoint returned HTTP 404 (object_not_found)")
			_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("object_not_found")
		})

		t.Run("schema read", func(t *testing.T) {
			fake := schemaFake(rows)
			fake.dataSourceErr = goerr.New("notion data source endpoint returned HTTP 403 (restricted_resource)")
			_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("restricted_resource")
			gt.String(t, err.Error()).Contains("failed to fetch notion data source schema")
		})

		t.Run("row query", func(t *testing.T) {
			fake := schemaFake(rows)
			fake.queryErr = goerr.New("notion data source query endpoint returned HTTP 429 (rate_limited)")
			_, err := newTool(fake).Run(context.Background(), map[string]any{
				"database_id": "db-1",
				"query":       "stuck job",
			})
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("rate_limited")
			gt.String(t, err.Error()).Contains("failed to query notion data source")
		})
	})

	// A failed tool call's goerr values are rendered into the response the model
	// reads and reproduced in the Slack thread, so a failure carries the size of
	// the request and never the row content it searched for.
	t.Run("attaches the request size but not the conditions to a failure", func(t *testing.T) {
		fake := schemaFake(rows)
		fake.queryErr = goerr.New("notion data source query endpoint returned HTTP 500")

		_, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"query":       "stuck job",
			"filter": map[string]any{"conditions": []any{
				map[string]any{"property": "Status", "operator": "equals", "value": "Published"},
			}},
		})
		gt.Value(t, err).NotNil().Required()

		values := goerr.Values(err)
		gt.Value(t, values["condition_count"]).Equal(3)
		rendered := fmt.Sprintf("%v", values)
		gt.Bool(t, strings.Contains(rendered, "stuck")).False()
		gt.Bool(t, strings.Contains(rendered, "Published")).False()
	})
}
