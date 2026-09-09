package notiontool

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gollem-dev/gollem"
)

// NewClientWithBaseURLForTest builds a Client whose Notion API requests all
// target the given base URL (typically a httptest server).
func NewClientWithBaseURLForTest(token, apiBaseURL string) Client {
	return &client{
		token:      token,
		httpClient: &http.Client{},
		apiBaseURL: apiBaseURL,
	}
}

// NewSearchToolWithNameBudgetForTest builds the search tool with a shortened
// deadline for the parent-name lookups, so a test can drive the budget running
// out without waiting the production five seconds.
func NewSearchToolWithNameBudgetForTest(client Client, budget time.Duration) gollem.Tool {
	return &searchTool{client: client, nameBudget: budget}
}

// RenderPropertyValueForTest exposes the rendering of one Notion property value
// as text. Driving it through a data source query would put an httptest server
// between the test and what is a pure function of one JSON value.
func RenderPropertyValueForTest(raw json.RawMessage) (string, bool) {
	return renderPropertyValue(raw)
}

// ResolvePropertyForTest exposes the property-name lookup, including the
// rejection it produces for a name the data source does not hold.
func ResolvePropertyForTest(ds *DataSource, name string) (PropertySchema, error) {
	return resolveProperty(ds, name)
}

// OperatorsForTest exposes the operator catalog for one property type. Nil means
// the type cannot be filtered on.
func OperatorsForTest(propType string) []string {
	return operatorsFor(propType)
}

// ParseNameListForTest exposes the array-of-property-names argument parser.
func ParseNameListForTest(args map[string]any, key string, max int) ([]string, error) {
	return parseNameList(args, key, max)
}

// IsRejectionForTest reports whether err is the kind of failure the tools answer
// as a result rather than propagate.
func IsRejectionForTest(err error) bool {
	_, ok := rejectionMessage(err)
	return ok
}

// BuildFilterForTest exposes the whole path from tool arguments to a Notion
// filter object: the keywords and the filter argument are parsed the way the
// tool parses them, then combined against the given schema. Asserting on the
// object itself is what pins the condition keys Notion requires, which an
// assertion on the tool's response could not see.
func BuildFilterForTest(ds *DataSource, args map[string]any) (map[string]any, error) {
	keywords, err := parseSearchQuery(args)
	if err != nil {
		return nil, err
	}
	spec, err := parseFilterArgs(args)
	if err != nil {
		return nil, err
	}
	names, err := parseNameList(args, "search_properties", searchPropertiesMax)
	if err != nil {
		return nil, err
	}

	var targets []PropertySchema
	if len(keywords) > 0 || len(names) > 0 {
		targets, err = resolveSearchTargets(ds, names)
		if err != nil {
			return nil, err
		}
	}
	return buildFilter(ds, keywords, targets, spec)
}

// BuildSortsForTest exposes the same path for the ordering keys.
func BuildSortsForTest(ds *DataSource, args map[string]any) ([]map[string]any, error) {
	specs, err := parseSortArgs(args)
	if err != nil {
		return nil, err
	}
	return buildSorts(ds, specs)
}
