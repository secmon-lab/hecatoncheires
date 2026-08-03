package slack_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

func TestMessageBody(t *testing.T) {
	t.Run("plain text only", func(t *testing.T) {
		got := slack.MessageBody("hello world", goslack.Blocks{}, nil)
		gt.Equal(t, got, "hello world")
	})

	t.Run("empty everywhere", func(t *testing.T) {
		got := slack.MessageBody("", goslack.Blocks{}, nil)
		gt.Equal(t, got, "")
	})

	// The bug this function exists for: the GitHub Slack integration renders
	// the whole notification through attachments and leaves text empty, so a
	// reader that looks only at text sees nothing.
	t.Run("attachment-only GitHub notification", func(t *testing.T) {
		got := slack.MessageBody("", goslack.Blocks{}, []goslack.Attachment{{
			Pretext:   "Pull request opened by octocat",
			Title:     "#297 Add a Design Doc for coding agent usage observability",
			TitleLink: "https://github.com/example/design-doc/pull/297",
			Text:      "Adds a Design Doc describing how coding agent usage is observed.",
			Fields: []goslack.AttachmentField{
				{Title: "Reviewers", Value: "@alice, @bob"},
				{Title: "Comments", Value: "1"},
			},
			Footer: "example/design-doc",
		}})

		gt.Value(t, got).Equal(strings.Join([]string{
			"Pull request opened by octocat",
			"#297 Add a Design Doc for coding agent usage observability",
			"https://github.com/example/design-doc/pull/297",
			"Adds a Design Doc describing how coding agent usage is observed.",
			"Reviewers: @alice, @bob",
			"Comments: 1",
			"example/design-doc",
		}, "\n"))
	})

	t.Run("attachment falls back to fallback when no other text", func(t *testing.T) {
		got := slack.MessageBody("", goslack.Blocks{}, []goslack.Attachment{{
			Fallback: "plaintext summary",
			Color:    "#36a64f",
		}})
		gt.Equal(t, got, "plaintext summary")
	})

	t.Run("fallback is not repeated when richer fields exist", func(t *testing.T) {
		got := slack.MessageBody("", goslack.Blocks{}, []goslack.Attachment{{
			Fallback: "PR opened",
			Title:    "PR opened",
			Text:     "details here",
		}})
		gt.Equal(t, got, "PR opened\ndetails here")
	})

	t.Run("block-only message", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewHeaderBlock(goslack.NewTextBlockObject(goslack.PlainTextType, "Deploy finished", false, false)),
			goslack.NewSectionBlock(
				goslack.NewTextBlockObject(goslack.MarkdownType, "*service*: api", false, false),
				[]*goslack.TextBlockObject{
					goslack.NewTextBlockObject(goslack.MarkdownType, "env: prod", false, false),
				},
				nil,
			),
			goslack.NewDividerBlock(),
			goslack.NewContextBlock("ctx",
				goslack.NewTextBlockObject(goslack.MarkdownType, "took 42s", false, false),
			),
		}}

		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "Deploy finished\n*service*: api\nenv: prod\ntook 42s")
	})

	t.Run("markdown block", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			&goslack.MarkdownBlock{Type: goslack.MBTMarkdown, Text: "# Title\nbody"},
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "# Title\nbody")
	})

	t.Run("image block contributes title and alt text", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewImageBlock("https://example.com/a.png", "a chart", "img",
				goslack.NewTextBlockObject(goslack.PlainTextType, "Weekly chart", false, false)),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "Weekly chart\na chart")
	})

	// A human message carries the same content twice: once as the top-level
	// text Slack derives, once as rich_text blocks. Emitting both would double
	// every human message in the agent's prompt.
	t.Run("rich_text duplicating text is not repeated", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
				goslack.NewRichTextSectionUserElement("U123", nil),
				goslack.NewRichTextSectionTextElement(" this is the auto review mechanism", nil),
			)),
		}}
		got := slack.MessageBody("<@U123> this is the auto review mechanism", blocks, nil)
		gt.Equal(t, got, "<@U123> this is the auto review mechanism")
	})

	// The rich_text skip must not depend on the rendered strings matching:
	// styling, list markers and quote prefixes all make the two forms differ,
	// and any formatted human message would otherwise be emitted twice.
	t.Run("formatted human message is not repeated despite differing render", func(t *testing.T) {
		bold := &goslack.RichTextSectionTextStyle{Bold: true}
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt",
				goslack.NewRichTextSection(
					goslack.NewRichTextSectionTextElement("please review ", nil),
					goslack.NewRichTextSectionTextElement("today", bold),
				),
				goslack.NewRichTextList(goslack.RTEListBullet, 0,
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("first", nil)),
				),
			),
		}}
		got := slack.MessageBody("please review *today*\n• first", blocks, nil)
		gt.Equal(t, got, "please review *today*\n• first")
	})

	// An attachment's body is independent of the message text, so rich_text
	// nested in an attachment is always rendered.
	t.Run("rich_text inside an attachment is kept even when text is set", func(t *testing.T) {
		got := slack.MessageBody("fallback line", goslack.Blocks{}, []goslack.Attachment{{
			Blocks: goslack.Blocks{BlockSet: []goslack.Block{
				goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
					goslack.NewRichTextSectionTextElement("attachment body", nil),
				)),
			}},
		}})
		gt.Equal(t, got, "fallback line\nattachment body")
	})

	// App-composed Block Kit is not mirrored into text, so it is rendered even
	// when text carries a short fallback.
	t.Run("section blocks are kept when text is set", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewSectionBlock(
				goslack.NewTextBlockObject(goslack.MarkdownType, "detail only in blocks", false, false),
				nil, nil),
		}}
		got := slack.MessageBody("short fallback", blocks, nil)
		gt.Equal(t, got, "short fallback\ndetail only in blocks")
	})

	t.Run("text styles render as mrkdwn markers", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
				goslack.NewRichTextSectionTextElement("b", &goslack.RichTextSectionTextStyle{Bold: true}),
				goslack.NewRichTextSectionTextElement(" i", &goslack.RichTextSectionTextStyle{Italic: true}),
				goslack.NewRichTextSectionTextElement(" s", &goslack.RichTextSectionTextStyle{Strike: true}),
				goslack.NewRichTextSectionTextElement(" c", &goslack.RichTextSectionTextStyle{Code: true}),
				goslack.NewRichTextSectionTextElement(" u", &goslack.RichTextSectionTextStyle{Underline: true}),
			)),
		}}
		got := slack.MessageBody("", blocks, nil)
		// The leading space stays outside the markers; Slack renders markers
		// literally when they wrap surrounding whitespace.
		gt.Equal(t, got, "*b* _i_ ~s~ `c` u")
	})

	t.Run("team, date and color elements are rendered", func(t *testing.T) {
		fallback := "Aug 1, 2026"
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
				&goslack.RichTextSectionTeamElement{Type: goslack.RTSETeam, TeamID: "T123"},
				goslack.NewRichTextSectionTextElement(" ", nil),
				&goslack.RichTextSectionDateElement{Type: goslack.RTSEDate, Timestamp: 1785673711, Fallback: &fallback},
				goslack.NewRichTextSectionTextElement(" ", nil),
				&goslack.RichTextSectionColorElement{Type: goslack.RTSEColor, Value: "#36a64f"},
			)),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "<!team^T123> Aug 1, 2026 #36a64f")
	})

	t.Run("date element without fallback renders the timestamp", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
				&goslack.RichTextSectionDateElement{Type: goslack.RTSEDate, Timestamp: 1785673711},
			)),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "2026-08-02T12:28:31Z")
	})

	t.Run("rich_text elements render in Slack mrkdwn form", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt", goslack.NewRichTextSection(
				goslack.NewRichTextSectionUserElement("U123", nil),
				goslack.NewRichTextSectionTextElement(" see ", nil),
				goslack.NewRichTextSectionLinkElement("https://example.com", "docs", nil),
				goslack.NewRichTextSectionTextElement(" in ", nil),
				goslack.NewRichTextSectionChannelElement("C999", nil),
				goslack.NewRichTextSectionTextElement(" ", nil),
				goslack.NewRichTextSectionEmojiElement("tada", 0, nil),
			)),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "<@U123> see <https://example.com|docs> in <#C999> :tada:")
	})

	t.Run("rich_text list and quote keep their markers", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt",
				goslack.NewRichTextList(goslack.RTEListBullet, 0,
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("first", nil)),
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("second", nil)),
				),
				&goslack.RichTextQuote{
					Type:     goslack.RTEQuote,
					Elements: []goslack.RichTextSectionElement{goslack.NewRichTextSectionTextElement("quoted", nil)},
				},
			),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "- first\n- second\n> quoted")
	})

	t.Run("ordered and nested lists are numbered and indented", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt",
				goslack.NewRichTextList(goslack.RTEListOrdered, 0,
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("step one", nil)),
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("step two", nil)),
				),
				goslack.NewRichTextList(goslack.RTEListBullet, 1,
					goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("nested", nil)),
				),
			),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "1. step one\n2. step two\n  - nested")
	})

	t.Run("ordered list honours its offset", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewRichTextBlock("rt",
				&goslack.RichTextList{
					Type:   goslack.RTEList,
					Style:  goslack.RTEListOrdered,
					Offset: 4,
					Elements: []goslack.RichTextElement{
						goslack.NewRichTextSection(goslack.NewRichTextSectionTextElement("fifth", nil)),
					},
				},
			),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "5. fifth")
	})

	t.Run("attachment blocks are read", func(t *testing.T) {
		got := slack.MessageBody("", goslack.Blocks{}, []goslack.Attachment{{
			Blocks: goslack.Blocks{BlockSet: []goslack.Block{
				goslack.NewSectionBlock(
					goslack.NewTextBlockObject(goslack.MarkdownType, "inside attachment", false, false),
					nil, nil),
			}},
		}})
		gt.Equal(t, got, "inside attachment")
	})

	t.Run("text, blocks and attachments combine in render order", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewHeaderBlock(goslack.NewTextBlockObject(goslack.PlainTextType, "from block", false, false)),
		}}
		got := slack.MessageBody("from text", blocks, []goslack.Attachment{{Text: "from attachment"}})
		gt.Equal(t, got, "from text\nfrom block\nfrom attachment")
	})

	t.Run("unknown block type is skipped without panicking", func(t *testing.T) {
		blocks := goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewActionBlock("act", goslack.NewButtonBlockElement("b", "v",
				goslack.NewTextBlockObject(goslack.PlainTextType, "Comment", false, false))),
			goslack.NewHeaderBlock(goslack.NewTextBlockObject(goslack.PlainTextType, "kept", false, false)),
		}}
		got := slack.MessageBody("", blocks, nil)
		gt.Equal(t, got, "kept")
	})
}
