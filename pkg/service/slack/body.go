package slack

import (
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// MessageBody returns the human-visible body of a Slack message.
//
// Slack renders a message from three places: the top-level `text`, the Block
// Kit `blocks`, and the legacy `attachments`. Integrations such as the GitHub
// Slack app put the entire notification in `attachments` and leave `text`
// empty, so a reader that looks only at `text` sees nothing at all — which is
// how a Design Doc PR notification reached the agent as an empty message.
//
// The three parts are emitted in Slack's own render order.
//
// A top-level rich_text block is skipped when `text` is non-empty. rich_text is
// what Slack generates from a human's own composition, and Slack mirrors that
// same content into `text`; rendering both would double every human message.
// The other block types are app-composed Block Kit that `text` is not
// guaranteed to cover, so they are always rendered. Deciding by block type
// rather than by comparing the rendered strings matters because the comparison
// cannot be made reliable: reproducing Slack's own text serialisation
// byte-for-byte (bold markers, list bullets, code fences, escaping) is not a
// contract Slack documents. Attachments are always rendered — they are never
// mirrored into `text`.
func MessageBody(text string, blocks slack.Blocks, attachments []slack.Attachment) string {
	parts := make([]string, 0, 3)
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}

	add(text)
	add(blocksText(blocks, strings.TrimSpace(text) != ""))
	add(attachmentsText(attachments))

	return strings.Join(parts, "\n")
}

