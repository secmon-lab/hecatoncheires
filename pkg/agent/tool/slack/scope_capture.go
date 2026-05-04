package slacktool

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// scopeCaptureCtxKey identifies a *scopeCapture stashed in a request context
// so a wrapping http.Client can pass missing_scope details back to the caller
// without mutating shared state.
type scopeCaptureCtxKey struct{}

// scopeCapture carries the `needed` / `provided` strings that Slack returns
// at the top level of a missing_scope response. slack-go's SlackResponse
// parser drops them, so we extract them ourselves on the way through.
type scopeCapture struct {
	needed   string
	provided string
}

func contextWithScopeCapture(ctx context.Context) (context.Context, *scopeCapture) {
	cap := &scopeCapture{}
	return context.WithValue(ctx, scopeCaptureCtxKey{}, cap), cap
}

func scopeCaptureFromContext(ctx context.Context) *scopeCapture {
	if cap, ok := ctx.Value(scopeCaptureCtxKey{}).(*scopeCapture); ok {
		return cap
	}
	return nil
}

// scopeCaptureMaxBody caps how much of a Slack response body the wrapper
// buffers while looking for missing_scope details. The full error response
// is on the order of a few hundred bytes; anything larger is a real result
// payload that we deliberately do not parse — we just stream it through
// untouched to slack-go.
const scopeCaptureMaxBody = 64 << 10 // 64 KiB

// capturingHTTPClient wraps an http.Client to extract Slack `missing_scope`
// metadata (`needed` / `provided`) from the response body before handing the
// response back to slack-go. If the request context carries no
// scopeCapture, the wrapper is a no-op pass-through.
type capturingHTTPClient struct {
	inner *http.Client
}

func (c *capturingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.inner.Do(req)
	if err != nil {
		return resp, err
	}
	cap := scopeCaptureFromContext(req.Context())
	if cap == nil || resp.Body == nil {
		return resp, nil
	}

	// Read one extra byte so we can detect bodies larger than the budget.
	// In the oversize case we splice the buffered prefix back in front of
	// the remaining stream, so slack-go receives the response intact.
	head, readErr := io.ReadAll(io.LimitReader(resp.Body, scopeCaptureMaxBody+1))
	if readErr != nil {
		safe.Close(req.Context(), resp.Body)
		return nil, readErr
	}
	if len(head) > scopeCaptureMaxBody {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}
		return resp, nil
	}
	safe.Close(req.Context(), resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(head))

	var parsed struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Needed   string `json:"needed"`
		Provided string `json:"provided"`
	}
	if jsonErr := json.Unmarshal(head, &parsed); jsonErr != nil {
		return resp, nil
	}
	if parsed.OK || parsed.Error != "missing_scope" {
		return resp, nil
	}
	cap.needed = parsed.Needed
	cap.provided = parsed.Provided
	return resp, nil
}
