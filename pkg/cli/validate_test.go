package cli_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
)

func TestRun_ValidateCommand_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "test-ws"
name = "Test Workspace"

[[fields]]
id = "priority"
name = "Priority"
type = "select"
required = true

  [[fields.options]]
  id = "high"
  name = "High"

  [[fields.options]]
  id = "low"
  name = "Low"

[[fields]]
id = "description"
name = "Description"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0o600)
	gt.NoError(t, err).Required()

	// Run validate command with only config (no DB check)
	err = cli.Run(context.Background(), []string{"hecatoncheires", "validate", "--config", configPath}, "test")
	gt.NoError(t, err)
}

func TestRun_ValidateCommand_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Invalid: field with bad ID format
	content := `
[workspace]
id = "test-ws"
name = "Test Workspace"

[[fields]]
id = "INVALID_ID"
name = "Bad Field"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0o600)
	gt.NoError(t, err).Required()

	err = cli.Run(context.Background(), []string{"hecatoncheires", "validate", "--config", configPath}, "test")
	gt.Value(t, err).NotNil()
}

func TestRun_ValidateCommand_MissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.toml")

	err := cli.Run(context.Background(), []string{"hecatoncheires", "validate", "--config", configPath}, "test")
	gt.Value(t, err).NotNil()
}

func TestRun_ValidateCommand_DBCheckWithMemory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "test-ws"
name = "Test Workspace"

[[fields]]
id = "status"
name = "Status"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0o600)
	gt.NoError(t, err).Required()

	// Run validate with --check-db and memory backend (empty DB, should pass)
	err = cli.Run(context.Background(), []string{
		"hecatoncheires", "validate",
		"--config", configPath,
		"--check-db",
		"--repository-backend", "memory",
	}, "test")
	gt.NoError(t, err)
}

// TestRun_ValidateCommand_DBCheckReportsInconsistency drives the real operational
// path: seed a Case whose select value is no longer in the config, then run the
// command exactly as an operator would. The memory backend cannot cover this —
// `Configure` hands out a fresh empty repository per run, so there is no way to
// plant inconsistent data before the check runs.
func TestRun_ValidateCommand_DBCheckReportsInconsistency(t *testing.T) {
	projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
	databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
	if databaseID == "" {
		databaseID = "(default)"
	}
	if projectID == "" {
		projectID = "test-project"
		if _, ok := os.LookupEnv("FIRESTORE_EMULATOR_HOST"); !ok {
			t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:28615")
		}
	}

	ctx := context.Background()
	wsID := fmt.Sprintf("cli-validate-%d", time.Now().UnixNano())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := fmt.Sprintf(`
[workspace]
id = %q
name = "CLI Validate Workspace"

[[fields]]
id = "severity"
name = "Severity"
type = "select"

  [[fields.options]]
  id = "high"
  name = "High"

  [[fields.options]]
  id = "low"
  name = "Low"
`, wsID)
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0o600)).Required()

	seed, err := firestore.New(ctx, projectID, databaseID)
	gt.NoError(t, err).Required()
	_, err = seed.Case().Create(ctx, wsID, &model.Case{
		ReporterID: "U-CLI",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
		Title:      "Value dropped from config",
		Status:     types.CaseStatusOpen,
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "retired-option"},
		},
	})
	gt.NoError(t, err).Required()
	gt.NoError(t, seed.Close()).Required()

	args := []string{
		"hecatoncheires", "validate",
		"--config", configPath,
		"--check-db",
		"--repository-backend", "firestore",
		"--firestore-project-id", projectID,
		"--firestore-database-id", databaseID,
	}

	err = cli.Run(ctx, args, "test")
	gt.Value(t, err).NotNil().Required()

	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	values := ge.Values()
	gt.Value(t, values["issue_groups"]).Equal(1)
	gt.Value(t, values["total_occurrences"]).Equal(int64(1))
}

func TestRun_ValidateCommand_ConfigDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple config files in a directory
	config1 := `
[workspace]
id = "ws-one"
name = "Workspace One"

[[fields]]
id = "priority"
name = "Priority"
type = "text"
`
	config2 := `
[workspace]
id = "ws-two"
name = "Workspace Two"

[[fields]]
id = "status"
name = "Status"
type = "text"
`
	err := os.WriteFile(filepath.Join(tmpDir, "ws1.toml"), []byte(config1), 0o600)
	gt.NoError(t, err).Required()

	err = os.WriteFile(filepath.Join(tmpDir, "ws2.toml"), []byte(config2), 0o600)
	gt.NoError(t, err).Required()

	// Point config to directory
	err = cli.Run(context.Background(), []string{
		"hecatoncheires", "validate",
		"--config", tmpDir,
	}, "test")
	gt.NoError(t, err)
}

func TestRun_ValidateCommand_DuplicateWorkspaceID(t *testing.T) {
	tmpDir := t.TempDir()

	config1 := `
[workspace]
id = "duplicate-ws"
name = "Workspace One"

[[fields]]
id = "priority"
name = "Priority"
type = "text"
`
	config2 := `
[workspace]
id = "duplicate-ws"
name = "Workspace Two"

[[fields]]
id = "status"
name = "Status"
type = "text"
`
	err := os.WriteFile(filepath.Join(tmpDir, "ws1.toml"), []byte(config1), 0o600)
	gt.NoError(t, err).Required()

	err = os.WriteFile(filepath.Join(tmpDir, "ws2.toml"), []byte(config2), 0o600)
	gt.NoError(t, err).Required()

	err = cli.Run(context.Background(), []string{
		"hecatoncheires", "validate",
		"--config", tmpDir,
	}, "test")
	gt.Value(t, err).NotNil()
}
