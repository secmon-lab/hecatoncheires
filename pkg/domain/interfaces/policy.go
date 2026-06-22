package interfaces

import "context"

// PolicyClient evaluates a Rego query against an input document and decodes
// the policy's result into out. It mirrors the opaq.Client.Query shape so the
// adapter is a thin wrapper, while keeping the usecase / controller layers
// free of any direct dependency on the Rego engine.
//
// query is a fully-qualified Rego reference such as "data.auth.mcp". input is
// marshalled to the policy's `input` document; out receives the policy's
// result (typically a struct with an `allow` boolean). An evaluation that
// produces no result is reported as an error rather than a zero-valued out.
type PolicyClient interface {
	Query(ctx context.Context, query string, input, out any) error
}
