package config_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
)

// parseAuthz parses a single workspace document whose [authz] section is the
// given TOML body, resolving relative paths against baseDir.
func parseAuthz(t *testing.T, authzBody, baseDir string) (*config.WorkspaceConfig, error) {
	t.Helper()
	doc := fmt.Sprintf(`
[workspace]
id = "security"
name = "Security"

%s
`, authzBody)
	configs, err := config.ParseWorkspaceConfigs([]config.WorkspaceConfigSource{
		{Name: "security.toml", Data: []byte(doc), BaseDir: baseDir},
	})
	if err != nil {
		return nil, err
	}
	return configs[0], nil
}

func evalAllow(t *testing.T, wc *config.WorkspaceConfig, user authz.WorkspaceUser) bool {
	t.Helper()
	var res authz.WorkspaceResult
	gt.NoError(t, wc.AuthzPolicy.Query(context.Background(), authz.WorkspaceQuery, authz.WorkspaceInput{
		Workspace: authz.WorkspaceRef{ID: wc.ID, Name: wc.Name},
		User:      user,
	}, &res))
	return res.Allow
}

func TestAuthz_RelativeFileIsCompiled(t *testing.T) {
	wc, err := parseAuthz(t, `
[authz]
policy = ["authz/allow_member.rego"]
`, "testdata")
	gt.NoError(t, err)
	gt.Value(t, wc.AuthzPolicy).NotNil()

	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0ALLOWED"})).Equal(true)
	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0OTHER"})).Equal(false)
}

func TestAuthz_DirectoryUsesEveryFile(t *testing.T) {
	wc, err := parseAuthz(t, `
[authz]
policy = ["authz/split"]
`, "testdata")
	gt.NoError(t, err)

	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0FIRST"})).Equal(true)
	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0X", Email: "second@example.com"})).Equal(true)
	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0X", Email: "third@example.com"})).Equal(false)
}

func TestAuthz_MissingPathFails(t *testing.T) {
	_, err := parseAuthz(t, `
[authz]
policy = ["authz/does-not-exist.rego"]
`, "testdata")
	gt.Error(t, err)
}

func TestAuthz_SyntaxErrorFails(t *testing.T) {
	_, err := parseAuthz(t, `
[authz]
policy = ["authz/syntax_error.rego"]
`, "testdata")
	gt.Error(t, err)
}

func TestAuthz_TrialEvaluationFailures(t *testing.T) {
	for _, file := range []string{
		"authz/other_package.rego",     // no package authz
		"authz/non_bool.rego",          // allow is not a boolean
		"authz/conflict_unsynced.rego", // conflicting values for the unsynced input only
	} {
		t.Run(file, func(t *testing.T) {
			_, err := parseAuthz(t, fmt.Sprintf(`
[authz]
policy = [%q]
`, file), "testdata")
			gt.Error(t, err).Is(config.ErrAuthzPolicyTrialFailed)
		})
	}
}

// conflict_unsynced.rego evaluates cleanly for a user with a synced record and
// breaks only for one without, so the failure being attributed to the
// "unsynced_user" trial proves the synced trial passed and both were run.
func TestAuthz_TrialEvaluatesBothShapes(t *testing.T) {
	_, err := parseAuthz(t, `
[authz]
policy = ["authz/conflict_unsynced.rego"]
`, "testdata")
	gt.Error(t, err).Is(config.ErrAuthzPolicyTrialFailed)
	gt.Value(t, goerr.Values(err)["trial"]).Equal(any("unsynced_user"))
}

func TestAuthz_AllowNeverHoldingStillLoads(t *testing.T) {
	wc, err := parseAuthz(t, `
[authz]
policy = ["authz/never.rego"]
`, "testdata")
	gt.NoError(t, err)
	gt.Value(t, evalAllow(t, wc, authz.WorkspaceUser{ID: "U0ANY"})).Equal(false)
}

func TestAuthz_EmptyPolicyList(t *testing.T) {
	for _, body := range []string{
		"[authz]\npolicy = []",
		"[authz]\npolicy = [\"\"]",
	} {
		_, err := parseAuthz(t, body, "testdata")
		gt.Error(t, err).Is(config.ErrAuthzPolicyEmpty)
	}
}

func TestAuthz_SectionAbsent(t *testing.T) {
	wc, err := parseAuthz(t, "", "testdata")
	gt.NoError(t, err)
	gt.Value(t, wc.AuthzPolicy).Nil()
}

// A document submitted over HTTP carries no BaseDir; its paths must not be
// read, so even a path that does not exist parses cleanly.
func TestAuthz_NoBaseDirReadsNoFile(t *testing.T) {
	wc, err := parseAuthz(t, `
[authz]
policy = ["/definitely/not/here.rego"]
`, "")
	gt.NoError(t, err)
	gt.Value(t, wc.AuthzPolicy).Nil()

	_, err = parseAuthz(t, "[authz]\npolicy = []", "")
	gt.Error(t, err).Is(config.ErrAuthzPolicyEmpty)
}
