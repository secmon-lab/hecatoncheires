package slacktool_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/slack-go/slack"
)

func TestSlackErrorAttrs_DirectSlackErrorResponse(t *testing.T) {
	src := slack.SlackErrorResponse{
		Err: "missing_scope",
		ResponseMetadata: slack.ResponseMetadata{
			Messages: []string{"required scope: search:read"},
			Warnings: []string{"superfluous_charset"},
		},
	}

	wrapped := goerr.Wrap(src, "search failed", slacktool.SlackErrorAttrsForTest(src)...)
	values := mergedValues(t, wrapped)

	gt.Value(t, values["slack_error"]).Equal("missing_scope")

	msgs, ok := values["slack_response_messages"].([]string)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, msgs).Length(1).Required()
	gt.String(t, msgs[0]).Equal("required scope: search:read")

	warns, ok := values["slack_response_warnings"].([]string)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, warns).Length(1).Required()
	gt.String(t, warns[0]).Equal("superfluous_charset")
}

func TestSlackErrorAttrs_ThroughGoerrWrap(t *testing.T) {
	// Real call sites first wrap the slack-go error and then re-wrap. The
	// helper must still see through the wrapping via errors.As.
	src := slack.SlackErrorResponse{Err: "channel_not_found"}
	inner := goerr.Wrap(src, "first")

	opts := slacktool.SlackErrorAttrsForTest(inner)
	gt.Array(t, opts).Length(1).Required()

	wrapped := goerr.Wrap(inner, "second", opts...)
	values := mergedValues(t, wrapped)
	gt.Value(t, values["slack_error"]).Equal("channel_not_found")
}

func TestSlackErrorAttrs_PlainError(t *testing.T) {
	gt.Array(t, slacktool.SlackErrorAttrsForTest(errors.New("plain"))).Length(0)
}

func TestSlackErrorAttrs_NilError(t *testing.T) {
	gt.Array(t, slacktool.SlackErrorAttrsForTest(nil)).Length(0)
}

func mergedValues(t *testing.T, err error) map[string]any {
	t.Helper()
	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	return ge.Values()
}
