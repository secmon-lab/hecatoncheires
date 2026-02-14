package slack_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	libslack "github.com/slack-go/slack"
)

func TestNewFileFromSlack(t *testing.T) {
	t.Run("extracts all metadata from slack file", func(t *testing.T) {
		slackFile := libslack.File{
			ID:         "F12345",
			Name:       "screenshot.png",
			Mimetype:   "image/png",
			Filetype:   "png",
			Size:       102400,
			URLPrivate: "https://files.slack.com/files-pri/T123-F12345/screenshot.png",
			Permalink:  "https://myworkspace.slack.com/files/U123/F12345/screenshot.png",
			Thumb1024:  "https://files.slack.com/files-tmb/T123-F12345/screenshot_1024.png",
			Thumb720:   "https://files.slack.com/files-tmb/T123-F12345/screenshot_720.png",
			Thumb480:   "https://files.slack.com/files-tmb/T123-F12345/screenshot_480.png",
		}

		f := slack.NewFileFromSlack(slackFile)

		gt.Value(t, f.ID()).Equal("F12345")
		gt.Value(t, f.Name()).Equal("screenshot.png")
		gt.Value(t, f.Mimetype()).Equal("image/png")
		gt.Value(t, f.Filetype()).Equal("png")
		gt.Value(t, f.Size()).Equal(102400)
		gt.Value(t, f.URLPrivate()).Equal("https://files.slack.com/files-pri/T123-F12345/screenshot.png")
		gt.Value(t, f.Permalink()).Equal("https://myworkspace.slack.com/files/U123/F12345/screenshot.png")
		gt.Value(t, f.ThumbURL()).Equal("https://files.slack.com/files-tmb/T123-F12345/screenshot_1024.png")
	})

	t.Run("selects best available thumbnail", func(t *testing.T) {
		// Only has small thumbnails
		slackFile := libslack.File{
			ID:       "F12345",
			Name:     "small.png",
			Mimetype: "image/png",
			Filetype: "png",
			Thumb160: "https://files.slack.com/thumb_160.png",
			Thumb80:  "https://files.slack.com/thumb_80.png",
		}

		f := slack.NewFileFromSlack(slackFile)
		gt.Value(t, f.ThumbURL()).Equal("https://files.slack.com/thumb_160.png")
	})

	t.Run("returns empty thumbURL when no thumbnails available", func(t *testing.T) {
		// Non-image file like PDF
		slackFile := libslack.File{
			ID:         "F67890",
			Name:       "document.pdf",
			Mimetype:   "application/pdf",
			Filetype:   "pdf",
			Size:       204800,
			URLPrivate: "https://files.slack.com/files-pri/T123-F67890/document.pdf",
			Permalink:  "https://myworkspace.slack.com/files/U123/F67890/document.pdf",
		}

		f := slack.NewFileFromSlack(slackFile)
		gt.Value(t, f.ThumbURL()).Equal("")
	})
}

func TestNewFileFromData(t *testing.T) {
	f := slack.NewFileFromData(
		"F12345",
		"report.pdf",
		"application/pdf",
		"pdf",
		512000,
		"https://files.slack.com/files-pri/T123-F12345/report.pdf",
		"https://myworkspace.slack.com/files/U123/F12345/report.pdf",
		"https://files.slack.com/thumb_360.png",
	)

	gt.Value(t, f.ID()).Equal("F12345")
	gt.Value(t, f.Name()).Equal("report.pdf")
	gt.Value(t, f.Mimetype()).Equal("application/pdf")
	gt.Value(t, f.Filetype()).Equal("pdf")
	gt.Value(t, f.Size()).Equal(512000)
	gt.Value(t, f.URLPrivate()).Equal("https://files.slack.com/files-pri/T123-F12345/report.pdf")
	gt.Value(t, f.Permalink()).Equal("https://myworkspace.slack.com/files/U123/F12345/report.pdf")
	gt.Value(t, f.ThumbURL()).Equal("https://files.slack.com/thumb_360.png")
}
