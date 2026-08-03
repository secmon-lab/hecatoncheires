package slack_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

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
