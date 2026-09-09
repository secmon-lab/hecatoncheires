package notiontool

import "time"

// Notion's property types, as they appear in a data source's schema and in a
// row's property values. They are named here rather than at each use because
// the same string is the type in the schema, the key of a value object, and —
// for most types — the key a filter condition takes.
const (
	propTypeTitle          = "title"
	propTypeRichText       = "rich_text"
	propTypeURL            = "url"
	propTypeEmail          = "email"
	propTypePhoneNumber    = "phone_number"
	propTypeNumber         = "number"
	propTypeUniqueID       = "unique_id"
	propTypeCheckbox       = "checkbox"
	propTypeSelect         = "select"
	propTypeStatus         = "status"
	propTypeMultiSelect    = "multi_select"
	propTypeDate           = "date"
	propTypeCreatedTime    = "created_time"
	propTypeLastEditedTime = "last_edited_time"
	propTypePeople         = "people"
	propTypeCreatedBy      = "created_by"
	propTypeLastEditedBy   = "last_edited_by"
	propTypeRelation       = "relation"
	propTypeFiles          = "files"
	propTypeFormula        = "formula"
	propTypeRollup         = "rollup"
)

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
	// Properties holds the values of the properties QueryOptions.Properties
	// asked for, rendered as text and keyed by property name. Nil on every
	// other path, including Search.
	Properties map[string]string
	// Parent names what this hit belongs to. Only filled in by Search: a data
	// source query's rows all share the queried data source as their parent.
	Parent ParentRef
}

// ParentRef names what a search hit belongs to. Notion reports the parent's id
// but never its title, so Name is left to the caller that resolves it.
type ParentRef struct {
	// Type is "database", "page", "block", "workspace", or empty when Notion
	// reported a parent kind this code does not model.
	Type string
	// ID is the parent's id. Empty when Type is "workspace" or unknown.
	ID string
	// Name is the parent database's title. Set only for Type == "database", and
	// only when the caller resolved it.
	Name string
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

// DataSource is one data source's column schema, as returned by
// GET /v1/data_sources/{id}. A database object reports only the id and name of
// each data source, so this is the only way to learn what a row's columns are —
// which is what a filter has to be written against.
type DataSource struct {
	ID   string
	Name string
	// Properties are ordered by name, so two calls describing the same data
	// source produce the same list.
	Properties []PropertySchema
}

// PropertySchema is one column definition of a data source.
type PropertySchema struct {
	// ID is Notion's property id. A title property's id is always "title".
	ID string
	// Name is the property's display name, unique within the data source.
	Name string
	// Type is Notion's property type ("rich_text", "number", "select", ...). A
	// type this package does not model is still reported: the agent should be
	// able to see that the column exists even when it cannot filter on it.
	Type string
	// Options are the choice names of a select / status / multi_select
	// property, capped at schemaOptionsMax. Empty for every other type.
	Options []string
	// OptionsTruncated says whether Options was cut at that cap.
	OptionsTruncated bool
}

// QueryOptions configures a QueryDataSource call.
type QueryOptions struct {
	// PageSize is the maximum number of rows to return. Clamped to [1, 100]; defaults to 20 when zero.
	PageSize int
	// StartCursor is the pagination cursor returned by a previous call. Empty starts from the beginning.
	StartCursor string
	// Filter is the Notion filter object to send. Nil omits the key entirely,
	// which is what an unfiltered listing sends.
	Filter map[string]any
	// Sorts is the Notion sorts array to send. Empty omits the key entirely.
	Sorts []map[string]any
	// Properties are the columns whose values each row should carry. Empty
	// sends no filter_properties and leaves SearchItem.Properties nil.
	Properties []PropertySchema
}

// QueryResult is the response of a QueryDataSource call. A data source's rows
// are pages, so each carries the same shape as a search hit.
type QueryResult struct {
	Items      []SearchItem
	HasMore    bool
	NextCursor string
}
