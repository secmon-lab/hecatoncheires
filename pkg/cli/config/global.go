package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// GlobalConfig represents a deployment-wide configuration file supplied via
// --global-config. It is distinct from the per-workspace files under --config
// (which stay "1 file = 1 workspace"): a global config file carries settings
// that span workspaces. Today it holds workspace group definitions only; new
// deployment-wide sections can be added here later without a new flag.
type GlobalConfig struct {
	// Workspace captures a stray [workspace] section so the loader can reject
	// it. Workspace definitions belong under --config, never here. It is a raw
	// map (not the real WorkspaceBaseConfig) because its only use is presence
	// detection; an empty [workspace] table still unmarshals to a non-nil map.
	Workspace       map[string]any          `toml:"workspace"`
	WorkspaceGroups []WorkspaceGroupSection `toml:"workspace_group"`
}

// WorkspaceGroupSection represents a single [[workspace_group]] table.
type WorkspaceGroupSection struct {
	ID          string   `toml:"id"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Members     []string `toml:"members"`
}

// Validate checks one group section in isolation: id presence and format, and
// member uniqueness within this group. Cross-file id uniqueness and member
// existence are enforced by the loader / ConfigureGroups once the full group
// and workspace sets are known.
func (s *WorkspaceGroupSection) Validate() error {
	if s.ID == "" {
		return goerr.Wrap(ErrMissingWorkspaceGroupID, "[[workspace_group]] id is required")
	}
	if !workspaceIDPattern.MatchString(s.ID) || len(s.ID) > 63 {
		return goerr.Wrap(ErrInvalidWorkspaceGroupID,
			"workspace group ID must match ^[a-z0-9]+(-[a-z0-9]+)*$ and be at most 63 characters",
			goerr.V(WorkspaceGroupIDKey, s.ID))
	}
	seen := make(map[string]bool, len(s.Members))
	for _, m := range s.Members {
		if seen[m] {
			return goerr.Wrap(ErrDuplicateGroupMember, "duplicate workspace group member",
				goerr.V(WorkspaceGroupIDKey, s.ID),
				goerr.V(GroupMemberKey, m))
		}
		seen[m] = true
	}
	return nil
}

// collectTOMLFiles expands the given file/dir paths into a flat list of .toml
// file paths, walking directories recursively. Shared by LoadWorkspaceConfigs
// and LoadWorkspaceGroups so the two loaders discover files identically.
func collectTOMLFiles(paths []string) ([]string, error) {
	var tomlFiles []string
	// Deduplicate by absolute path so overlapping inputs (a file listed twice,
	// or a file also reachable through a listed directory) collect it once.
	// Without this, the same file yields a spurious duplicate-ID error at load.
	seen := make(map[string]bool)
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = filepath.Clean(path)
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		tomlFiles = append(tomlFiles, path)
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to stat config path", goerr.V(ConfigPathKey, p))
		}

		if info.IsDir() {
			err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(d.Name(), ".toml") {
					add(path)
				}
				return nil
			})
			if err != nil {
				return nil, goerr.Wrap(err, "failed to walk config directory", goerr.V(ConfigPathKey, p))
			}
		} else {
			add(p)
		}
	}
	return tomlFiles, nil
}

// LoadWorkspaceGroups walks the given file/dir paths, parses each .toml as a
// GlobalConfig, validates every [[workspace_group]] section, and rejects
// duplicate group IDs across files. It does not know the workspace set; member
// existence is checked by ConfigureGroups. Zero files (empty paths) yields an
// empty slice with no error — an unset --global-config is a valid state.
func LoadWorkspaceGroups(paths []string) ([]*model.WorkspaceGroup, error) {
	tomlFiles, err := collectTOMLFiles(paths)
	if err != nil {
		return nil, err
	}

	var groups []*model.WorkspaceGroup
	seenIDs := make(map[string]string) // group ID -> file path
	for _, f := range tomlFiles {
		// #nosec G304 - path is expected to be provided by CLI argument
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to read global config file", goerr.V(ConfigPathKey, f))
		}

		var gc GlobalConfig
		if err := toml.Unmarshal(data, &gc); err != nil {
			return nil, goerr.Wrap(err, "failed to parse global config TOML", goerr.V(ConfigPathKey, f))
		}

		// A [workspace] section in a global config file is almost certainly a
		// misplaced workspace definition. Reject it loudly rather than ignore it
		// silently (the docs promise this file never carries [workspace]).
		if gc.Workspace != nil {
			return nil, goerr.Wrap(ErrGlobalConfigContainsWorkspace,
				"global config file must not contain a [workspace] section",
				goerr.V(ConfigPathKey, f))
		}

		for i := range gc.WorkspaceGroups {
			section := &gc.WorkspaceGroups[i]
			if err := section.Validate(); err != nil {
				return nil, goerr.Wrap(err, "invalid [[workspace_group]]", goerr.V(ConfigPathKey, f))
			}

			if existing, ok := seenIDs[section.ID]; ok {
				return nil, goerr.Wrap(ErrDuplicateWorkspaceGroupID, "duplicate workspace group ID",
					goerr.V(WorkspaceGroupIDKey, section.ID),
					goerr.V("first_file", existing),
					goerr.V("second_file", f))
			}
			seenIDs[section.ID] = f

			name := section.Name
			if name == "" {
				name = section.ID
			}
			g := &model.WorkspaceGroup{
				ID:          section.ID,
				Name:        name,
				Description: section.Description,
				MemberIDs:   section.Members,
			}
			if err := g.Validate(); err != nil {
				return nil, goerr.Wrap(err, "invalid workspace group", goerr.V(ConfigPathKey, f))
			}
			groups = append(groups, g)
		}
	}

	return groups, nil
}

// ConfigureGroups reads the --global-config flag, loads workspace groups, and
// cross-checks every member against the workspace registry. It returns a
// never-nil registry: an unset flag yields an empty registry (feature
// dormant). It is a separate method from Configure so the callers that do not
// need groups (assist / diagnosis / job runtime) are untouched.
func (a *AppConfig) ConfigureGroups(c *cli.Command, ws *model.WorkspaceRegistry) (*model.WorkspaceGroupRegistry, error) {
	registry := model.NewWorkspaceGroupRegistry()

	paths := c.StringSlice("global-config")
	if len(paths) == 0 {
		return registry, nil
	}

	groups, err := LoadWorkspaceGroups(paths)
	if err != nil {
		return nil, err
	}

	for _, g := range groups {
		for _, memberID := range g.MemberIDs {
			if _, err := ws.Get(memberID); err != nil {
				return nil, goerr.Wrap(ErrUnknownGroupMember,
					"workspace group member references an unknown workspace",
					goerr.V(WorkspaceGroupIDKey, g.ID),
					goerr.V(GroupMemberKey, memberID))
			}
		}
		registry.Register(g)
		logging.Default().Info("Registered workspace group",
			"id", g.ID, "name", g.Name, "member_count", len(g.MemberIDs))
	}

	return registry, nil
}
