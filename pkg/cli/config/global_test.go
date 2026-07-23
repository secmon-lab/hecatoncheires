package config_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/urfave/cli/v3"
)

// writeGlobalConfig writes content to a temp .toml file and returns its path.
func writeGlobalConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	gt.NoError(t, os.WriteFile(path, []byte(content), 0600)).Required()
	return path
}

// wsRegistry builds a workspace registry populated with the given IDs.
func wsRegistry(ids ...string) *model.WorkspaceRegistry {
	reg := model.NewWorkspaceRegistry()
	for _, id := range ids {
		reg.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: id, Name: id}})
	}
	return reg
}

// runConfigureGroups exercises the real --global-config flag path.
func runConfigureGroups(t *testing.T, ws *model.WorkspaceRegistry, paths ...string) (*model.WorkspaceGroupRegistry, error) {
	t.Helper()
	var appCfg config.AppConfig
	var result *model.WorkspaceGroupRegistry
	var resErr error
	cmd := &cli.Command{
		Flags: appCfg.Flags(),
		Action: func(_ context.Context, c *cli.Command) error {
			result, resErr = appCfg.ConfigureGroups(c, ws)
			return nil
		},
	}
	args := []string{"test"}
	for _, p := range paths {
		args = append(args, "--global-config", p)
	}
	gt.NoError(t, cmd.Run(context.Background(), args)).Required()
	return result, resErr
}

func TestLoadWorkspaceGroups_SingleFileMultipleGroups(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "security"
name = "Security"
description = "Security workspaces"
members = ["risk", "incident"]

[[workspace_group]]
id = "corp"
members = ["legal"]
`)

	groups, err := config.LoadWorkspaceGroups([]string{path})
	gt.NoError(t, err).Required()
	gt.Array(t, groups).Length(2).Required()

	gt.Value(t, groups[0].ID).Equal("security")
	gt.Value(t, groups[0].Name).Equal("Security")
	gt.Value(t, groups[0].Description).Equal("Security workspaces")
	gt.Array(t, groups[0].MemberIDs).Length(2)
	gt.Value(t, groups[0].MemberIDs[0]).Equal("risk")
	gt.Value(t, groups[0].MemberIDs[1]).Equal("incident")

	// name defaults to id when omitted.
	gt.Value(t, groups[1].ID).Equal("corp")
	gt.Value(t, groups[1].Name).Equal("corp")
	gt.Array(t, groups[1].MemberIDs).Length(1)
	gt.Value(t, groups[1].MemberIDs[0]).Equal("legal")
}

func TestLoadWorkspaceGroups_EmptyMembersAllowed(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "wip"
name = "Work in progress"
`)

	groups, err := config.LoadWorkspaceGroups([]string{path})
	gt.NoError(t, err).Required()
	gt.Array(t, groups).Length(1).Required()
	gt.Value(t, groups[0].ID).Equal("wip")
	gt.Array(t, groups[0].MemberIDs).Length(0)
}

func TestLoadWorkspaceGroups_Directory(t *testing.T) {
	dir := t.TempDir()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "a.toml"), []byte(`
[[workspace_group]]
id = "security"
members = ["risk"]
`), 0600)).Required()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "b.toml"), []byte(`
[[workspace_group]]
id = "corp"
members = ["legal"]
`), 0600)).Required()

	groups, err := config.LoadWorkspaceGroups([]string{dir})
	gt.NoError(t, err).Required()
	gt.Array(t, groups).Length(2)

	ids := map[string]bool{}
	for _, g := range groups {
		ids[g.ID] = true
	}
	gt.Bool(t, ids["security"]).True()
	gt.Bool(t, ids["corp"]).True()
}

func TestLoadWorkspaceGroups_DuplicateGroupIDAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "a.toml"), []byte(`
[[workspace_group]]
id = "security"
members = ["risk"]
`), 0600)).Required()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "b.toml"), []byte(`
[[workspace_group]]
id = "security"
members = ["legal"]
`), 0600)).Required()

	_, err := config.LoadWorkspaceGroups([]string{dir})
	gt.Error(t, err).Is(config.ErrDuplicateWorkspaceGroupID)
}

func TestLoadWorkspaceGroups_MissingID(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
name = "No ID"
members = ["risk"]
`)
	_, err := config.LoadWorkspaceGroups([]string{path})
	gt.Error(t, err).Is(config.ErrMissingWorkspaceGroupID)
}

func TestLoadWorkspaceGroups_InvalidID(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "Bad_ID"
members = ["risk"]
`)
	_, err := config.LoadWorkspaceGroups([]string{path})
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceGroupID)
}

