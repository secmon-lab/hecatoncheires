package config

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

// NewGeminiForTest creates a Gemini config for testing purposes
func NewGeminiForTest(projectID, location string) *Gemini {
	return &Gemini{
		projectID: projectID,
		location:  location,
	}
}
