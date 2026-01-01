package notion_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   "test-token",
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "whitespace token",
			token:   "   ",
			wantErr: false, // Not validated, but will fail on API call
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := notion.New(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Error("New() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("New() unexpected error: %v", err)
			}
			if svc == nil {
				t.Error("New() returned nil service")
			}
		})
	}
}

func TestNew_WithRetryOption(t *testing.T) {
	// Test that the service is created with retry option
	svc, err := notion.New("test-token")
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	if svc == nil {
		t.Fatal("Service should not be nil")
	}
	// The retry option is internal, but we've tested it's created without error
}

func TestQueryUpdatedPages_Integration(t *testing.T) {
	token := os.Getenv("TEST_NOTION_API_TOKEN")
	if token == "" {
		t.Skip("TEST_NOTION_API_TOKEN environment variable not set")
	}

	dbID := os.Getenv("TEST_NOTION_DATABASE_ID")
	if dbID == "" {
		t.Skip("TEST_NOTION_DATABASE_ID environment variable not set")
	}

	svc, err := notion.New(token)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	ctx := context.Background()
	// Period: November 1-30, 2024
	since := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2024, 11, 30, 23, 59, 59, 999999999, time.UTC)

	pageCount := 0
	var pages []*notion.Page

	for page, err := range svc.QueryUpdatedPages(ctx, dbID, since) {
		if err != nil {
			t.Fatalf("Iterator returned error: %v", err)
		}

		if page == nil {
			t.Error("Iterator returned nil page")
			continue
		}

		// Skip pages updated after November 30, 2024
		if page.LastEditedTime.After(until) {
			continue
		}

		pages = append(pages, page)
		pageCount++
	}

	t.Logf("Retrieved %d page(s) updated between %s and %s",
		pageCount, since.Format("2006-01-02"), until.Format("2006-01-02"))

	// Validate each page
	for i, page := range pages {
		t.Run(page.ID, func(t *testing.T) {
			// Validate page structure
			if page.ID == "" {
				t.Error("Page ID is empty")
			}
			if page.URL == "" {
				t.Error("Page URL is empty")
			}
			if page.Properties == nil {
				t.Error("Page Properties is nil")
			}
			if page.CreatedTime.IsZero() {
				t.Error("Page CreatedTime is zero")
			}
			if page.LastEditedTime.IsZero() {
				t.Error("Page LastEditedTime is zero")
			}

			// Check if LastEditedTime is within November 2024
			if page.LastEditedTime.Before(since) {
				t.Errorf("Page LastEditedTime %v is before since %v",
					page.LastEditedTime, since)
			}
			if page.LastEditedTime.After(until) {
				t.Errorf("Page LastEditedTime %v is after until %v",
					page.LastEditedTime, until)
			}

			t.Logf("Page %d: ID=%s, URL=%s, LastEditedTime=%s",
				i+1, page.ID, page.URL, page.LastEditedTime.Format(time.RFC3339))

			// Validate blocks
			if len(page.Blocks) > 0 {
				t.Logf("  Blocks: %d total", len(page.Blocks))
				validateBlocks(t, page.Blocks, 0)

				// Test markdown conversion
				markdown := page.Blocks.ToMarkdown()
				if markdown == "" {
					t.Log("  Warning: Markdown conversion returned empty string")
				} else {
					t.Logf("  Markdown length: %d characters", len(markdown))
					// Log first 200 characters of markdown
					if len(markdown) > 200 {
						t.Logf("  Markdown preview: %s...", markdown[:200])
					} else {
						t.Logf("  Markdown: %s", markdown)
					}
				}
			} else {
				t.Log("  No blocks in this page")
			}

			// Validate properties
			if len(page.Properties) > 0 {
				t.Logf("  Properties: %d total", len(page.Properties))
				for key := range page.Properties {
					t.Logf("    - %s", key)
				}
			}
		})
	}

	if pageCount == 0 {
		t.Log("No pages found in the specified time range (November 2024)")
	}
}

// validateBlocks recursively validates block structure
func validateBlocks(t *testing.T, blocks notion.Blocks, depth int) {
	indent := ""
	for range depth {
		indent += "  "
	}

	for i, block := range blocks {
		if block.ID == "" {
			t.Errorf("%sBlock[%d] ID is empty", indent, i)
		}
		if block.Type == "" {
			t.Errorf("%sBlock[%d] Type is empty", indent, i)
		}

		t.Logf("%s  Block %d: Type=%s, ID=%s, HasChildren=%v",
			indent, i+1, block.Type, block.ID, len(block.Children) > 0)

		// Recursively validate children
		if len(block.Children) > 0 {
			validateBlocks(t, block.Children, depth+1)
		}
	}
}

func TestQueryUpdatedPages_MarkdownOutput(t *testing.T) {
	token := os.Getenv("TEST_NOTION_API_TOKEN")
	if token == "" {
		t.Skip("TEST_NOTION_API_TOKEN environment variable not set")
	}

	dbID := os.Getenv("TEST_NOTION_DATABASE_ID")
	if dbID == "" {
		t.Skip("TEST_NOTION_DATABASE_ID environment variable not set")
	}

	svc, err := notion.New(token)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	ctx := context.Background()
	// Period: November 1-30, 2024
	since := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2024, 11, 30, 23, 59, 59, 999999999, time.UTC)

	foundPage := false
	for page, err := range svc.QueryUpdatedPages(ctx, dbID, since) {
		if err != nil {
			t.Fatalf("Iterator returned error: %v", err)
		}

		if page == nil {
			t.Error("Iterator returned nil page")
			continue
		}

		// Skip pages updated after November 30, 2024
		if page.LastEditedTime.After(until) {
			continue
		}

		foundPage = true

		t.Logf("\n=== Page: %s ===", page.ID)
		t.Logf("URL: %s", page.URL)
		t.Logf("Last Edited: %s", page.LastEditedTime.Format(time.RFC3339))
		t.Logf("Blocks: %d", len(page.Blocks))

		if len(page.Blocks) > 0 {
			markdown := page.Blocks.ToMarkdown()
			t.Logf("\n--- Markdown Output ---\n%s\n--- End of Markdown ---\n", markdown)
		} else {
			t.Log("No blocks in this page")
		}

		// Only test first page in the range
		break
	}

	if !foundPage {
		t.Log("No pages found in November 2024")
	}
}
