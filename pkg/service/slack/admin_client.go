package slack

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

// AdminService provides admin-level Slack API operations using a User OAuth Token.
// These operations require admin.conversations:write scope and can only be performed
// with a User token (xoxp-), not a Bot token (xoxb-).
type AdminService interface {
	// ConnectChannelToWorkspace adds target workspaces to a channel's visibility
	// using admin.conversations.setTeams API (Enterprise Grid only).
	ConnectChannelToWorkspace(ctx context.Context, channelID string, targetTeamIDs []string) error
}

// adminClient implements AdminService using a Slack User OAuth Token
type adminClient struct {
	api *slack.Client
}

// NewAdminClient creates a new AdminService with the provided User OAuth Token
func NewAdminClient(userToken string) (AdminService, error) {
	if userToken == "" {
		return nil, goerr.New("Slack User OAuth Token is required")
	}

	return &adminClient{
		api: slack.New(userToken),
	}, nil
}

// ConnectChannelToWorkspace adds target workspaces to a channel's visibility
// using admin.conversations.setTeams API (Enterprise Grid only).
func (c *adminClient) ConnectChannelToWorkspace(ctx context.Context, channelID string, targetTeamIDs []string) error {
	err := c.api.AdminConversationsSetTeams(ctx, slack.AdminConversationsSetTeamsParams{
		ChannelID:     channelID,
		TargetTeamIDs: targetTeamIDs,
	})
	if err != nil {
		return goerr.Wrap(err, "failed to connect channel to workspaces",
			goerr.V("channel_id", channelID),
			goerr.V("target_team_ids", targetTeamIDs))
	}
	return nil
}
