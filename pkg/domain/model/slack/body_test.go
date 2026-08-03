package slack_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	libslack "github.com/slack-go/slack"

	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

func TestMessageBody(t *testing.T) {
	t.Run("plain text only", func(t *testing.T) {
		got := slackmodel.MessageBody("hello world", libslack.Blocks{}, nil)
		gt.Equal(t, got, "hello world")
	})

	t.Run("empty everywhere", func(t *testing.T) {
		got := slackmodel.MessageBody("", libslack.Blocks{}, nil)
		gt.Equal(t, got, "")
	})

	// The bug this function exists for: the GitHub Slack integration renders
	// the whole notification through attachments and leaves text empty, so a
	// reader that looks only at text sees nothing.
	t.Run("attachment-only GitHub notification", func(t *testing.T) {
		got := slackmodel.MessageBody("", libslack.Blocks{}, []libslack.Attachment{{
			Pretext:   "Pull request opened by octocat",
			Title:     "#297 Add a Design Doc for coding agent usage observability",
			TitleLink: "https://github.com/example/design-doc/pull/297",
			Text:      "Adds a Design Doc describing how coding agent usage is observed.",
			Fields: []libslack.AttachmentField{
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
		got := slackmodel.MessageBody("", libslack.Blocks{}, []libslack.Attachment{{
			Fallback: "plaintext summary",
			Color:    "#36a64f",
		}})
		gt.Equal(t, got, "plaintext summary")
	})

	t.Run("fallback is not repeated when richer fields exist", func(t *testing.T) {
		got := slackmodel.MessageBody("", libslack.Blocks{}, []libslack.Attachment{{
			Fallback: "PR opened",
			Title:    "PR opened",
			Text:     "details here",
		}})
		gt.Equal(t, got, "PR opened\ndetails here")
	})

	t.Run("block-only message", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewHeaderBlock(libslack.NewTextBlockObject(libslack.PlainTextType, "Deploy finished", false, false)),
			libslack.NewSectionBlock(
				libslack.NewTextBlockObject(libslack.MarkdownType, "*service*: api", false, false),
				[]*libslack.TextBlockObject{
					libslack.NewTextBlockObject(libslack.MarkdownType, "env: prod", false, false),
				},
				nil,
			),
			libslack.NewDividerBlock(),
			libslack.NewContextBlock("ctx",
				libslack.NewTextBlockObject(libslack.MarkdownType, "took 42s", false, false),
			),
		}}

		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "Deploy finished\n*service*: api\nenv: prod\ntook 42s")
	})

	t.Run("markdown block", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			&libslack.MarkdownBlock{Type: libslack.MBTMarkdown, Text: "# Title\nbody"},
		}}
		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "# Title\nbody")
	})

	t.Run("image block contributes title and alt text", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewImageBlock("https://example.com/a.png", "a chart", "img",
				libslack.NewTextBlockObject(libslack.PlainTextType, "Weekly chart", false, false)),
		}}
		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "Weekly chart\na chart")
	})

	// A human message carries the same content twice: once as the top-level
	// text Slack derives, once as rich_text blocks. Emitting both would double
	// every human message in the agent's prompt.
	t.Run("rich_text duplicating text is not repeated", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewRichTextBlock("rt", libslack.NewRichTextSection(
				libslack.NewRichTextSectionUserElement("U123", nil),
				libslack.NewRichTextSectionTextElement(" this is the auto review mechanism", nil),
			)),
		}}
		got := slackmodel.MessageBody("<@U123> this is the auto review mechanism", blocks, nil)
		gt.Equal(t, got, "<@U123> this is the auto review mechanism")
	})

	t.Run("rich_text elements render in Slack mrkdwn form", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewRichTextBlock("rt", libslack.NewRichTextSection(
				libslack.NewRichTextSectionUserElement("U123", nil),
				libslack.NewRichTextSectionTextElement(" see ", nil),
				libslack.NewRichTextSectionLinkElement("https://example.com", "docs", nil),
				libslack.NewRichTextSectionTextElement(" in ", nil),
				libslack.NewRichTextSectionChannelElement("C999", nil),
				libslack.NewRichTextSectionTextElement(" ", nil),
				libslack.NewRichTextSectionEmojiElement("tada", 0, nil),
			)),
		}}
		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "<@U123> see <https://example.com|docs> in <#C999> :tada:")
	})

	t.Run("rich_text list and quote", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewRichTextBlock("rt",
				libslack.NewRichTextList(libslack.RTEListBullet, 0,
					libslack.NewRichTextSection(libslack.NewRichTextSectionTextElement("first", nil)),
					libslack.NewRichTextSection(libslack.NewRichTextSectionTextElement("second", nil)),
				),
				&libslack.RichTextQuote{
					Type:     libslack.RTEQuote,
					Elements: []libslack.RichTextSectionElement{libslack.NewRichTextSectionTextElement("quoted", nil)},
				},
			),
		}}
		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "first\nsecond\nquoted")
	})

	t.Run("attachment blocks are read", func(t *testing.T) {
		got := slackmodel.MessageBody("", libslack.Blocks{}, []libslack.Attachment{{
			Blocks: libslack.Blocks{BlockSet: []libslack.Block{
				libslack.NewSectionBlock(
					libslack.NewTextBlockObject(libslack.MarkdownType, "inside attachment", false, false),
					nil, nil),
			}},
		}})
		gt.Equal(t, got, "inside attachment")
	})

	t.Run("text, blocks and attachments combine in render order", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewHeaderBlock(libslack.NewTextBlockObject(libslack.PlainTextType, "from block", false, false)),
		}}
		got := slackmodel.MessageBody("from text", blocks, []libslack.Attachment{{Text: "from attachment"}})
		gt.Equal(t, got, "from text\nfrom block\nfrom attachment")
	})

	t.Run("unknown block type is skipped without panicking", func(t *testing.T) {
		blocks := libslack.Blocks{BlockSet: []libslack.Block{
			libslack.NewActionBlock("act", libslack.NewButtonBlockElement("b", "v",
				libslack.NewTextBlockObject(libslack.PlainTextType, "Comment", false, false))),
			libslack.NewHeaderBlock(libslack.NewTextBlockObject(libslack.PlainTextType, "kept", false, false)),
		}}
		got := slackmodel.MessageBody("", blocks, nil)
		gt.Equal(t, got, "kept")
	})
}
