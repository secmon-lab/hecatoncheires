package notiontool

import (
	"net/http"
	"net/url"

	"github.com/jomei/notionapi"
)

// rewriteTransport rewrites every request's scheme/host so the official notionapi
// client (which hardcodes https://api.notion.com) can be pointed at a httptest server.
type rewriteTransport struct {
	target *url.URL
}

func (rt *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = rt.target.Scheme
	r.URL.Host = rt.target.Host
	r.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

// NewClientWithBaseURLForTest builds a Client whose underlying notionapi client
// and raw markdown HTTP client both target the given base URL (typically a
// httptest server).
func NewClientWithBaseURLForTest(token, apiBaseURL string) Client {
	u, err := url.Parse(apiBaseURL)
	if err != nil {
		panic(err)
	}
	httpClient := &http.Client{Transport: &rewriteTransport{target: u}}
	return &client{
		api: notionapi.NewClient(
			notionapi.Token(token),
			notionapi.WithHTTPClient(httpClient),
		),
		token:      token,
		httpClient: httpClient,
		apiBaseURL: apiBaseURL,
	}
}
