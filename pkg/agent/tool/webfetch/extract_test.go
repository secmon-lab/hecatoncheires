package webfetch_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
)

func TestExtract(t *testing.T) {
	t.Run("html is rendered to structured text", func(t *testing.T) {
		html := `<html><head><title>T</title><style>.x{color:red}</style></head>
<body>
<h1>Title</h1>
<p>Hello <a href="https://example.com">world</a>.</p>
<ul><li>one</li><li>two</li></ul>
<script>alert('x')</script>
</body></html>`
		text, isHTML, err := webfetch.ExtractForTest("text/html; charset=utf-8", []byte(html))
		gt.NoError(t, err).Required()
		gt.Bool(t, isHTML).True()
		gt.String(t, text).Contains("# Title")
		gt.String(t, text).Contains("Hello world.")
		gt.String(t, text).Contains("- one")
		gt.String(t, text).Contains("- two")
		// script / style content must be dropped.
		gt.Bool(t, strings.Contains(text, "alert")).False()
		gt.Bool(t, strings.Contains(text, "color:red")).False()
		// href is dropped, only visible text kept.
		gt.Bool(t, strings.Contains(text, "https://example.com")).False()
	})

	t.Run("hidden nodes are dropped", func(t *testing.T) {
		html := `<body><p>visible</p><p hidden>secret-hidden</p><div style="display:none">secret-css</div></body>`
		text, _, err := webfetch.ExtractForTest("text/html", []byte(html))
		gt.NoError(t, err).Required()
		gt.String(t, text).Contains("visible")
		gt.Bool(t, strings.Contains(text, "secret-hidden")).False()
		gt.Bool(t, strings.Contains(text, "secret-css")).False()
	})

	t.Run("table renders with pipe separators", func(t *testing.T) {
		html := `<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>2</td></tr></table>`
		text, _, err := webfetch.ExtractForTest("application/xhtml+xml", []byte(html))
		gt.NoError(t, err).Required()
		gt.String(t, text).Contains("a | b")
		gt.String(t, text).Contains("1 | 2")
	})

	t.Run("json is returned verbatim", func(t *testing.T) {
		body := `{"key":"value","n":1}`
		text, isHTML, err := webfetch.ExtractForTest("application/json", []byte(body))
		gt.NoError(t, err).Required()
		gt.Bool(t, isHTML).False()
		gt.String(t, text).Equal(body)
	})

	t.Run("plain text is returned verbatim", func(t *testing.T) {
		body := "line1\nline2"
		text, isHTML, err := webfetch.ExtractForTest("text/plain; charset=utf-8", []byte(body))
		gt.NoError(t, err).Required()
		gt.Bool(t, isHTML).False()
		gt.String(t, text).Equal(body)
	})

	t.Run("binary content type is rejected", func(t *testing.T) {
		_, _, err := webfetch.ExtractForTest("application/octet-stream", []byte{0x00, 0x01})
		gt.Error(t, err).Required()
	})
}

func TestCollapseWhitespace(t *testing.T) {
	t.Run("collapses spaces and newline runs and trims", func(t *testing.T) {
		in := "  a   b\n\n\n\nc  \t d  "
		out := webfetch.CollapseWhitespaceForTest(in)
		gt.String(t, out).Equal("a b\n\nc d")
	})
}
