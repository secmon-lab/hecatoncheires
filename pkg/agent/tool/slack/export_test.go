package slacktool

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

// NewSearchClientWithAPIURLForTest builds a SearchService whose underlying slack.Client
// targets the given API URL. Used to point the client at httptest servers in unit tests.
func NewSearchClientWithAPIURLForTest(userToken, apiURL string) SearchService {
	return &searchClient{
		api: slack.New(userToken, slack.OptionAPIURL(apiURL)),
	}
}

// SlackErrorAttrsForTest re-exports slackErrorAttrs so the helper's
// extraction behaviour can be unit-tested without importing the slack-go
// types from the test package machinery.
func SlackErrorAttrsForTest(err error) []goerr.Option { return slackErrorAttrs(err) }
