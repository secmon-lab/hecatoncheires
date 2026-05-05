package diagnosis

import (
	"context"
	"errors"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// FixUnsentActionsReport summarises the outcome of a FixUnsentActions sweep.
type FixUnsentActionsReport struct {
	// Total is the number of candidate actions (SlackMessageTS == "")
	// across every workspace inspected.
	Total int
	// Fixed is the number of candidates whose Slack message was posted
	// successfully and whose timestamp was persisted.
	Fixed int
	// Skipped is the number of candidates that the underlying
	// PostSlackMessageToAction explicitly declined: parent case has no
	// Slack channel, or the action was already posted between listing
	// and posting (race).
	Skipped int
	// Failed is the number of candidates that returned an unexpected
	// error from the post path. These are reported via errutil.Handle so
	// they reach the configured error sink, but the sweep continues
	// regardless.
	Failed int
}

// FixUnsentActions sweeps every workspace in the registry, finds Actions
// whose SlackMessageTS is empty, and replays the Slack post for each via
// the unified ActionUseCase.PostSlackMessageToAction entry point. The
// sweep is best-effort per item: a failure on one action never stops the
// rest, so a single Slack hiccup or one orphaned record cannot block
// recovery of the long tail.
//
// Idempotency: the candidate filter excludes actions that already have a
// timestamp, so repeat runs are safe. PostSlackMessageToAction itself
// returns ErrSlackMessageAlreadyPosted if a concurrent run sneaks in
// between our list and our post; that race is bucketed into Skipped.
//
// This routine intentionally does NOT route every Action change through
// the standard CreateAction / UpdateAction surfaces — the spec for this
// repair carved out an exemption that the repair may bypass the usecase
// layer. In practice the post / persist pair is owned by the shared
// ActionUseCase.postSlackMessageForAction helper, so we still benefit
// from the same Slack rendering and timestamp-write logic that
// CreateAction uses.
func (uc *UseCase) FixUnsentActions(ctx context.Context) (FixUnsentActionsReport, error) {
	logger := logging.From(ctx)

	if uc.registry == nil {
		return FixUnsentActionsReport{}, goerr.New("workspace registry is not configured")
	}
	if uc.actionUC == nil {
		return FixUnsentActionsReport{}, goerr.New("action poster is not configured")
	}

	var report FixUnsentActionsReport

	for _, entry := range uc.registry.List() {
		workspaceID := entry.Workspace.ID
		// One-shot in-memory scan per workspace: this is an operator-run
		// repair tool, not a hot path. For workspaces with O(10^5+)
		// actions a streaming or paginated repository iterator would be
		// preferable, but the existing ActionRepository surface returns
		// the full list and adding a paginated variant just for this
		// job is over-engineering until a real workspace hits that
		// scale. Revisit if the repair starts touching live operational
		// budgets.
		// Sweep archived actions too — an archived action with no Slack
		// post is still a candidate for repair (operators can unarchive
		// later and the message will be there).
		actions, err := uc.repo.Action().List(ctx, workspaceID, interfaces.ActionListOptions{IncludeArchived: true})
		if err != nil {
			// Listing should not fail for normal operation; if it does,
			// surface the whole-workspace error and move on so the rest
			// of the registry still gets repaired. errutil.Handle keeps
			// the error in the configured sink (Sentry / log).
			errutil.Handle(ctx, goerr.Wrap(err, "failed to list actions",
				goerr.V("workspace_id", workspaceID)),
				"diagnosis: list actions for fix-unsent-action sweep")
			continue
		}

		for _, action := range actions {
			if action.SlackMessageTS != "" {
				continue
			}
			report.Total++
			logger.Info("diagnosis: posting unsent action",
				slog.String("workspace_id", workspaceID),
				slog.Int64("action_id", action.ID),
				slog.Int64("case_id", action.CaseID),
			)

			_, postErr := uc.actionUC.PostSlackMessageToAction(ctx, workspaceID, action.ID)
			switch {
			case postErr == nil:
				report.Fixed++
			case errors.Is(postErr, usecase.ErrCaseHasNoSlackChannel),
				errors.Is(postErr, usecase.ErrSlackMessageAlreadyPosted),
				errors.Is(postErr, usecase.ErrActionNotFound),
				errors.Is(postErr, usecase.ErrCaseNotFound):
				// Documented skip conditions: parent case has no
				// channel, or the action / case was concurrently
				// changed between list and post. Not a failure.
				report.Skipped++
				logger.Info("diagnosis: skipped unsent action",
					slog.String("workspace_id", workspaceID),
					slog.Int64("action_id", action.ID),
					slog.String("reason", postErr.Error()),
				)
			default:
				report.Failed++
				errutil.Handle(ctx, postErr, "diagnosis: failed to post unsent action")
			}
		}
	}

	logger.Info("diagnosis: fix-unsent-action sweep completed",
		slog.Int("total", report.Total),
		slog.Int("fixed", report.Fixed),
		slog.Int("skipped", report.Skipped),
		slog.Int("failed", report.Failed),
	)
	return report, nil
}
