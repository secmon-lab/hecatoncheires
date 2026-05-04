package slacktool

import "github.com/slack-go/slack"

// NewSearchClientWithAPIURLForTest builds a SearchService whose underlying slack.Client
// targets the given API URL. Used to point the client at httptest servers in unit tests.
func NewSearchClientWithAPIURLForTest(userToken, apiURL string) SearchService {
	return &searchClient{
		api: slack.New(userToken, slack.OptionAPIURL(apiURL)),
	}
}
