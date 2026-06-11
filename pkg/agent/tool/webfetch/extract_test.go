package webfetch_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
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

	t.Run("shift_jis html is decoded to utf-8", func(t *testing.T) {
		const jp = "ログイン障害の調査"
		sjis, _, encErr := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(jp))
		gt.NoError(t, encErr).Required()
		// Sanity: the encoded bytes are not already UTF-8 of the same string.
		gt.Bool(t, string(sjis) == jp).False()

		body := append([]byte("<html><body><p>"), sjis...)
		body = append(body, []byte("</p></body></html>")...)

		text, _, err := webfetch.ExtractForTest("text/html; charset=Shift_JIS", body)
		gt.NoError(t, err).Required()
		gt.String(t, text).Contains(jp)
	})
}

func TestCollapseWhitespace(t *testing.T) {
	t.Run("collapses spaces and newline runs and trims", func(t *testing.T) {
		in := "  a   b\n\n\n\nc  \t d  "
		out := webfetch.CollapseWhitespaceForTest(in)
		gt.String(t, out).Equal("a b\n\nc d")
	})

	t.Run("crlf line endings collapse like lf", func(t *testing.T) {
		// Without \r stripping, the stray \r resets the newline-run counter and
		// these 4 CRLF newlines would survive instead of collapsing to two.
		in := "a\r\n\r\n\r\n\r\nb"
		out := webfetch.CollapseWhitespaceForTest(in)
		gt.String(t, out).Equal("a\n\nb")
	})
}
