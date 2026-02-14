package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli"
)

func TestRun_ValidateCommand_ValidConfig(t *testing.T) {
	t.Setenv("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "")
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
	t.Setenv("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "")
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
	t.Setenv("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "")
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.toml")

	err := cli.Run(context.Background(), []string{"hecatoncheires", "validate", "--config", configPath}, "test")
	gt.Value(t, err).NotNil()
}

func TestRun_ValidateCommand_ConfigDirectory(t *testing.T) {
	t.Setenv("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "")
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
	t.Setenv("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "")
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
