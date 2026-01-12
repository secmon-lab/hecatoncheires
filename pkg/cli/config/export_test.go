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
