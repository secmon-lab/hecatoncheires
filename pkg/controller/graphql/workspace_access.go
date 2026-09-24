package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// workspaceIDArg is the argument name every workspace-scoped root field uses.
const workspaceIDArg = "workspaceId"

// WorkspaceAccessMiddleware rejects a Query / Mutation root field whose
// workspaceId argument names a workspace the caller may not access.
//
// It sits at the field level rather than inside each resolver so a root field
// added later is covered without anyone remembering to call a check; gqlgen
// has already parsed the arguments into FieldContext.Args when it runs. The
// decision itself belongs to the authorizer — this only extracts the argument.
func WorkspaceAccessMiddleware(access interfaces.WorkspaceAuthorizer) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (any, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil || (fc.Object != "Query" && fc.Object != "Mutation") {
			return next(ctx)
		}
		workspaceID, ok := fc.Args[workspaceIDArg].(string)
		if !ok {
			return next(ctx)
		}
		if err := access.AuthorizeCurrentUser(ctx, workspaceID); err != nil {
			return nil, err
		}
		return next(ctx)
	}
}
