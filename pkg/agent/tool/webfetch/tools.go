package webfetch

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
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
	analyzeText(ctx context.Context, text string) (*analyzeResult, error)
	analyzePDF(ctx context.Context, data []byte) (*analyzeResult, error)
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
		Description: "Fetch a web page or PDF over HTTP(S) and return its body reformatted as Markdown. The body is screened for indirect prompt injection before it is returned; if injection is detected the call fails instead of returning the content. Connections to non-public IP addresses are blocked.",
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

	var result *analyzeResult
	if isPDFContentType(contentType) {
		// A truncated PDF is a broken file, not a shorter one — and it keeps the
		// %PDF- signature, so nothing downstream would catch it. Handing that to
		// the model produces the least visible failure available ("read it, but
		// the content was incomplete"), so refuse instead. Benign for the same
		// reason as the unsupported-type error in extract: the model chose the
		// URL.
		if truncated {
			return nil, goerr.New("pdf exceeds the webfetch size limit",
				goerr.V("url", rawURL), goerr.V("content_type", contentType),
				goerr.V("bytes", len(body)), goerr.T(errutil.TagBenign))
		}
		if len(body) == 0 {
			return emptyResult(rawURL, status, contentType, truncated), nil
		}

		r, err := t.client.analyzePDF(ctx, body)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to analyze pdf", goerr.V("url", rawURL))
		}
		result = r
	} else {
		text, _, err := extract(contentType, body)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to extract body", goerr.V("url", rawURL))
		}

		// A body that renders to nothing — a WAF 403 page whose markup is all
		// script, a zero-length response — has no content to screen, and handing
		// it to analyze is a hard failure rather than an empty verdict: the
		// provider drops an empty text block, so the request carries no message at
		// all and is rejected (400 invalid_request_error). Report the empty result
		// with the status instead, matching how fetch already leaves non-2xx for
		// the agent to reason about.
		if strings.TrimSpace(text) == "" {
			return emptyResult(rawURL, status, contentType, truncated), nil
		}

		r, err := t.client.analyzeText(ctx, text)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to analyze body", goerr.V("url", rawURL))
		}
		result = r
	}
	if result.Malicious {
		// reason reaches the calling agent, because a failed tool call's goerr
		// values are rendered into its function response (pkg/agent/kernel,
		// toolErrorValuesMiddleware). That is intended: it is the screening
		// model's own short verdict, bounded by analyzeSchema, not the page it
		// judged — and an agent that can say WHY a fetch was refused can tell the
		// user. The page body itself never takes this path; a page able to write
		// through the screener's verdict could also have bought a benign one,
		// which passes the whole body through.
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

// emptyResult is the reply for a fetch that returned nothing to screen. It
// carries the HTTP status so the agent can tell "the page is empty" from "the
// server refused".
func emptyResult(rawURL string, status int, contentType string, truncated bool) map[string]any {
	return map[string]any{
		"result":       "",
		"url":          rawURL,
		"status":       status,
		"content_type": contentType,
		"truncated":    truncated,
	}
}
