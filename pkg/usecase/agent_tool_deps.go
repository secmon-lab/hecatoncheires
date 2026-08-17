package usecase

import (
	"context"

	slackgo "github.com/slack-go/slack" //nolint:depguard

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slackpost"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// AgentToolDeps assembles the tool dependencies the agent Kernel is built from.
//
// It exists as ONE function because the previous shape — a struct literal
// repeated at every entry point that builds a Kernel — is what produced a
// production bug: `SlackPoster` was declared on the struct and set by none of
// them, so `slack__post_to_case_channel` was advertised to every Job planner and
// built for nobody, and every run that reached for it died on "unknown tool".
// A field added here now reaches serve, the scheduled sweep and the eval harness
// together, and there is no literal left for a caller to forget it in.
//
// Every value is read off the UseCases, which already holds each client the
// options installed; nothing is passed in, so a caller cannot supply a client
// that differs from the one the rest of the application uses.
func (uc *UseCases) AgentToolDeps() agentkernel.ToolDeps {
	return agentkernel.ToolDeps{
		Repo:              uc.repo,
		Registry:          uc.workspaceRegistry,
		SlackBot:          uc.slackService,
		SlackPoster:       NewSlackPoster(uc.slackService),
		SlackSearch:       uc.slackSearch,
		SlackRetriever:    uc.slackRetriever,
		NotionClient:      uc.notionTool,
		GitHubClient:      uc.githubClient,
		WebFetchClient:    uc.webfetchClient,
		JiraTools:         uc.jiraTools,
		ActionUC:          NewActionToolAdapter(uc.Action),
		ActionStepUC:      NewActionStepToolAdapter(uc.ActionStep),
		CaseUC:            NewCaseToolAdapter(uc.Case),
		CaseRefUC:         uc.Case,
		CaseMultiUC:       NewCaseMultiCaseAdapter(uc.Case),
		CaseMultiActionUC: NewCaseMultiActionAdapter(uc.Action, uc.ActionStep),
		MemoUC:            NewMemoToolAdapter(uc.Memo),
		KnowledgeAccessor: NewKnowledgeToolAccessor(uc.Knowledge, uc.Tag),
		KnowledgeMutator:  NewKnowledgeToolMutator(uc.Knowledge, uc.Tag),
	}
}

// NewSlackPoster returns the narrow posting surface the channel-pinned Slack
// tool is built from, or a nil interface when Slack is not configured.
//
// Returning a nil INTERFACE rather than a wrapper around a nil service is
// load-bearing: both the tool-set resolver and slackpost.New decide whether
// slack__post_to_case_channel exists at all from Poster != nil, so a typed-nil
// wrapper would advertise a tool that posts nowhere.
//
// It is exported because the in-process Job executor builds its tools outside
// this package and must not grow a second copy of that nil rule.
func NewSlackPoster(svc slack.Service) slackpost.Poster {
	if svc == nil {
		return nil
	}
	return slackPoster{svc: svc}
}

// slackPoster narrows slack.Service to the two posting calls. The narrowing is
// the point: an LLM holding the post tool must not reach the wider Slack API,
// and slack.Service.PostThreadMessage is variadic where slackpost.Poster is not,
// so the service does not satisfy the interface on its own anyway.
type slackPoster struct{ svc slack.Service }

func (p slackPoster) PostMessage(ctx context.Context, channelID string, blocks []slackgo.Block, text string) (string, error) {
	return p.svc.PostMessage(ctx, channelID, blocks, text)
}

func (p slackPoster) PostThreadMessage(ctx context.Context, channelID, threadTS string, blocks []slackgo.Block, text string) (string, error) {
	return p.svc.PostThreadMessage(ctx, channelID, threadTS, blocks, text)
}
