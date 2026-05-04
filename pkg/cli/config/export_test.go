package config

import (
	"context"

	"github.com/slack-go/slack"
)

// NewSlackForTest creates a Slack config for testing purposes
func NewSlackForTest(clientID, clientSecret, botToken, signingSecret, noAuthUID string) *Slack {
	return &Slack{
		clientID:      clientID,
		clientSecret:  clientSecret,
		botToken:      botToken,
		signingSecret: signingSecret,
		noAuthUID:     noAuthUID,
	}
}

// SetOrgLevelForTest sets org-level detection results for testing purposes
func (x *Slack) SetOrgLevelForTest(isOrgLevel bool, authTeamID string) {
	x.isOrgLevel = isOrgLevel
	x.authTeamID = authTeamID
}

// MockSlackAuthAPI is a test double for slackAuthAPI
type MockSlackAuthAPI struct {
	AuthTestResp *slack.AuthTestResponse
	AuthTestErr  error
	Teams        []slack.Team
	ListTeamsErr error
}

func (m *MockSlackAuthAPI) AuthTestContext(_ context.Context) (*slack.AuthTestResponse, error) {
	return m.AuthTestResp, m.AuthTestErr
}

func (m *MockSlackAuthAPI) ListTeamsContext(_ context.Context, _ slack.ListTeamsParameters) ([]slack.Team, string, error) {
	return m.Teams, "", m.ListTeamsErr
}

// SetAuthAPIForTest injects a mock slackAuthAPI for testing DetectOrgLevel
func (x *Slack) SetAuthAPIForTest(api *MockSlackAuthAPI) {
	x.authAPI = api
}

// NewLLMForTest creates an LLM config for testing purposes.
func NewLLMForTest(provider, model, openaiAPIKey, claudeAPIKey, geminiProjectID, geminiLocation string) *LLM {
	return &LLM{
		provider:        provider,
		model:           model,
		openaiAPIKey:    openaiAPIKey,
		claudeAPIKey:    claudeAPIKey,
		geminiProjectID: geminiProjectID,
		geminiLocation:  geminiLocation,
	}
}

// NewEmbeddingForTest creates an Embedding config for testing purposes.
func NewEmbeddingForTest(geminiProjectID, geminiLocation, model string) *Embedding {
	return &Embedding{
		geminiProjectID: geminiProjectID,
		geminiLocation:  geminiLocation,
		model:           model,
	}
}
