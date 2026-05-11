package slacktool

import (
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

// NewSearchClientWithAPIURLForTest builds a SearchService whose underlying slack.Client
// targets the given API URL. The client is wrapped with the same scope-capturing
// transport as the production client so tests exercise that code path too.
func NewSearchClientWithAPIURLForTest(userToken, apiURL string) SearchService {
	return newUserClientWithAPIURLForTest(userToken, apiURL)
}

// NewMessageRetrieverWithAPIURLForTest builds a MessageRetriever whose underlying
// slack.Client targets the given API URL. Mirrors NewSearchClientWithAPIURLForTest
// for the conversations.* methods.
func NewMessageRetrieverWithAPIURLForTest(userToken, apiURL string) MessageRetriever {
	return newUserClientWithAPIURLForTest(userToken, apiURL)
}

func newUserClientWithAPIURLForTest(userToken, apiURL string) *searchClient {
	httpClient := &capturingHTTPClient{inner: &http.Client{}}
	return &searchClient{
		api: slack.New(userToken, slack.OptionAPIURL(apiURL), slack.OptionHTTPClient(httpClient)),
	}
}

// SlackErrorAttrsForTest re-exports slackErrorAttrs so the helper's
// extraction behaviour can be unit-tested without importing the slack-go
// types from the test package machinery.
func SlackErrorAttrsForTest(err error) []goerr.Option { return slackErrorAttrs(err) }
