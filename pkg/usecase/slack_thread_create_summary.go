package usecase

import (
	"context"
	"fmt"
	"strings"

	goslack "github.com/slack-go/slack" //nolint:depguard

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
)

// buildThreadCaseSummaryBlocks renders the Block Kit summary posted to the
// thread once the initialization (create) agent commits a new case. It shows
// the title, description, the populated custom fields (resolved to option
// names), the board status, and a web-UI link. The fallback string is used by
// Slack clients that cannot render blocks.
func buildThreadCaseSummaryBlocks(ctx context.Context, c *model.Case, entry *model.WorkspaceEntry, url string) ([]goslack.Block, string) {
	header := i18n.T(ctx, i18n.MsgThreadCaseSummaryHeader)
	titleLabel := i18n.T(ctx, i18n.MsgThreadCaseSummaryTitle)
	descLabel := i18n.T(ctx, i18n.MsgThreadCaseSummaryDesc)
	statusLabel := i18n.T(ctx, i18n.MsgThreadCaseSummaryStatus)

	blocks := []goslack.Block{
		goslack.NewHeaderBlock(goslack.NewTextBlockObject(goslack.PlainTextType, header, true, false)),
	}

	body := fmt.Sprintf("*%s*\n%s", titleLabel, orDash(c.Title))
	if c.Description != "" {
		body += fmt.Sprintf("\n\n*%s*\n%s", descLabel, c.Description)
	}
	blocks = append(blocks, goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType, body, false, false), nil, nil))

	if fieldLines := renderSummaryFields(c, entry); len(fieldLines) > 0 {
		blocks = append(blocks, goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, strings.Join(fieldLines, "\n"), false, false), nil, nil))
	}

	if status := renderBoardStatus(c, entry); status != "" {
		blocks = append(blocks, goslack.NewContextBlock("thread_case_summary_status",
			goslack.NewTextBlockObject(goslack.MarkdownType, fmt.Sprintf("*%s*: %s", statusLabel, status), false, false)))
	}

	if url != "" {
		blocks = append(blocks, goslack.NewContextBlock("thread_case_summary_link",
			goslack.NewTextBlockObject(goslack.MarkdownType, i18n.T(ctx, i18n.MsgThreadCaseSummaryLink, url), false, false)))
	}

	fallback := header + " — " + orDash(c.Title)
	if url != "" {
		fallback += " " + url
	}
	return blocks, fallback
}

// renderSummaryFields turns the case's populated custom fields into
// "• Name: value" lines, resolving select / multi-select option ids to their
// display names via the workspace schema.
func renderSummaryFields(c *model.Case, entry *model.WorkspaceEntry) []string {
	if len(c.FieldValues) == 0 || entry == nil || entry.FieldSchema == nil {
		return nil
	}
	var lines []string
	for _, def := range entry.FieldSchema.Fields {
		fv, ok := c.FieldValues[def.ID]
		if !ok {
			continue
		}
		rendered := renderFieldValue(def, fv)
		if rendered == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("• *%s*: %s", def.Name, rendered))
	}
	return lines
}

func renderFieldValue(def config.FieldDefinition, fv model.FieldValue) string {
	switch def.Type {
	case types.FieldTypeSelect:
		if id, ok := fv.Value.(string); ok {
			return optionName(def, id)
		}
	case types.FieldTypeMultiSelect:
		ids := toStringSlice(fv.Value)
		if len(ids) == 0 {
			return ""
		}
		names := make([]string, 0, len(ids))
		for _, id := range ids {
			names = append(names, optionName(def, id))
		}
		return strings.Join(names, ", ")
	default:
		if fv.Value == nil {
			return ""
		}
		return fmt.Sprintf("%v", fv.Value)
	}
	return ""
}

func optionName(def config.FieldDefinition, id string) string {
	for _, o := range def.Options {
		if o.ID == id {
			return o.Name
		}
	}
	return id
}

func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case string:
		// LLMs sometimes emit a bare string for a single-option multi-select.
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func renderBoardStatus(c *model.Case, entry *model.WorkspaceEntry) string {
	if c.BoardStatus == "" {
		return ""
	}
	if entry != nil && entry.CaseStatusSet != nil {
		if def, ok := entry.CaseStatusSet.Get(c.BoardStatus); ok {
			return def.Name
		}
	}
	return c.BoardStatus
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
