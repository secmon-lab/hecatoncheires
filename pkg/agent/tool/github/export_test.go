package github

import (
	"net/http"

	ghapi "github.com/google/go-github/v88/github"
	"github.com/shurcooL/githubv4"
)

// NewTestClient builds a Client whose REST and GraphQL traffic both go to the
// given base URL. The httptest server is expected to mount REST endpoints
// under "/" and the GraphQL endpoint under "/api/graphql" (matching what
// githubv4.NewEnterpriseClient produces).
//
// Exposed only to tests in this package via export_test.go.
func NewTestClient(baseURL string, httpClient *http.Client) *Client {
	// WithURLs adds the trailing slash go-github requires, so callers can
	// pass the raw httptest.Server URL as-is.
	rest, err := ghapi.NewClient(ghapi.WithHTTPClient(httpClient), ghapi.WithURLs(&baseURL, &baseURL))
	if err != nil {
		// baseURL/httpClient come from the caller's httptest.Server, so this
		// should never fail in practice; panic loudly rather than returning
		// a half-built test double that would produce confusing downstream
		// failures.
		panic(err)
	}

	gql := githubv4.NewEnterpriseClient(baseURL+"/api/graphql", httpClient)

	return &Client{gql: gql, restHTTP: httpClient, restClient: rest}
}

// SafeTruncateForTest is exported for testing the byte-safe truncation helper.
var SafeTruncateForTest = safeTruncate

// MaxFileBytesForTest exposes the file-size cap for tests that need to
// generate over-limit fixtures without hard-coding the constant.
const MaxFileBytesForTest = maxFileBytes

// ToolClientForTest is the package-private toolClient interface re-exported
// for the external test file (tools_test.go). Production code never uses
// this — *Client and external fakes both satisfy it implicitly.
type ToolClientForTest = toolClient

// NewToolsForTest is the package-private newTools constructor re-exported so
// tests can inject a fake without going through the public New(*Client).
var NewToolsForTest = newTools
