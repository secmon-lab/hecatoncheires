package slack

import (
	"slices"
	"strings"

	libslack "github.com/slack-go/slack"
)

// MessageBody returns the human-visible body of a Slack message.
//
// Slack renders a message from three places: the top-level `text`, the Block
// Kit `blocks`, and the legacy `attachments`. Integrations such as the GitHub
// Slack app put the entire notification in `attachments` and leave `text`
// empty, so a reader that looks only at `text` sees nothing at all — which is
// how a Design Doc PR notification reached the agent as an empty message.
//
// The three parts are emitted in Slack's own render order. A part that is
// byte-identical to one already emitted is dropped: a human message carries
// the same content twice (top-level `text` plus the rich_text blocks Slack
// derives it from), and emitting both would double every human message. The
// comparison is deliberately whole-part equality rather than a fuzzy match —
// duplicated content is harmless next to content silently lost.
func MessageBody(text string, blocks libslack.Blocks, attachments []libslack.Attachment) string {
	parts := make([]string, 0, 3)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if slices.Contains(parts, s) {
			return
		}
		parts = append(parts, s)
	}

	add(text)
	add(blocksText(blocks))
	add(attachmentsText(attachments))

	return strings.Join(parts, "\n")
}

func blocksText(blocks libslack.Blocks) string {
	lines := make([]string, 0, len(blocks.BlockSet))
	for _, b := range blocks.BlockSet {
		if s := blockText(b); s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}

// blockText extracts the text of a single block. Only the text-bearing block
// types are handled; interactive-only blocks (actions, input) and decorative
// ones (divider) carry no body, and an unrecognised block yields "" rather
// than an error — a new Slack block type must not break message ingestion.
func blockText(block libslack.Block) string {
	switch b := block.(type) {
	case *libslack.SectionBlock:
		lines := make([]string, 0, 1+len(b.Fields))
		if s := textObject(b.Text); s != "" {
			lines = append(lines, s)
		}
		for _, f := range b.Fields {
			if s := textObject(f); s != "" {
				lines = append(lines, s)
			}
		}
		return strings.Join(lines, "\n")
	case *libslack.HeaderBlock:
		return textObject(b.Text)
	case *libslack.MarkdownBlock:
		return b.Text
	case *libslack.ContextBlock:
		lines := make([]string, 0, len(b.ContextElements.Elements))
		for _, e := range b.ContextElements.Elements {
			if t, ok := e.(*libslack.TextBlockObject); ok {
				if s := textObject(t); s != "" {
					lines = append(lines, s)
				}
			}
		}
		return strings.Join(lines, "\n")
	case *libslack.ImageBlock:
		lines := make([]string, 0, 2)
		if s := textObject(b.Title); s != "" {
			lines = append(lines, s)
		}
		if alt := strings.TrimSpace(b.AltText); alt != "" {
			lines = append(lines, alt)
		}
		return strings.Join(lines, "\n")
	case *libslack.RichTextBlock:
		return richTextElementsText(b.Elements)
	default:
		return ""
	}
}

func textObject(t *libslack.TextBlockObject) string {
	if t == nil {
		return ""
	}
	return strings.TrimSpace(t.Text)
}

func richTextElementsText(elements []libslack.RichTextElement) string {
	lines := make([]string, 0, len(elements))
	for _, e := range elements {
		var s string
		switch el := e.(type) {
		case *libslack.RichTextSection:
			s = richTextSectionText(el.Elements)
		case *libslack.RichTextQuote:
			s = richTextSectionText(el.Elements)
		case *libslack.RichTextPreformatted:
			s = richTextSectionText(el.Elements)
		case *libslack.RichTextList:
			s = richTextElementsText(el.Elements)
		}
		if s = strings.TrimSpace(s); s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}

// richTextSectionText renders rich_text elements back into the mrkdwn form
// Slack itself puts in the top-level `text` field (`<@U…>`, `<url|label>`,
// `:emoji:`). Matching that form is what lets MessageBody recognise a human
// message's blocks as a duplicate of its text instead of emitting both.
func richTextSectionText(elements []libslack.RichTextSectionElement) string {
	var sb strings.Builder
	for _, e := range elements {
		switch el := e.(type) {
		case *libslack.RichTextSectionTextElement:
			sb.WriteString(el.Text)
		case *libslack.RichTextSectionUserElement:
			sb.WriteString("<@" + el.UserID + ">")
		case *libslack.RichTextSectionUserGroupElement:
			sb.WriteString("<!subteam^" + el.UsergroupID + ">")
		case *libslack.RichTextSectionChannelElement:
			sb.WriteString("<#" + el.ChannelID + ">")
		case *libslack.RichTextSectionBroadcastElement:
			sb.WriteString("<!" + el.Range + ">")
		case *libslack.RichTextSectionEmojiElement:
			sb.WriteString(":" + el.Name + ":")
		case *libslack.RichTextSectionLinkElement:
			if el.Text != "" {
				sb.WriteString("<" + el.URL + "|" + el.Text + ">")
			} else {
				sb.WriteString("<" + el.URL + ">")
			}
		}
	}
	return sb.String()
}

func attachmentsText(attachments []libslack.Attachment) string {
	lines := make([]string, 0, len(attachments))
	for _, a := range attachments {
		if s := attachmentText(a); s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}

// attachmentText renders one attachment in the order Slack displays it. The
// title link is kept because for integration notifications it is the only
// pointer to the underlying resource (the PR being reviewed, for instance).
// Fallback is the attachment's own plaintext substitute, so it is used only
// when nothing richer was present.
func attachmentText(a libslack.Attachment) string {
	lines := make([]string, 0, 6+len(a.Fields))
	appendLine := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			lines = append(lines, s)
		}
	}

	appendLine(a.Pretext)
	appendLine(a.AuthorName)
	appendLine(a.Title)
	appendLine(a.TitleLink)
	appendLine(a.Text)
	for _, f := range a.Fields {
		switch {
		case f.Title != "" && f.Value != "":
			appendLine(f.Title + ": " + f.Value)
		case f.Value != "":
			appendLine(f.Value)
		default:
			appendLine(f.Title)
		}
	}
	appendLine(blocksText(a.Blocks))
	appendLine(a.Footer)

	if len(lines) == 0 {
		return strings.TrimSpace(a.Fallback)
	}
	return strings.Join(lines, "\n")
}
