// Package policy adapts the opaq Rego engine to the interfaces.PolicyClient
// port. Policies are compiled once at construction from local .rego files
// (recursively discovered under the given paths) and the resulting client is
// safe for concurrent Query calls, so a single instance is shared across all
// requests.
package policy

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/opaq"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// client wraps an opaq.Client behind the PolicyClient port.
type client struct {
	opaq *opaq.Client
}

// New compiles the Rego policy files found under filePaths (each path may be a
// file or a directory; directories are walked recursively for .rego files)
// and returns a PolicyClient. Compilation happens here so a malformed policy
// fails loudly at startup rather than on the first request. At least one path
// must be supplied.
func New(filePaths []string) (interfaces.PolicyClient, error) {
	if len(filePaths) == 0 {
		return nil, goerr.New("at least one policy path is required")
	}

	c, err := opaq.New(
		opaq.Files(filePaths...),
		opaq.WithLogger(logging.Default()),
	)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to compile Rego policy", goerr.V("paths", filePaths))
	}

	return &client{opaq: c}, nil
}

// Query evaluates the Rego query against input and decodes the result into
// out. The opaq client logs the input at debug level through the project
// logger, so the Authorization header and the env allow-list are redacted by
// the logger's masq configuration. We therefore do not attach input to the
// error context here.
func (c *client) Query(ctx context.Context, query string, input, out any) error {
	if err := c.opaq.Query(ctx, query, input, out); err != nil {
		return goerr.Wrap(err, "failed to evaluate Rego query", goerr.V("query", query))
	}
	return nil
}
