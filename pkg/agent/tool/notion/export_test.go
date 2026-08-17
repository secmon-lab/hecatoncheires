package notiontool

import (
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