func TestLoadWorkspaceGroups_DuplicateMemberWithinGroup(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "security"
members = ["risk", "risk"]
`)
	_, err := config.LoadWorkspaceGroups([]string{path})
	gt.Error(t, err).Is(config.ErrDuplicateGroupMember)
}

func TestLoadWorkspaceGroups_NoPaths(t *testing.T) {
	groups, err := config.LoadWorkspaceGroups(nil)
	gt.NoError(t, err)
	gt.Array(t, groups).Length(0)
}

func TestLoadWorkspaceGroups_RejectsWorkspaceSection(t *testing.T) {
	// A misplaced workspace definition in a global config file must be rejected,
	// not silently ignored.
	path := writeGlobalConfig(t, "global.toml", `
[workspace]
id = "risk"

[[workspace_group]]
id = "security"
members = ["risk"]
`)
	_, err := config.LoadWorkspaceGroups([]string{path})
	gt.Error(t, err).Is(config.ErrGlobalConfigContainsWorkspace)
}

func TestLoadWorkspaceGroups_DeduplicatesOverlappingPaths(t *testing.T) {
	// The same file reachable via both a direct path and its directory must be
	// collected once, not twice (which would look like a duplicate group ID).
	dir := t.TempDir()
	path := filepath.Join(dir, "global.toml")
	gt.NoError(t, os.WriteFile(path, []byte(`
[[workspace_group]]
id = "security"
members = ["risk"]
`), 0600)).Required()

	groups, err := config.LoadWorkspaceGroups([]string{path, dir, path})
	gt.NoError(t, err).Required()
	gt.Array(t, groups).Length(1).Required()
	gt.Value(t, groups[0].ID).Equal("security")
}

func TestConfigureGroups_Dormant(t *testing.T) {
	// No --global-config flag: registry is empty, not nil.
	reg, err := runConfigureGroups(t, wsRegistry("risk"))
	gt.NoError(t, err).Required()
	gt.Value(t, reg).NotNil()
	gt.Array(t, reg.List()).Length(0)
}

func TestConfigureGroups_Valid(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "security"
name = "Security"
members = ["risk", "incident"]
`)
	reg, err := runConfigureGroups(t, wsRegistry("risk", "incident", "legal"), path)
	gt.NoError(t, err).Required()

	groups := reg.List()
	gt.Array(t, groups).Length(1).Required()
	gt.Value(t, groups[0].ID).Equal("security")
	gt.Value(t, groups[0].Name).Equal("Security")
	gt.Array(t, groups[0].MemberIDs).Length(2)
	gt.Value(t, groups[0].MemberIDs[0]).Equal("risk")
	gt.Value(t, groups[0].MemberIDs[1]).Equal("incident")
}

