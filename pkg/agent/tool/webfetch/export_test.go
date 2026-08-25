package webfetch

import (
	"context"
	"net"

	"github.com/gollem-dev/gollem"
)

// Test seams. These re-export internal identifiers so the external
// (webfetch_test) package can exercise them without widening the production
// API surface.

var (
	ExtractForTest            = extract
	CollapseWhitespaceForTest = collapseWhitespace
	IsBlockedIPForTest        = isBlockedIP
	IsPDFContentTypeForTest   = isPDFContentType
)

// FetchForTest exposes the unexported fetch method.
func (c *Client) FetchForTest(ctx context.Context, rawURL string) (int, string, []byte, bool, error) {
	return c.fetch(ctx, rawURL)
}

// AnalyzeTextForTest exposes the unexported analyzeText method, flattening the
// unexported analyzeResult into plain values for assertions.
func (c *Client) AnalyzeTextForTest(ctx context.Context, text string) (malicious bool, reason, markdown string, err error) {
	return flattenAnalyze(c.analyzeText(ctx, text))
}

// AnalyzePDFForTest exposes the unexported analyzePDF method.
func (c *Client) AnalyzePDFForTest(ctx context.Context, data []byte) (malicious bool, reason, markdown string, err error) {
	return flattenAnalyze(c.analyzePDF(ctx, data))
}

func flattenAnalyze(r *analyzeResult, err error) (bool, string, string, error) {
	if err != nil {
		return false, "", "", err
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

	// LastAnalyzeText records the text passed to analyzeText for assertions.
	LastAnalyzeText string
	// LastAnalyzePDF records the bytes passed to analyzePDF for assertions.
	LastAnalyzePDF []byte
	// AnalyzeCalled records whether either analyze method was invoked.
	AnalyzeCalled bool
	// AnalyzePDFCalled records whether the PDF path was taken.
	AnalyzePDFCalled bool
}

func (f *FakeFetchClient) fetch(_ context.Context, _ string) (int, string, []byte, bool, error) {
	if f.FetchErr != nil {
		return 0, "", nil, false, f.FetchErr
	}
	return f.Status, f.ContentType, f.Body, f.Truncated, nil
}

func (f *FakeFetchClient) analyzeText(_ context.Context, text string) (*analyzeResult, error) {
	f.AnalyzeCalled = true
	f.LastAnalyzeText = text
	return f.analyzeResponse()
}

func (f *FakeFetchClient) analyzePDF(_ context.Context, data []byte) (*analyzeResult, error) {
	f.AnalyzeCalled = true
	f.AnalyzePDFCalled = true
	f.LastAnalyzePDF = data
	return f.analyzeResponse()
}

func (f *FakeFetchClient) analyzeResponse() (*analyzeResult, error) {
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
