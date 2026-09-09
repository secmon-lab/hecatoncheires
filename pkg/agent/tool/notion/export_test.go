package notiontool

import (
	"encoding/json"
	"net/http"
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
