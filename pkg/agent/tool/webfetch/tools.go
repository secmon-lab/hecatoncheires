package webfetch

import (
	"context"
	"fmt"
	"net/url"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// New returns the webfetch-backed agent tools. It returns nil (the tool is not
// registered) when client is nil or has no LLM client: the LLM screen is the
// only injection defense in this codebase — there is no HITL fallback — so
// webfetch fails closed rather than serving unscreened content.
func New(client *Client) []gollem.Tool {
	if client == nil || client.llm == nil {
		return nil
	}
	return newTools(client)
}

// fetchClient is the package-private surface the tool uses, defined here as the
// test seam. *Client satisfies it implicitly; tests inject a fake by calling
// newTools directly.
type fetchClient interface {
	fetch(ctx context.Context, rawURL string) (status int, contentType string, body []byte, truncated bool, err error)
	analyze(ctx context.Context, text string) (*analyzeResult, error)
}

func newTools(c fetchClient) []gollem.Tool {
	return []gollem.Tool{&fetchTool{client: c}}
}

type fetchTool struct {
	client fetchClient
}

func (t *fetchTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "webfetch",
		Description: "Fetch a web page over HTTP(S) and return its body reformatted as Markdown. The body is screened for indirect prompt injection before it is returned; if injection is detected the call fails instead of returning the content. Connections to non-public IP addresses are blocked.",
		Parameters: map[string]*gollem.Parameter{
			"url": {
				Type:        gollem.TypeString,
				Description: "The URL to fetch (http or https only).",
				Required:    true,
			},
		},
	}
}

func (t *fetchTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return nil, goerr.New("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to parse url", goerr.V("url", rawURL))
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return nil, goerr.New("unsupported url scheme (only http/https are allowed)",
			goerr.V("url", rawURL), goerr.V("scheme", parsed.Scheme))
	}
	if parsed.Host == "" {
		return nil, goerr.New("url is missing a host", goerr.V("url", rawURL))
	}

	tool.Update(ctx, fmt.Sprintf("Fetching %s", rawURL))

	status, contentType, body, truncated, err := t.client.fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	text, _, err := extract(contentType, body)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to extract body", goerr.V("url", rawURL))
	}

	result, err := t.client.analyze(ctx, text)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to analyze body", goerr.V("url", rawURL))
	}
	if result.Malicious {
		return nil, goerr.New("indirect prompt injection detected in fetched body",
			goerr.V("url", rawURL), goerr.V("reason", result.Reason))
	}

	return map[string]any{
		"result":       result.Markdown,
		"url":          rawURL,
		"status":       status,
		"content_type": contentType,
		"truncated":    truncated,
	}, nil
}
