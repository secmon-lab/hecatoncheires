package sample

import "context"

type closer2 interface{ Close() error }

// Closing via safe.Close — both bare and deferred forms — is the sanctioned
// pattern and must not be flagged. (safe is an undefined identifier here; goast
// only parses, so this is fine for a fixture.)
func good(ctx context.Context, c closer2) {
	defer safe.Close(ctx, c)
	safe.Close(ctx, c)
}
