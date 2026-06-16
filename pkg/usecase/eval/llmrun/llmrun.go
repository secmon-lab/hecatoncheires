// Package llmrun provides the gollem-backed implementation of
// evaltype.Completer. It wraps gollem.New(...).Execute so the eval components
// can issue a single system+user completion (optionally JSON-schema
// constrained) without each re-implementing the agent wiring.
package llmrun

import (
	"context"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
)

// Completer drives a single gollem completion against one LLM client.
type Completer struct {
	llm gollem.LLMClient
}

var _ evaltype.Completer = (*Completer)(nil)

// New builds a Completer over the given LLM client.
func New(llm gollem.LLMClient) *Completer {
	return &Completer{llm: llm}
}

// Complete runs one completion. When schema is non-nil the call is constrained
// to JSON output and the first top-level JSON object is returned; otherwise the
// joined plain text is returned. LoopLimit is 2 (one generate plus the minimum
// to surface a structured-output round), matching the planexec final phase.
func (c *Completer) Complete(ctx context.Context, systemPrompt, userPrompt string, schema *gollem.Parameter) (string, error) {
	opts := []gollem.Option{
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithLoopLimit(2),
	}
	if schema != nil {
		opts = append(opts,
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(schema),
		)
	}

	resp, err := gollem.New(c.llm, opts...).Execute(ctx, gollem.Text(userPrompt))
	if err != nil {
		return "", goerr.Wrap(err, "llm completion failed")
	}
	if resp == nil || resp.IsEmpty() {
		return "", goerr.New("llm completion returned empty response")
	}

	combined := strings.Join(resp.Texts, "\n")
	if schema != nil {
		return extractJSONObject(combined), nil
	}
	return combined, nil
}

// extractJSONObject returns the substring from the first '{' to the last '}'
// inclusive, stripping any prose or markdown fences the model wrapped around
// the JSON. If no object delimiters are present the input is returned as-is so
// the caller's JSON decode surfaces a clear error.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < 0 || end < start {
		return s
	}
	return s[start : end+1]
}