// blocksText renders a block list. skipRichText drops rich_text blocks, which
// the caller sets when the message's `text` already carries the same content.
// Blocks nested inside an attachment never set it: an attachment's body is
// independent of the message's `text`.
func blocksText(blocks slack.Blocks, skipRichText bool) string {
	lines := make([]string, 0, len(blocks.BlockSet))
	for _, b := range blocks.BlockSet {
		if _, isRichText := b.(*slack.RichTextBlock); isRichText && skipRichText {
			continue
		}
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
func blockText(block slack.Block) string {
	switch b := block.(type) {
	case *slack.SectionBlock:
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
	case *slack.HeaderBlock:
		return textObject(b.Text)
	case *slack.MarkdownBlock:
		return b.Text
	case *slack.ContextBlock:
		lines := make([]string, 0, len(b.ContextElements.Elements))
		for _, e := range b.ContextElements.Elements {
			if t, ok := e.(*slack.TextBlockObject); ok {
				if s := textObject(t); s != "" {
					lines = append(lines, s)
				}
			}
		}
		return strings.Join(lines, "\n")
	case *slack.ImageBlock:
		lines := make([]string, 0, 2)
		if s := textObject(b.Title); s != "" {
			lines = append(lines, s)
		}
		if alt := strings.TrimSpace(b.AltText); alt != "" {
			lines = append(lines, alt)
		}
		return strings.Join(lines, "\n")
	case *slack.RichTextBlock:
		return richTextElementsText(b.Elements)
	default:
		return ""
	}
}

func textObject(t *slack.TextBlockObject) string {
	if t == nil {
		return ""
	}
	return strings.TrimSpace(t.Text)
}

// richTextElementsText renders the top-level elements of a rich_text block.
// This runs only when the message's `text` is empty (see blocksText), i.e. for
// app-posted rich_text with no fallback — a human message's copy is skipped.
// List and quote markers are reproduced so the structure of such a message
// survives into the agent's prompt.
func richTextElementsText(elements []slack.RichTextElement) string {
	lines := make([]string, 0, len(elements))
	for _, e := range elements {
		switch el := e.(type) {
		case *slack.RichTextSection:
			appendRichTextLine(&lines, "", richTextSectionText(el.Elements))
		case *slack.RichTextQuote:
			appendRichTextLine(&lines, "> ", richTextSectionText(el.Elements))
		case *slack.RichTextPreformatted:
			appendRichTextLine(&lines, "", richTextSectionText(el.Elements))
		case *slack.RichTextList:
			lines = append(lines, richTextListLines(el)...)
		}
	}
	return strings.Join(lines, "\n")
}

// richTextListLines renders one list, prefixing each item the way Slack shows
// it. Offset is where an ordered list starts counting; Indent nests it.
func richTextListLines(list *slack.RichTextList) []string {
	indent := strings.Repeat("  ", max(list.Indent, 0))
	var out []string
	for i, e := range list.Elements {
		var body string
		switch el := e.(type) {
		case *slack.RichTextSection:
			body = richTextSectionText(el.Elements)
		default:
			body = richTextElementsText([]slack.RichTextElement{e})
		}
		marker := indent + "- "
		if list.Style == slack.RTEListOrdered {
			marker = indent + strconv.Itoa(list.Offset+i+1) + ". "
		}
		appendRichTextLine(&out, marker, body)
	}
	return out
}

func appendRichTextLine(lines *[]string, prefix, body string) {
	if body = strings.TrimSpace(body); body != "" {
		*lines = append(*lines, prefix+body)
	}
}

// richTextSectionText renders rich_text elements into the mrkdwn form Slack
// itself uses (`<@U…>`, `<url|label>`, `:emoji:`, `*bold*`). Reproducing the
// markup keeps an app-posted rich_text message readable; it is not relied on
// for any equality check (see MessageBody).
func richTextSectionText(elements []slack.RichTextSectionElement) string {
	var sb strings.Builder
	for _, e := range elements {
		switch el := e.(type) {
		case *slack.RichTextSectionTextElement:
			sb.WriteString(styledText(el.Text, el.Style))
		case *slack.RichTextSectionUserElement:
			sb.WriteString("<@" + el.UserID + ">")
		case *slack.RichTextSectionUserGroupElement:
			sb.WriteString("<!subteam^" + el.UsergroupID + ">")
		case *slack.RichTextSectionChannelElement:
			sb.WriteString("<#" + el.ChannelID + ">")
		case *slack.RichTextSectionTeamElement:
			sb.WriteString("<!team^" + el.TeamID + ">")
		case *slack.RichTextSectionBroadcastElement:
			sb.WriteString("<!" + el.Range + ">")
		case *slack.RichTextSectionEmojiElement:
			sb.WriteString(":" + el.Name + ":")
		case *slack.RichTextSectionColorElement:
			sb.WriteString(el.Value)
		case *slack.RichTextSectionDateElement:
			sb.WriteString(richTextDateText(el))
		case *slack.RichTextSectionLinkElement:
			if el.Text != "" {
				sb.WriteString("<" + el.URL + "|" + el.Text + ">")
			} else {
				sb.WriteString("<" + el.URL + ">")
			}
		}
	}
	return sb.String()
}

// styledText wraps a text run in the mrkdwn markers matching its style. Slack
// has no markup for underline / highlight, so those are rendered plain.
func styledText(text string, style *slack.RichTextSectionTextStyle) string {
	if style == nil || text == "" {
		return text
	}
	// Leading and trailing spaces must stay outside the markers, otherwise
	// Slack renders the markers literally instead of applying the style.
	body := strings.TrimSpace(text)
	if body == "" {
		return text
	}
	lead, trail := text[:strings.Index(text, body)], ""
	if end := len(lead) + len(body); end < len(text) {
		trail = text[end:]
	}

	switch {
	// Code wins on its own: Slack does not combine `code` with other markers.
	case style.Code:
		body = "`" + body + "`"
	default:
		if style.Bold {
			body = "*" + body + "*"
		}
		if style.Italic {
			body = "_" + body + "_"
		}
		if style.Strike {
			body = "~" + body + "~"
		}
	}
	return lead + body + trail
}

// richTextDateText renders a date element. Slack supplies Fallback as the
// human-readable rendering; without it the raw epoch is the only value there
// is, and dropping the element entirely would lose the reference.
func richTextDateText(el *slack.RichTextSectionDateElement) string {
	if el.Fallback != nil && *el.Fallback != "" {
		return *el.Fallback
	}
	return time.Unix(int64(el.Timestamp), 0).UTC().Format(time.RFC3339)
}

func attachmentsText(attachments []slack.Attachment) string {
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
func attachmentText(a slack.Attachment) string {
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
	appendLine(blocksText(a.Blocks, false))
	appendLine(a.Footer)

	if len(lines) == 0 {
		return strings.TrimSpace(a.Fallback)
	}
	return strings.Join(lines, "\n")
}
