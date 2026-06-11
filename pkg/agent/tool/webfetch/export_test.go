package webfetch

import (
	"context"
	"net"

	"github.com/m-mizutani/gollem"
)

// Test seams. These re-export internal identifiers so the external
// (webfetch_test) package can exercise them without widening the production
// API surface.

var (
	ExtractForTest            = extract
	CollapseWhitespaceForTest = collapseWhitespace
	IsBlockedIPForTest        = isBlockedIP
)

// FetchForTest exposes the unexported fetch method.
func (c *Client) FetchForTest(ctx context.Context, rawURL string) (int, string, []byte, bool, error) {
	return c.fetch(ctx, rawURL)
}

// AnalyzeForTest exposes the unexported analyze method, flattening the
// unexported analyzeResult into plain values for assertions.
func (c *Client) AnalyzeForTest(ctx context.Context, text string) (malicious bool, reason, markdown string, err error) {
	r, aErr := c.analyze(ctx, text)
	if aErr != nil {
		return false, "", "", aErr
	}
	return r.Malicious, r.Reason, r.Markdown, nil
}

// FakeFetchClient is a test double for the fetchClient interface. It lives in
// the internal package because fetchClient has unexported methods that an
// external test package cannot implement.
type FakeFetchClient struct {
	Status      int
	ContentType string
	Body        []byte
	Truncated   bool
	FetchErr    error

	Malicious  bool
	Reason     string
	Markdown   string
	AnalyzeErr error

	// LastAnalyzeText records the text passed to analyze for assertions.
	LastAnalyzeText string
	// AnalyzeCalled records whether analyze was invoked.
	AnalyzeCalled bool
}

func (f *FakeFetchClient) fetch(_ context.Context, _ string) (int, string, []byte, bool, error) {
	if f.FetchErr != nil {
		return 0, "", nil, false, f.FetchErr
	}
	return f.Status, f.ContentType, f.Body, f.Truncated, nil
}

func (f *FakeFetchClient) analyze(_ context.Context, text string) (*analyzeResult, error) {
	f.AnalyzeCalled = true
	f.LastAnalyzeText = text
	if f.AnalyzeErr != nil {
		return nil, f.AnalyzeErr
	}
	return &analyzeResult{Malicious: f.Malicious, Reason: f.Reason, Markdown: f.Markdown}, nil
}

// NewToolForTest builds the gollem tool list around a FakeFetchClient.
func NewToolForTest(f *FakeFetchClient) []gollem.Tool {
	return newTools(f)
}

// ParseIPForTest is a thin helper so range tables in the external test can
// build net.IP values without importing net at every call site.
func ParseIPForTest(s string) net.IP {
	return net.ParseIP(s)
}
