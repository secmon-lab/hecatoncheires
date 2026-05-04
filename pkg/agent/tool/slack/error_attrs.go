package slacktool

import (
	"errors"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

// slackErrorAttrs returns goerr options describing a Slack API error so
// callers can attach the slack-side error code and response metadata to a
// wrapped error without each call site repeating the errors.As dance.
//
// Returns nil for non-Slack errors (and for nil), which is safe to pass
// through Go's variadic spread (no extra options are appended).
func slackErrorAttrs(err error) []goerr.Option {
	var serr slack.SlackErrorResponse
	if !errors.As(err, &serr) {
		return nil
	}

	opts := []goerr.Option{
		goerr.V("slack_error", serr.Err),
	}
	if msgs := serr.ResponseMetadata.Messages; len(msgs) > 0 {
		opts = append(opts, goerr.V("slack_response_messages", msgs))
	}
	if warnings := serr.ResponseMetadata.Warnings; len(warnings) > 0 {
		opts = append(opts, goerr.V("slack_response_warnings", warnings))
	}
	return opts
}
