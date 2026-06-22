package config

import (
	"log/slog"
	"os"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/adapter/policy"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/urfave/cli/v3"
)

// MCP holds the configuration for the MCP (Model Context Protocol) server
// endpoint and its Rego-based authorization. The MCP endpoint is only exposed
// when enabled, and only when at least one policy path is supplied — we never
// serve MCP without an authorization policy.
type MCP struct {
	enabled bool
	// policyPaths and envPassthrough are read from the cli.Command in
	// Configure, since urfave/cli/v3 StringSliceFlag does not support
	// Destination.
	policyPaths    []string
	envPassthrough []string
}

// Flags returns CLI flags for the MCP server and its policy source.
func (m *MCP) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "mcp",
			Usage:       "Enable the MCP (Model Context Protocol) endpoint at /mcp (requires --policy)",
			Value:       false,
			Sources:     cli.EnvVars("HECATONCHEIRES_MCP"),
			Destination: &m.enabled,
		},
		&cli.StringSliceFlag{
			Name:     "policy",
			Usage:    "Paths to Rego policy files or directories used to authorize MCP requests (data.auth.mcp). Can be specified multiple times.",
			Sources:  cli.EnvVars("HECATONCHEIRES_POLICY"),
			Category: "MCP",
		},
		&cli.StringSliceFlag{
			Name:     "mcp-env",
			Usage:    "Names of environment variables to expose to the Rego policy as input.env (allow-list). Can be specified multiple times.",
			Sources:  cli.EnvVars("HECATONCHEIRES_MCP_ENV"),
			Category: "MCP",
		},
	}
}

// IsEnabled reports whether the MCP endpoint should be wired.
func (m *MCP) IsEnabled() bool {
	return m.enabled
}

// Configure builds the PolicyClient and the env snapshot when MCP is enabled.
// It reads the slice flags from c (StringSliceFlag has no Destination).
//
// When MCP is disabled it returns (nil, nil, nil). When MCP is enabled without
// any policy path it returns an error: exposing the MCP endpoint without an
// authorization policy would be an unauthenticated data leak, so we refuse to
// start rather than fall back to an open endpoint.
func (m *MCP) Configure(c *cli.Command) (interfaces.PolicyClient, map[string]string, error) {
	if !m.enabled {
		return nil, nil, nil
	}

	m.policyPaths = c.StringSlice("policy")
	m.envPassthrough = c.StringSlice("mcp-env")

	if len(m.policyPaths) == 0 {
		return nil, nil, goerr.New("--mcp requires at least one --policy path; refusing to expose the MCP endpoint without an authorization policy")
	}

	pc, err := policy.New(m.policyPaths)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to build policy client for MCP")
	}

	return pc, m.envSnapshot(), nil
}

// envSnapshot reads the current values of the allow-listed environment
// variables. Names absent from the environment are omitted so the policy sees
// only the variables that are actually set.
func (m *MCP) envSnapshot() map[string]string {
	out := make(map[string]string, len(m.envPassthrough))
	for _, name := range m.envPassthrough {
		if v, ok := os.LookupEnv(name); ok {
			out[name] = v
		}
	}
	return out
}

// LogAttrs returns log attributes describing the MCP configuration. Only the
// env variable names are logged, never their values.
func (m *MCP) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Bool("enabled", m.enabled),
		slog.Int("policy_paths", len(m.policyPaths)),
		slog.Any("env_passthrough", m.envPassthrough),
	}
}
