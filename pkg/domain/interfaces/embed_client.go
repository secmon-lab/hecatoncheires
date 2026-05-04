package interfaces

import "context"

// EmbedClient produces fixed-dimension embedding vectors for the given input
// strings. The signature mirrors gollem.LLMClient.GenerateEmbedding so any
// gollem client (in practice, the Gemini client wired via the dedicated
// embedding configuration) satisfies it directly.
type EmbedClient interface {
	GenerateEmbedding(ctx context.Context, dimension int, input []string) ([][]float64, error)
}
