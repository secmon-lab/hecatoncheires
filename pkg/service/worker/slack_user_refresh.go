package worker

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// SlackUserRefreshWorker manages background refresh of Slack users from Slack API to database
//
// Architecture assumptions:
// - Single server instance (no distributed locking)
// - For future horizontal scaling, implement distributed locking or leader election
type SlackUserRefreshWorker struct {
	repo         interfaces.Repository
	slackService slack.Service
	interval     time.Duration
	isOrgLevel   bool
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewSlackUserRefreshWorker creates a new worker for refreshing Slack users.
// If isOrgLevel is true, the worker will call ListTeams to discover all workspaces
// and then ListUsers per workspace.
func NewSlackUserRefreshWorker(repo interfaces.Repository, slackSvc slack.Service, interval time.Duration, isOrgLevel bool) *SlackUserRefreshWorker {
	return &SlackUserRefreshWorker{
		repo:         repo,
		slackService: slackSvc,
		interval:     interval,
		isOrgLevel:   isOrgLevel,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins the background refresh loop
// - Initial sync and periodic refresh both run in a background goroutine
// - Does not block server startup
func (w *SlackUserRefreshWorker) Start(ctx context.Context) error {
	logging.Default().Info("Slack user refresh worker starting",
		"interval", w.interval.String())

	go w.run(ctx)

	return nil
}

// Stop signals the worker to stop and waits for completion
func (w *SlackUserRefreshWorker) Stop() {
	logging.Default().Info("Slack user refresh worker stopping")
	close(w.stopCh)
	<-w.doneCh
	logging.Default().Info("Slack user refresh worker stopped")
}

// run is the main worker loop (runs in goroutine)
func (w *SlackUserRefreshWorker) run(ctx context.Context) {
	defer close(w.doneCh)

	// Initial sync (runs in goroutine, does not block server startup)
	if err := w.refresh(ctx); err != nil {
		logging.Default().Error("Initial Slack user refresh failed (will retry next interval)",
			"error", err.Error())
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.refresh(ctx); err != nil {
				// Log error but continue worker
				logging.Default().Error("Slack user refresh failed (will retry next interval)",
					"error", err.Error())
			}

		case <-w.stopCh:
			logging.Default().Info("Slack user refresh worker received stop signal")
			return

		case <-ctx.Done():
			logging.Default().Info("Slack user refresh worker context cancelled")
			return
		}
	}
}

// refresh performs a single refresh cycle (Replace strategy: DeleteAll → SaveMany)
func (w *SlackUserRefreshWorker) refresh(ctx context.Context) error {
	startTime := time.Now()
	logging.Default().Info("Starting Slack user refresh")

	// Get existing metadata to preserve values on failure
	existingMetadata, err := w.repo.SlackUser().GetMetadata(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to get existing metadata")
	}

	// Update metadata: attempt started
	attemptMetadata := &model.SlackUserMetadata{
		LastRefreshSuccess: existingMetadata.LastRefreshSuccess,
		LastRefreshAttempt: startTime,
		UserCount:          existingMetadata.UserCount,
	}
	if err := w.repo.SlackUser().SaveMetadata(ctx, attemptMetadata); err != nil {
		return goerr.Wrap(err, "failed to save refresh attempt metadata")
	}

	// Fetch all users from Slack API
	var slackUsers []*slack.User
	if w.isOrgLevel {
		// Org-level app: discover all workspaces and fetch users per workspace
		teams, err := w.slackService.ListTeams(ctx)
		if err != nil {
			return goerr.Wrap(err, "failed to list teams for org-level user refresh")
		}

		seen := make(map[string]struct{})
		for _, team := range teams {
			users, err := w.slackService.ListUsers(ctx, team.ID)
			if err != nil {
				return goerr.Wrap(err, "failed to list Slack users from API",
					goerr.V("team_id", team.ID),
					goerr.V("team_name", team.Name))
			}
			for _, u := range users {
				if _, ok := seen[u.ID]; !ok {
					seen[u.ID] = struct{}{}
					slackUsers = append(slackUsers, u)
				}
			}
		}

		logging.Default().Info("fetched users from all workspaces",
			"team_count", len(teams),
			"unique_user_count", len(slackUsers),
		)
	} else {
		// WS-level app: single call without team_id
		var err error
		slackUsers, err = w.slackService.ListUsers(ctx, "")
		if err != nil {
			return goerr.Wrap(err, "failed to list Slack users from API")
		}
	}

	// Convert to domain models
	users := make([]*model.SlackUser, len(slackUsers))
	for i, su := range slackUsers {
		users[i] = &model.SlackUser{
			ID:        model.SlackUserID(su.ID),
			Name:      su.Name,
			RealName:  su.RealName,
			Email:     su.Email,
			ImageURL:  su.ImageURL,
			UpdatedAt: startTime,
		}
	}

	// Replace strategy: DeleteAll → SaveMany
	// This prevents orphaned records and simplifies implementation
	if err := w.repo.SlackUser().DeleteAll(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete existing Slack users")
	}

	if err := w.repo.SlackUser().SaveMany(ctx, users); err != nil {
		return goerr.Wrap(err, "failed to save Slack users", goerr.V("count", len(users)))
	}

	// Update metadata: success
	successMetadata := &model.SlackUserMetadata{
		LastRefreshSuccess: startTime,
		LastRefreshAttempt: startTime,
		UserCount:          len(users),
	}
	if err := w.repo.SlackUser().SaveMetadata(ctx, successMetadata); err != nil {
		return goerr.Wrap(err, "failed to save refresh success metadata")
	}

	duration := time.Since(startTime)
	logging.Default().Info("Slack user refresh completed",
		"count", len(users),
		"duration", duration.String())

	return nil
}
