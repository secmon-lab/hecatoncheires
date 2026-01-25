package notion_test

import (
	"strings"
	"testing"

	"github.com/jomei/notionapi"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
)

func TestBlocks_ToMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		blocks notion.Blocks
		want   string
	}{
		{
			name: "paragraph",
			blocks: notion.Blocks{
				{
					Type: "paragraph",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "This is a paragraph"},
						},
					},
				},
			},
			want: "This is a paragraph\n",
		},
		{
			name: "headings",
			blocks: notion.Blocks{
				{
					Type: "heading_1",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Heading 1"},
						},
					},
				},
				{
					Type: "heading_2",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Heading 2"},
						},
					},
				},
				{
					Type: "heading_3",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Heading 3"},
						},
					},
				},
			},
			want: "# Heading 1\n## Heading 2\n### Heading 3\n",
		},
		{
			name: "bulleted list",
			blocks: notion.Blocks{
				{
					Type: "bulleted_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Item 1"},
						},
					},
				},
				{
					Type: "bulleted_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Item 2"},
						},
					},
				},
			},
			want: "- Item 1\n- Item 2\n",
		},
		{
			name: "numbered list",
			blocks: notion.Blocks{
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "First"},
						},
					},
				},
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Second"},
						},
					},
				},
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Third"},
						},
					},
				},
			},
			want: "1. First\n2. Second\n3. Third\n",
		},
		{
			name: "nested numbered list",
			blocks: notion.Blocks{
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{{PlainText: "Parent 1"}},
					},
					Children: notion.Blocks{
						{
							Type: "numbered_list_item",
							Content: map[string]interface{}{
								"rich_text": []notionapi.RichText{{PlainText: "Child 1.1"}},
							},
						},
					},
				},
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{{PlainText: "Parent 2"}},
					},
				},
			},
			want: "1. Parent 1\n  1. Child 1.1\n2. Parent 2\n",
		},
		{
			name: "code block",
			blocks: notion.Blocks{
				{
					Type: "code",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "fmt.Println(\"Hello\")"},
						},
						"language": "go",
					},
				},
			},
			want: "```go\nfmt.Println(\"Hello\")\n```\n",
		},
		{
			name: "quote",
			blocks: notion.Blocks{
				{
					Type: "quote",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "This is a quote"},
						},
					},
				},
			},
			want: "> This is a quote\n",
		},
		{
			name: "divider",
			blocks: notion.Blocks{
				{
					Type: "divider",
				},
			},
			want: "---\n",
		},
		{
			name: "to-do",
			blocks: notion.Blocks{
				{
					Type: "to_do",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Task 1"},
						},
						"checked": false,
					},
				},
				{
					Type: "to_do",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Task 2"},
						},
						"checked": true,
					},
				},
			},
			want: "- [ ] Task 1\n- [x] Task 2\n",
		},
		{
			name: "nested blocks",
			blocks: notion.Blocks{
				{
					Type: "bulleted_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Parent item"},
						},
					},
					Children: notion.Blocks{
						{
							Type: "bulleted_list_item",
							Content: map[string]interface{}{
								"rich_text": []notionapi.RichText{
									{PlainText: "Child item"},
								},
							},
						},
					},
				},
			},
			want: "- Parent item\n  - Child item\n",
		},
		{
			name: "rich text formatting",
			blocks: notion.Blocks{
				{
					Type: "paragraph",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{
								PlainText: "bold",
								Annotations: &notionapi.Annotations{
									Bold: true,
								},
							},
							{PlainText: " and "},
							{
								PlainText: "italic",
								Annotations: &notionapi.Annotations{
									Italic: true,
								},
							},
							{PlainText: " and "},
							{
								PlainText: "code",
								Annotations: &notionapi.Annotations{
									Code: true,
								},
							},
						},
					},
				},
			},
			want: "**bold** and *italic* and `code`\n",
		},
		{
			name: "link",
			blocks: notion.Blocks{
				{
					Type: "paragraph",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{
								PlainText: "link text",
								Href:      "https://example.com",
							},
						},
					},
				},
			},
			want: "[link text](https://example.com)\n",
		},
		{
			name:   "empty blocks",
			blocks: notion.Blocks{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.blocks.ToMarkdown()
			if got != tt.want {
				t.Errorf("ToMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlocks_ToMarkdown_ComplexNesting(t *testing.T) {
	blocks := notion.Blocks{
		{
			Type: "heading_1",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Document Title"},
				},
			},
		},
		{
			Type: "paragraph",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Introduction paragraph"},
				},
			},
		},
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "First item"},
				},
			},
			Children: notion.Blocks{
				{
					Type: "bulleted_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Nested bullet"},
						},
					},
				},
			},
		},
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Second item"},
				},
			},
		},
	}

	want := "# Document Title\nIntroduction paragraph\n1. First item\n  - Nested bullet\n2. Second item\n"
	got := blocks.ToMarkdown()

	if got != want {
		t.Errorf("ToMarkdown() with complex nesting:\ngot  = %q\nwant = %q", got, want)
	}
}

