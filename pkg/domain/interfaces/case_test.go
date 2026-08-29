package interfaces_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestCaseArchiveScope_Allows(t *testing.T) {
	t.Run("active only excludes archived", func(t *testing.T) {
		scope := interfaces.CaseArchiveScopeActiveOnly
		gt.Bool(t, scope.Allows(false)).True()
		gt.Bool(t, scope.Allows(true)).False()
	})

	t.Run("archived only excludes active", func(t *testing.T) {
		scope := interfaces.CaseArchiveScopeArchivedOnly
		gt.Bool(t, scope.Allows(false)).False()
		gt.Bool(t, scope.Allows(true)).True()
	})

	t.Run("all allows both", func(t *testing.T) {
		scope := interfaces.CaseArchiveScopeAll
		gt.Bool(t, scope.Allows(false)).True()
		gt.Bool(t, scope.Allows(true)).True()
	})

	// The zero value has to be the safe slice: every caller that names no
	// scope (the board, the dashboard, the Job scanner, the agent and MCP
	// tools, the case_ref picker) relies on it to exclude archived cases.
	t.Run("zero value is active only", func(t *testing.T) {
		var scope interfaces.CaseArchiveScope
		gt.Value(t, scope).Equal(interfaces.CaseArchiveScopeActiveOnly)
		gt.Bool(t, scope.Allows(false)).True()
		gt.Bool(t, scope.Allows(true)).False()
	})
}

func TestBuildListCaseConfig_ArchiveScope(t *testing.T) {
	t.Run("no options yields the active-only scope and no status filter", func(t *testing.T) {
		cfg := interfaces.BuildListCaseConfig()
		gt.Value(t, cfg.Status()).Nil()
		gt.Value(t, cfg.ArchiveScope()).Equal(interfaces.CaseArchiveScopeActiveOnly)
	})

	t.Run("WithArchiveScope sets the scope", func(t *testing.T) {
		cfg := interfaces.BuildListCaseConfig(
			interfaces.WithArchiveScope(interfaces.CaseArchiveScopeArchivedOnly),
		)
		gt.Value(t, cfg.Status()).Nil()
		gt.Value(t, cfg.ArchiveScope()).Equal(interfaces.CaseArchiveScopeArchivedOnly)
	})

	t.Run("status and archive scope combine", func(t *testing.T) {
		cfg := interfaces.BuildListCaseConfig(
			interfaces.WithStatus(types.CaseStatusClosed),
			interfaces.WithArchiveScope(interfaces.CaseArchiveScopeAll),
		)
		gt.Value(t, cfg.Status()).NotNil().Required()
		gt.Value(t, *cfg.Status()).Equal(types.CaseStatusClosed)
		gt.Value(t, cfg.ArchiveScope()).Equal(interfaces.CaseArchiveScopeAll)
	})

	t.Run("the last archive scope wins", func(t *testing.T) {
		cfg := interfaces.BuildListCaseConfig(
			interfaces.WithArchiveScope(interfaces.CaseArchiveScopeAll),
			interfaces.WithArchiveScope(interfaces.CaseArchiveScopeArchivedOnly),
		)
		gt.Value(t, cfg.ArchiveScope()).Equal(interfaces.CaseArchiveScopeArchivedOnly)
	})
}
