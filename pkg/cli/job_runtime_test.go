package cli_test

import (
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/cli"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

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
