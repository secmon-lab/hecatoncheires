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
