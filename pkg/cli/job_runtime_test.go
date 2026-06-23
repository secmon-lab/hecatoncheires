package cli_test

import (
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/cli"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// The fakes below stub their interfaces by embedding them (nil), so any method
// call would panic. buildJobTools only constructs the tool structs, so the
// methods are never invoked — a non-nil fake is enough to prove the tool is
// wired in.
type fakeJobBot struct{ slacksvc.Service }
type fakeJobSearch struct{ slacktool.SearchService }
type fakeJobRetriever struct{ slacktool.MessageRetriever }
type fakeJobNotion struct{ notiontool.Client }

func toolNames(tools []gollem.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Spec().Name] = true
	}
	return names
}

func hasActionTool(tools []gollem.Tool) bool {
	for _, t := range tools {
		// Both the read-only (core__list_actions / core__get_action) and writer
		// (core__create_action, ...) action tools share the core__ prefix.
		if strings.HasPrefix(t.Spec().Name, "core__") {
			return true
		}
	}
	return false
}

func TestBuildJobTools_ChannelModeHasActionTools(t *testing.T) {
	ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}}
	c := &model.Case{ID: 1, SlackChannelID: "C1"}

	tools := cli.BuildJobToolsForTest(c, ws)

	gt.Bool(t, hasActionTool(tools)).True()
	names := toolNames(tools)
	gt.Bool(t, names["core__list_actions"]).True()
	gt.Bool(t, names["core__create_action"]).True()
	// Case-editing tools are present in both modes.
	gt.Bool(t, len(names) > 0).True()
	gt.Bool(t, hasCaseTool(tools)).True()
}

func TestBuildJobTools_ThreadModeOmitsActionTools(t *testing.T) {
	ws := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		CaseMode:  model.CaseModeThread,
	}
	// Thread-mode case binds a thread, not a dedicated channel.
	c := &model.Case{ID: 1, SlackChannelID: "C-MONITOR", SlackThreadTS: "1700000000.000100"}

	tools := cli.BuildJobToolsForTest(c, ws)

	// No action tools at all — thread-mode workspaces manage no Actions.
	gt.Bool(t, hasActionTool(tools)).False()
	// Case-editing tools (incl. board status) remain available.
	gt.Bool(t, hasCaseTool(tools)).True()
}

func hasCaseTool(tools []gollem.Tool) bool {
	for _, t := range tools {
		if strings.HasPrefix(t.Spec().Name, "case__") {
			return true
		}
	}
	return false
}

// readToolNames are the read-only Slack / Notion tools the Job agent uses to
// read its case thread and corroborate findings.
var readToolNames = []string{
	"slack__get_messages",
	"slack__search_messages",
	"notion__search",
	"notion__get_page",
}

func TestBuildJobTools_ReadToolsWiredInBothModes(t *testing.T) {
	deps := cli.JobReadToolDepsForTest{
		Bot:       &fakeJobBot{},
		Search:    &fakeJobSearch{},
		Retriever: &fakeJobRetriever{},
		Notion:    &fakeJobNotion{},
	}

	cases := []struct {
		name string
		ws   *model.WorkspaceEntry
		c    *model.Case
	}{
		{
			name: "channel-mode",
			ws:   &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}},
			c:    &model.Case{ID: 1, SlackChannelID: "C1"},
		},
		{
			name: "thread-mode",
			ws:   &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, CaseMode: model.CaseModeThread},
			c:    &model.Case{ID: 1, SlackChannelID: "C-MONITOR", SlackThreadTS: "1700000000.000100"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := toolNames(cli.BuildJobToolsWithReadDepsForTest(deps, tc.c, tc.ws))
			for _, n := range readToolNames {
				gt.Bool(t, names[n]).True()
			}
			// The post tool (separate slackpost package) stays wired alongside
			// the read tools; NewReadOnly must not have displaced it.
			gt.Bool(t, names["slack__post_to_case_channel"]).True()
			// Read tools are not Action tools, so thread-mode still forgoes
			// the core__ action tools.
			if tc.ws.IsThreadMode() {
				gt.Bool(t, hasActionTool(cli.BuildJobToolsWithReadDepsForTest(deps, tc.c, tc.ws))).False()
			}
		})
	}
}

func TestBuildJobTools_ReadToolsOmittedWhenDepsNil(t *testing.T) {
	// Zero deps: every read dependency is nil, so none of the read tools bind.
	ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}}
	c := &model.Case{ID: 1, SlackChannelID: "C1"}

	names := toolNames(cli.BuildJobToolsWithReadDepsForTest(cli.JobReadToolDepsForTest{}, c, ws))
	for _, n := range readToolNames {
		gt.Bool(t, names[n]).False()
	}
	// With a nil Bot, the post tool is also absent (it shares the Bot dep).
	gt.Bool(t, names["slack__post_to_case_channel"]).False()
}

func TestBuildJobTools_GetMessagesNeedsBotButSearchIsIndependent(t *testing.T) {
	ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}}
	c := &model.Case{ID: 1, SlackChannelID: "C1"}

	// Search wired but Bot nil: search_messages binds, get_messages does not.
	names := toolNames(cli.BuildJobToolsWithReadDepsForTest(
		cli.JobReadToolDepsForTest{Search: &fakeJobSearch{}}, c, ws))
	gt.Bool(t, names["slack__search_messages"]).True()
	gt.Bool(t, names["slack__get_messages"]).False()

	// Bot wired but Search nil: get_messages binds, search_messages does not.
	names = toolNames(cli.BuildJobToolsWithReadDepsForTest(
		cli.JobReadToolDepsForTest{Bot: &fakeJobBot{}}, c, ws))
	gt.Bool(t, names["slack__get_messages"]).True()
	gt.Bool(t, names["slack__search_messages"]).False()
}
