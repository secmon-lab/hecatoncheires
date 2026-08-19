package notiontool

import "time"

// SearchOptions configures a Search call.
type SearchOptions struct {
	// PageSize is the maximum number of results to return. Clamped to [1, 100]; defaults to 20 when zero.
	PageSize int
	// FilterType narrows results by Notion object type. Empty for no filter,
	// "page" for pages only, "database" for databases only.
	FilterType string
	// SortByEdit orders results by last_edited_time. Empty for default Notion ordering,
	// "ascending" for oldest first, "descending" for most recent first.
	SortByEdit string
	// StartCursor is the pagination cursor returned by a previous call. Empty starts from the beginning.
	StartCursor string
}

// SearchResult is the response of a Search call.
type SearchResult struct {
	Items      []SearchItem
	HasMore    bool
	NextCursor string
}

// SearchItem is a single matched page or database in the search response.
type SearchItem struct {
	ID         string
	Type       string // "page" or "database"
	Title      string
	URL        string
	LastEdited time.Time
}

// PageMarkdown is the response of GetPageMarkdown.
type PageMarkdown struct {
	PageID    string
	Markdown  string
	Truncated bool
}

// Database is the response of GetDatabase.
type Database struct {
	ID         string
	Title      string
	URL        string
	LastEdited time.Time
	// DataSources are the row collections this database holds. Notion's
	// 2025-09-03 API split moved the rows out of the database object itself, so
	// listing a database's contents means querying one of these.
	DataSources []DataSourceRef
}

// DataSourceRef names one data source of a database.
type DataSourceRef struct {
	ID   string
	Name string
}

// QueryOptions configures a QueryDataSource call.
type QueryOptions struct {
	// PageSize is the maximum number of rows to return. Clamped to [1, 100]; defaults to 20 when zero.
	PageSize int
	// StartCursor is the pagination cursor returned by a previous call. Empty starts from the beginning.
	StartCursor string
}

// QueryResult is the response of a QueryDataSource call. A data source's rows
// are pages, so each carries the same shape as a search hit.
type QueryResult struct {
	Items      []SearchItem
	HasMore    bool
	NextCursor string
}
