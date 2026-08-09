package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// dbConsistencyChecker backs the POST /api/validate/db endpoint. It lives at the
// composition root because the operation spans two things wired here and nowhere
// else: parsing workspace configuration (pkg/cli/config) and the consistency
// check (pkg/usecase). The HTTP layer only translates the request into
// documents and the result into JSON.
type dbConsistencyChecker struct {
	uc *usecase.UseCases
}

func newDBConsistencyChecker(uc *usecase.UseCases) *dbConsistencyChecker {
	return &dbConsistencyChecker{uc: uc}
}

// CheckDBConsistency parses the submitted workspace configuration and checks the
// persisted data against it. Configuration failures come back wrapped with
// httpctrl.ErrInvalidConfigDocument so the handler answers 400 rather than 500.
func (c *dbConsistencyChecker) CheckDBConsistency(ctx context.Context, docs []httpctrl.ConfigDocument) (*usecase.ValidationResult, error) {
	if c == nil || c.uc == nil {
		return nil, goerr.New("db consistency checker is not configured")
	}

	sources := make([]config.WorkspaceConfigSource, 0, len(docs))
	for _, doc := range docs {
		// BaseDir stays empty on purpose: a submitted document has no directory,
		// and resolving its prompt_file against the server's filesystem would
		// turn this endpoint into an arbitrary file read. Prompt text plays no
		// part in the consistency check.
		sources = append(sources, config.WorkspaceConfigSource{
			Name: doc.Name,
			Data: doc.Data,
		})
	}

	workspaceConfigs, err := config.ParseWorkspaceConfigs(sources)
	if err != nil {
		return nil, goerr.Join(httpctrl.ErrInvalidConfigDocument, err)
	}

	result, err := c.uc.ValidateDBWithConfig(ctx, config.BuildWorkspaceRegistry(workspaceConfigs))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to check data against the submitted configuration",
			goerr.V("workspace_count", len(workspaceConfigs)))
	}
	return result, nil
}