func TestBlocks_ToMarkdown_NestedNumberedLists(t *testing.T) {
	blocks := notion.Blocks{
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "First item"},
				},
			},
			Children: notion.Blocks{
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Nested first"},
						},
					},
				},
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Nested second"},
						},
					},
				},
			},
		},
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Second item"},
				},
			},
		},
	}

	// Nested numbered lists should start from 1
	want := "1. First item\n  1. Nested first\n  2. Nested second\n2. Second item\n"
	got := blocks.ToMarkdown()

	if got != want {
		t.Errorf("ToMarkdown() with nested numbered lists:\ngot  = %q\nwant = %q", got, want)
	}
}

func TestBlocks_ToMarkdown_ToggleWithNumberedList(t *testing.T) {
	blocks := notion.Blocks{
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "First item"},
				},
			},
		},
		{
			Type: "toggle",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Toggle content"},
				},
			},
			Children: notion.Blocks{
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Toggle nested first"},
						},
					},
				},
				{
					Type: "numbered_list_item",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{
							{PlainText: "Toggle nested second"},
						},
					},
				},
			},
		},
		{
			Type: "numbered_list_item",
			Content: map[string]interface{}{
				"rich_text": []notionapi.RichText{
					{PlainText: "Second item"},
				},
			},
		},
	}

	// Toggle block should have its own numbered list context starting from 1
	// Note: After toggle, the numbered list restarts from 1 because toggle uses continue
	want := "1. First item\n<details><summary>Toggle content</summary>\n  1. Toggle nested first\n  2. Toggle nested second\n</details>\n1. Second item\n"
	got := blocks.ToMarkdown()

	if got != want {
		t.Errorf("ToMarkdown() with toggle and numbered list:\ngot  = %q\nwant = %q", got, want)
	}
}

func TestPage_ToMarkdown(t *testing.T) {
	t.Run("with properties and blocks", func(t *testing.T) {
		page := &notion.Page{
			Properties: map[string]interface{}{
				"Title": notionapi.TitleProperty{
					Title: []notionapi.RichText{{PlainText: "Test Document"}},
				},
				"Status": notionapi.StatusProperty{
					Status: notionapi.Status{Name: "In Progress"},
				},
			},
			Blocks: notion.Blocks{
				{
					Type: "paragraph",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{{PlainText: "Content here"}},
					},
				},
			},
		}

		got := page.ToMarkdown()

		// Check frontmatter exists
		if !strings.HasPrefix(got, "---\n") {
			t.Error("expected YAML frontmatter to start with ---")
		}
		if !strings.Contains(got, "---\n\n") {
			t.Error("expected YAML frontmatter to end with ---")
		}

		// Check properties are included
		if !strings.Contains(got, "Title: Test Document") {
			t.Error("expected Title property in frontmatter")
		}
		if !strings.Contains(got, "Status: In Progress") {
			t.Error("expected Status property in frontmatter")
		}

		// Check content is included after frontmatter
		if !strings.Contains(got, "Content here") {
			t.Error("expected block content after frontmatter")
		}
	})

	t.Run("without properties", func(t *testing.T) {
		page := &notion.Page{
			Properties: map[string]interface{}{},
			Blocks: notion.Blocks{
				{
					Type: "paragraph",
					Content: map[string]interface{}{
						"rich_text": []notionapi.RichText{{PlainText: "Just content"}},
					},
				},
			},
		}

		got := page.ToMarkdown()

		// Should not have frontmatter
		if strings.HasPrefix(got, "---") {
			t.Error("expected no frontmatter when properties are empty")
		}

		// Should have content
		if got != "Just content\n" {
			t.Errorf("expected 'Just content\\n', got %q", got)
		}
	})

	t.Run("various property types", func(t *testing.T) {
		page := &notion.Page{
			Properties: map[string]interface{}{
				"Name": notionapi.TitleProperty{
					Title: []notionapi.RichText{{PlainText: "My Page"}},
				},
				"Description": notionapi.RichTextProperty{
					RichText: []notionapi.RichText{{PlainText: "A description"}},
				},
				"Tags": notionapi.MultiSelectProperty{
					MultiSelect: []notionapi.Option{
						{Name: "tag1"},
						{Name: "tag2"},
					},
				},
				"Priority": notionapi.SelectProperty{
					Select: notionapi.Option{Name: "High"},
				},
				"Count": notionapi.NumberProperty{
					Number: 42,
				},
				"Done": notionapi.CheckboxProperty{
					Checkbox: true,
				},
				"Link": notionapi.URLProperty{
					URL: "https://example.com",
				},
			},
			Blocks: notion.Blocks{},
		}

		got := page.ToMarkdown()

		if !strings.Contains(got, "Name: My Page") {
			t.Error("expected Name property")
		}
		if !strings.Contains(got, "Description: A description") {
			t.Error("expected Description property")
		}
		if !strings.Contains(got, "Tags: tag1, tag2") {
			t.Error("expected Tags property")
		}
		if !strings.Contains(got, "Priority: High") {
			t.Error("expected Priority property")
		}
		if !strings.Contains(got, "Count: 42") {
			t.Error("expected Count property")
		}
		if !strings.Contains(got, "Done: true") {
			t.Error("expected Done property")
		}
		if !strings.Contains(got, "Link: https://example.com") {
			t.Error("expected Link property")
		}
	})
}
