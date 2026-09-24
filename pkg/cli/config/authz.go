package config

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/adapter/policy"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
)

// AuthzSection is the [authz] section of a workspace config.
type AuthzSection struct {
	// Policy lists Rego files or directories (walked recursively), relative to
	// the config file's directory. Required and non-empty when the section is present.
	Policy []string `toml:"policy"`
}

// authzTrialInput is one input compile evaluates a freshly compiled policy
// against, named so a failure says which shape broke the policy.
type authzTrialInput struct {
	name  string
	input authz.WorkspaceInput
}

// authzTrialInputs returns the inputs compile evaluates: one for a user with a
// synced record and one for a user without (every field but ID empty). They
// are the two shapes a runtime decision can produce, and are used for nothing
// but the trial evaluation.
func authzTrialInputs(ws authz.WorkspaceRef) []authzTrialInput {
	return []authzTrialInput{
		{
			name: "synced_user",
			input: authz.WorkspaceInput{
				Workspace: ws,
				User: authz.WorkspaceUser{
					ID:          "U00000000",
					Email:       "trial-user@example.com",
					Name:        "trial-user",
					DisplayName: "Trial User",
				},
			},
		},
		{
			name: "unsynced_user",
			input: authz.WorkspaceInput{
				Workspace: ws,
				User:      authz.WorkspaceUser{ID: "U00000000"},
			},
		},
	}
}

// compile resolves Policy against baseDir, compiles it, and trial-evaluates it
// with the inputs a runtime decision can produce for ws. baseDir == "" selects
// structural-validation mode: the paths are checked for emptiness only and no
// file is read, so it returns (nil, nil).
func (s *AuthzSection) compile(baseDir string, ws authz.WorkspaceRef) (interfaces.PolicyClient, error) {
	if s == nil {
		return nil, nil
	}
	if len(s.Policy) == 0 {
		return nil, goerr.Wrap(ErrAuthzPolicyEmpty, "[authz] policy lists no path")
	}
	for i, p := range s.Policy {
		if p == "" {
			return nil, goerr.Wrap(ErrAuthzPolicyEmpty, "[authz] policy has an empty path", goerr.V("index", i))
		}
	}
	// A document submitted over HTTP has no directory of its own; reading its
	// paths against the server's filesystem would turn config submission into
	// an arbitrary file read (see WorkspaceConfigSource.BaseDir).
	if baseDir == "" {
		return nil, nil
	}

	paths := make([]string, len(s.Policy))
	for i, p := range s.Policy {
		if filepath.IsAbs(p) {
			paths[i] = p
		} else {
			paths[i] = filepath.Join(baseDir, p)
		}
	}
	pc, err := policy.New(paths)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to compile workspace authz policy", goerr.V("policy", s.Policy))
	}

	for _, t := range authzTrialInputs(ws) {
		var res authz.WorkspaceResult
		if err := pc.Query(context.Background(), authz.WorkspaceQuery, t.input, &res); err != nil {
			return nil, goerr.Wrap(errors.Join(ErrAuthzPolicyTrialFailed, err),
				"workspace authz policy failed a trial evaluation",
				goerr.V("policy", s.Policy), goerr.V("trial", t.name))
		}
	}
	return pc, nil
}