func TestConfigureGroups_UnknownMember(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "security"
members = ["risk", "ghost"]
`)
	// "ghost" is not a registered workspace.
	_, err := runConfigureGroups(t, wsRegistry("risk"), path)
	gt.Error(t, err).Is(config.ErrUnknownGroupMember)
}

func TestConfigureGroups_MultiMembership(t *testing.T) {
	path := writeGlobalConfig(t, "global.toml", `
[[workspace_group]]
id = "security"
members = ["risk", "incident"]

[[workspace_group]]
id = "audit"
members = ["risk", "legal"]
`)
	reg, err := runConfigureGroups(t, wsRegistry("risk", "incident", "legal"), path)
	gt.NoError(t, err).Required()

	sec, err := reg.Get("security")
	gt.NoError(t, err).Required()
	audit, err := reg.Get("audit")
	gt.NoError(t, err).Required()

	// "risk" belongs to both groups.
	gt.Bool(t, slices.Contains(sec.MemberIDs, "risk")).True()
	gt.Bool(t, slices.Contains(audit.MemberIDs, "risk")).True()
	gt.Bool(t, slices.Contains(audit.MemberIDs, "legal")).True()
}

const exportConfigTOML = `
[export]
include_private = true

[export.bigquery]
project = "my-project"
location = "asia-northeast1"

[[export.bigquery.workspace]]
id = "risk"
dataset = "hecato_risk"

[[export.bigquery.workspace]]
id = "incident"
dataset = "hecato_incident"
include_private = false
`

func TestLoadExportConfig_Basic(t *testing.T) {
	path := writeGlobalConfig(t, "export.toml", exportConfigTOML)

	section, err := config.LoadExportConfig([]string{path})
	gt.NoError(t, err).Required()
	gt.Value(t, section).NotNil().Required()
	gt.Bool(t, section.IncludePrivate).True()
	gt.Value(t, section.BigQuery).NotNil().Required()
	gt.Value(t, section.BigQuery.Project).Equal("my-project")
	gt.Value(t, section.BigQuery.Location).Equal("asia-northeast1")
	gt.Array(t, section.BigQuery.Workspaces).Length(2)
	gt.Value(t, section.BigQuery.Workspaces[0].ID).Equal("risk")
	gt.Value(t, section.BigQuery.Workspaces[0].Dataset).Equal("hecato_risk")

	// Per-workspace resolution: "risk" inherits the section default (true);
	// "incident" overrides to false.
	gt.Bool(t, section.IncludePrivateFor(section.BigQuery.Workspaces[0])).True()
	gt.Bool(t, section.IncludePrivateFor(section.BigQuery.Workspaces[1])).False()
}

func TestExportSection_IncludePrivateFor_DefaultsToExcluded(t *testing.T) {
	// With no include_private set anywhere, the effective value is false — private
	// data is NOT exported by default.
	s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{
		Project:    "p",
		Workspaces: []config.ExportWorkspaceMapping{{ID: "risk", Dataset: "ds"}},
	}}
	gt.Bool(t, s.IncludePrivateFor(s.BigQuery.Workspaces[0])).False()
}

func TestLoadExportConfig_None(t *testing.T) {
	path := writeGlobalConfig(t, "noexport.toml", "[[workspace_group]]\nid = \"g\"\n")
	section, err := config.LoadExportConfig([]string{path})
	gt.NoError(t, err).Required()
	gt.Value(t, section).Nil()
}

func TestLoadExportConfig_DuplicateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "a.toml"), []byte(exportConfigTOML), 0600)).Required()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "b.toml"), []byte(exportConfigTOML), 0600)).Required()

	_, err := config.LoadExportConfig([]string{dir})
	gt.Error(t, err).Is(config.ErrDuplicateExportConfig)
}

func TestExportSection_Validate(t *testing.T) {
	reg := wsRegistry("risk", "incident")

	t.Run("valid", func(t *testing.T) {
		section, err := config.LoadExportConfig([]string{writeGlobalConfig(t, "e.toml", exportConfigTOML)})
		gt.NoError(t, err).Required()
		gt.NoError(t, section.Validate(reg))
	})

	t.Run("missing project", func(t *testing.T) {
		s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{}}
		gt.Error(t, s.Validate(reg)).Is(config.ErrInvalidExportConfig)
	})

	t.Run("missing bigquery", func(t *testing.T) {
		s := &config.ExportSection{}
		gt.Error(t, s.Validate(reg)).Is(config.ErrInvalidExportConfig)
	})

	t.Run("unknown workspace", func(t *testing.T) {
		s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{
			Project:    "p",
			Workspaces: []config.ExportWorkspaceMapping{{ID: "nope", Dataset: "ds"}},
		}}
		gt.Error(t, s.Validate(reg)).Is(config.ErrUnknownExportWorkspace)
	})

	t.Run("invalid dataset name (hyphen)", func(t *testing.T) {
		s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{
			Project:    "p",
			Workspaces: []config.ExportWorkspaceMapping{{ID: "risk", Dataset: "bad-name"}},
		}}
		gt.Error(t, s.Validate(reg)).Is(config.ErrInvalidExportDataset)
	})

	t.Run("duplicate workspace id", func(t *testing.T) {
		s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{
			Project: "p",
			Workspaces: []config.ExportWorkspaceMapping{
				{ID: "risk", Dataset: "ds1"},
				{ID: "risk", Dataset: "ds2"},
			},
		}}
		gt.Error(t, s.Validate(reg)).Is(config.ErrDuplicateExportWorkspace)
	})

	t.Run("duplicate dataset", func(t *testing.T) {
		s := &config.ExportSection{BigQuery: &config.ExportBigQuerySection{
			Project: "p",
			Workspaces: []config.ExportWorkspaceMapping{
				{ID: "risk", Dataset: "same"},
				{ID: "incident", Dataset: "same"},
			},
		}}
		gt.Error(t, s.Validate(reg)).Is(config.ErrDuplicateExportWorkspace)
	})
}
