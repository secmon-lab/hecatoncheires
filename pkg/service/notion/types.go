package notion

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/jomei/notionapi"
)

// Service provides interface to Notion API
type Service interface {
	// QueryUpdatedPages retrieves pages updated since the specified time from a database
	// Returns an iterator that yields Page and error pairs
	QueryUpdatedPages(ctx context.Context, dbID string, since time.Time) iter.Seq2[*Page, error]
}

// Page represents a Notion page with its properties and content
type Page struct {
	ID             string
	Properties     map[string]interface{}
	Blocks         Blocks
	CreatedTime    time.Time
	LastEditedTime time.Time
	URL            string
}

// Block represents a Notion block with recursive children
type Block struct {
	ID       string
	Type     string
	Content  interface{}
	Children Blocks
}

// Blocks is a slice of Block with helper methods
type Blocks []Block

// ToMarkdown converts blocks to Markdown format
func (b Blocks) ToMarkdown() string {
	var sb strings.Builder
	b.toMarkdownWithIndent(&sb, 0, &numberedListContext{})
	return sb.String()
}

type numberedListContext struct {
	counter int
}

func (b Blocks) toMarkdownWithIndent(sb *strings.Builder, indent int, nlCtx *numberedListContext) {
	indentStr := strings.Repeat("  ", indent)

	for i, block := range b {
		// Reset numbered list counter if not consecutive
		if i > 0 && b[i-1].Type == "numbered_list_item" && block.Type != "numbered_list_item" {
			nlCtx.counter = 0
		}

		switch block.Type {
		case "paragraph":
			text := extractRichText(block.Content)
			if text != "" {
				sb.WriteString(indentStr)
				sb.WriteString(text)
				sb.WriteString("\n")
			}

		case "heading_1":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("# ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "heading_2":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("## ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "heading_3":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("### ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "bulleted_list_item":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("- ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "numbered_list_item":
			text := extractRichText(block.Content)
			if i == 0 || b[i-1].Type != "numbered_list_item" {
				nlCtx.counter = 1
			} else {
				nlCtx.counter++
			}
			sb.WriteString(indentStr)
			fmt.Fprintf(sb, "%d. ", nlCtx.counter)
			sb.WriteString(text)
			sb.WriteString("\n")

		case "code":
			if codeData, ok := block.Content.(map[string]interface{}); ok {
				language := ""
				if lang, ok := codeData["language"].(string); ok {
					language = lang
				}
				text := extractRichText(codeData)
				sb.WriteString(indentStr)
				sb.WriteString("```")
				sb.WriteString(language)
				sb.WriteString("\n")
				sb.WriteString(indentStr)
				sb.WriteString(text)
				sb.WriteString("\n")
				sb.WriteString(indentStr)
				sb.WriteString("```\n")
			}

		case "quote":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("> ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "divider":
			sb.WriteString(indentStr)
			sb.WriteString("---\n")

		case "callout":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("> ")
			sb.WriteString(text)
			sb.WriteString("\n")

		case "toggle":
			text := extractRichText(block.Content)
			sb.WriteString(indentStr)
			sb.WriteString("<details><summary>")
			sb.WriteString(text)
			sb.WriteString("</summary>\n")
			if len(block.Children) > 0 {
				block.Children.toMarkdownWithIndent(sb, indent+1, &numberedListContext{})
			}
			sb.WriteString(indentStr)
			sb.WriteString("</details>\n")
			continue // Skip children processing below

		case "to_do":
			if todoData, ok := block.Content.(map[string]interface{}); ok {
				checked := false
				if c, ok := todoData["checked"].(bool); ok {
					checked = c
				}
				text := extractRichText(todoData)
				sb.WriteString(indentStr)
				if checked {
					sb.WriteString("- [x] ")
				} else {
					sb.WriteString("- [ ] ")
				}
				sb.WriteString(text)
				sb.WriteString("\n")
			}

		default:
			// For unsupported types, just extract text if available
			text := extractRichText(block.Content)
			if text != "" {
				sb.WriteString(indentStr)
				sb.WriteString(text)
				sb.WriteString("\n")
			}
		}

		// Process children for most block types (except toggle which handles it specially)
		if block.Type != "toggle" && len(block.Children) > 0 {
			// Create new numberedListContext for nested lists to reset numbering
			if block.Type == "numbered_list_item" || block.Type == "bulleted_list_item" {
				block.Children.toMarkdownWithIndent(sb, indent+1, &numberedListContext{})
			} else {
				block.Children.toMarkdownWithIndent(sb, indent+1, nlCtx)
			}
		}
	}
}

// extractRichText extracts plain text from Notion rich text content
func extractRichText(content interface{}) string {
	if content == nil {
		return ""
	}

	// Handle map with rich_text or text field
	if m, ok := content.(map[string]interface{}); ok {
		if richText, ok := m["rich_text"]; ok {
			return extractRichTextArray(richText)
		}
		if richText, ok := m["text"]; ok {
			return extractRichTextArray(richText)
		}
	}

	// Handle direct rich text array
	return extractRichTextArray(content)
}

func extractRichTextArray(richText interface{}) string {
	if richText == nil {
		return ""
	}

	var sb strings.Builder

	switch rt := richText.(type) {
	case []notionapi.RichText:
		for _, text := range rt {
			sb.WriteString(formatRichText(text))
		}
	case []interface{}:
		for _, item := range rt {
			if rtItem, ok := item.(notionapi.RichText); ok {
				sb.WriteString(formatRichText(rtItem))
			}
		}
	}

	return sb.String()
}

func formatRichText(rt notionapi.RichText) string {
	text := rt.PlainText

	// Apply formatting
	if rt.Annotations != nil {
		if rt.Annotations.Bold {
			text = "**" + text + "**"
		}
		if rt.Annotations.Italic {
			text = "*" + text + "*"
		}
		if rt.Annotations.Code {
			text = "`" + text + "`"
		}
		if rt.Annotations.Strikethrough {
			text = "~~" + text + "~~"
		}
	}

	// Handle links
	if rt.Href != "" {
		text = fmt.Sprintf("[%s](%s)", text, rt.Href)
	}

	return text
}
