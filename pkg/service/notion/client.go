package notion

import (
	"context"
	"iter"
	"time"

	"github.com/jomei/notionapi"
	"github.com/m-mizutani/goerr/v2"
)

// client implements Service interface
type client struct {
	api *notionapi.Client
}

// New creates a new Notion service with the provided API token
func New(token string) (Service, error) {
	if token == "" {
		return nil, goerr.New("Notion API token is required")
	}

	return &client{
		api: notionapi.NewClient(
			notionapi.Token(token),
			notionapi.WithRetry(3), // Retry up to 3 times on rate limit (HTTP 429)
		),
	}, nil
}

// QueryUpdatedPages retrieves pages updated since the specified time from a database
func (c *client) QueryUpdatedPages(ctx context.Context, dbID string, since time.Time) iter.Seq2[*Page, error] {
	return func(yield func(*Page, error) bool) {
		var cursor notionapi.Cursor

		for {
			// Query database with filter
			onOrAfter := notionapi.Date(since)
			resp, err := c.api.Database.Query(ctx, notionapi.DatabaseID(dbID), &notionapi.DatabaseQueryRequest{
				Filter: &notionapi.TimestampFilter{
					Timestamp: "last_edited_time",
					LastEditedTime: &notionapi.DateFilterCondition{
						OnOrAfter: &onOrAfter,
					},
				},
				StartCursor: cursor,
				PageSize:    100,
			})

			if err != nil {
				yield(nil, goerr.Wrap(err, "failed to query database", goerr.V("dbID", dbID), goerr.V("since", since)))
				return
			}

			// Process each page
			for _, pageObj := range resp.Results {
				page, err := c.fetchPageDetails(ctx, pageObj.ID.String())
				if err != nil {
					if !yield(nil, err) {
						return
					}
					continue
				}

				if !yield(page, nil) {
					return
				}
			}

			// Check if there are more pages
			if !resp.HasMore {
				break
			}
			cursor = resp.NextCursor
		}
	}
}

// fetchPageDetails retrieves detailed information about a page including its blocks
func (c *client) fetchPageDetails(ctx context.Context, pageID string) (*Page, error) {
	// Get page properties
	pageObj, err := c.api.Page.Get(ctx, notionapi.PageID(pageID))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get page", goerr.V("pageID", pageID))
	}

	// Get page blocks
	blocks, err := c.fetchBlocksRecursively(ctx, pageID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch page blocks", goerr.V("pageID", pageID))
	}

	// Convert properties to map
	props := make(map[string]interface{})
	for key, prop := range pageObj.Properties {
		props[key] = prop
	}

	page := &Page{
		ID:             pageObj.ID.String(),
		Properties:     props,
		Blocks:         blocks,
		CreatedTime:    time.Time(pageObj.CreatedTime),
		LastEditedTime: time.Time(pageObj.LastEditedTime),
		URL:            pageObj.URL,
	}

	return page, nil
}

// fetchBlocksRecursively retrieves all blocks for a page or block, including nested children
func (c *client) fetchBlocksRecursively(ctx context.Context, blockID string) (Blocks, error) {
	var blocks Blocks
	var cursor notionapi.Cursor

	for {
		// Get blocks
		resp, err := c.api.Block.GetChildren(ctx, notionapi.BlockID(blockID), &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})

		if err != nil {
			return nil, goerr.Wrap(err, "failed to get block children", goerr.V("blockID", blockID))
		}

		for _, blockObj := range resp.Results {
			block, err := c.convertBlock(ctx, blockObj)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to convert block", goerr.V("blockID", blockObj.GetID()))
			}
			blocks = append(blocks, block)
		}

		if !resp.HasMore {
			break
		}
		cursor = notionapi.Cursor(resp.NextCursor)
	}

	return blocks, nil
}

// convertBlock converts a Notion API block to our Block type
func (c *client) convertBlock(ctx context.Context, blockObj notionapi.Block) (Block, error) {
	block := Block{
		ID:   blockObj.GetID().String(),
		Type: string(blockObj.GetType()),
	}

	// Extract content based on block type
	switch blockObj.GetType() {
	case notionapi.BlockTypeParagraph:
		if p, ok := blockObj.(*notionapi.ParagraphBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": p.Paragraph.RichText,
			}
		}
	case notionapi.BlockTypeHeading1:
		if h, ok := blockObj.(*notionapi.Heading1Block); ok {
			block.Content = map[string]interface{}{
				"rich_text": h.Heading1.RichText,
			}
		}
	case notionapi.BlockTypeHeading2:
		if h, ok := blockObj.(*notionapi.Heading2Block); ok {
			block.Content = map[string]interface{}{
				"rich_text": h.Heading2.RichText,
			}
		}
	case notionapi.BlockTypeHeading3:
		if h, ok := blockObj.(*notionapi.Heading3Block); ok {
			block.Content = map[string]interface{}{
				"rich_text": h.Heading3.RichText,
			}
		}
	case notionapi.BlockTypeBulletedListItem:
		if b, ok := blockObj.(*notionapi.BulletedListItemBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": b.BulletedListItem.RichText,
			}
		}
	case notionapi.BlockTypeNumberedListItem:
		if n, ok := blockObj.(*notionapi.NumberedListItemBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": n.NumberedListItem.RichText,
			}
		}
	case notionapi.BlockTypeCode:
		if code, ok := blockObj.(*notionapi.CodeBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": code.Code.RichText,
				"language":  code.Code.Language,
			}
		}
	case notionapi.BlockTypeQuote:
		if q, ok := blockObj.(*notionapi.QuoteBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": q.Quote.RichText,
			}
		}
	case notionapi.BlockTypeCallout:
		if c, ok := blockObj.(*notionapi.CalloutBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": c.Callout.RichText,
			}
		}
	case notionapi.BlockTypeToggle:
		if t, ok := blockObj.(*notionapi.ToggleBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": t.Toggle.RichText,
			}
		}
	case notionapi.BlockTypeToDo:
		if todo, ok := blockObj.(*notionapi.ToDoBlock); ok {
			block.Content = map[string]interface{}{
				"rich_text": todo.ToDo.RichText,
				"checked":   todo.ToDo.Checked,
			}
		}
	case notionapi.BlockTypeDivider:
		block.Content = nil
	default:
		// For unsupported block types, set content to nil
		block.Content = nil
	}

	// Recursively fetch children if the block has any
	if blockObj.GetHasChildren() {
		children, err := c.fetchBlocksRecursively(ctx, blockObj.GetID().String())
		if err != nil {
			return block, goerr.Wrap(err, "failed to fetch children blocks", goerr.V("blockID", blockObj.GetID()), goerr.V("blockType", blockObj.GetType()))
		}
		block.Children = children
	}

	return block, nil
}
