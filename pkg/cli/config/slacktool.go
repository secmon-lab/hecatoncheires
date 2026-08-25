package config

import (
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/urfave/cli/v3"
)

// SlackTool holds the size bounds applied to the Slack read tools'
// results. It is separate from Slack because it configures no credential and
// reaches no API: it bounds what a tool result may inject into the model
// context, the way WebFetch does for a fetched page.
type SlackTool struct {
	maxTextBytes   int
	maxResultBytes int
}

// Flags returns CLI flags for the Slack agent tools.
//
// One pair covers every Slack read tool rather than one pair per tool, so an
// operator sets a single budget for what a Slack read may inject and a tool
// added later is bounded without a new flag.
func (s *SlackTool) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:     "slack-tool-max-text-size",
			Usage:    "Maximum bytes of a single Slack message's text in an agent tool result (excess is truncated). 0 disables the bound",
			Category: "Slack",
			// 4 KiB: longer than any ordinary Slack message, so a normal result
			// is untouched, while the occasional pasted log or long-form post —
			// which is what drives an outlier call — is cut. The model cannot
			// bound this itself: it chooses how many results to ask for, not how
			// long each one is.
			Value:       4096,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_TOOL_MAX_TEXT_SIZE"),
			Destination: &s.maxTextBytes,
		},
		&cli.IntFlag{
			Name:     "slack-tool-max-result-size",
			Usage:    "Maximum combined bytes of the messages one Slack agent tool call returns (excess messages are dropped and reported). 0 disables the bound",
			Category: "Slack",
			// 32 KiB: the per-message cap above bounds one long message but not
			// a call returning many short ones, and a sub-agent re-sends its
			// accumulated tool results on every later turn, so an unbounded call
			// is paid for repeatedly.
			Value:       32768,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_TOOL_MAX_RESULT_SIZE"),
			Destination: &s.maxResultBytes,
		},
	}
}

// Validate rejects a negative bound. Called at startup so a typo fails fast
// instead of silently behaving like "no limit": the tools read any non-positive
// value as "this bound is off", so `-1` would start a deployment with Slack
// reads unbounded and nothing but the startup log to show for it.
func (s *SlackTool) Validate() error {
	if s.maxTextBytes < 0 {
		return goerr.New("--slack-tool-max-text-size must not be negative",
			goerr.V("slack_tool_max_text_size", s.maxTextBytes))
	}
	if s.maxResultBytes < 0 {
		return goerr.New("--slack-tool-max-result-size must not be negative",
			goerr.V("slack_tool_max_result_size", s.maxResultBytes))
	}
	return nil
}

// LogAttrs returns log attributes for the Slack tool configuration.
func (s *SlackTool) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Int("max_text_bytes", s.maxTextBytes),
		slog.Int("max_result_bytes", s.maxResultBytes),
	}
}

// Limits returns the bounds the Slack read tools apply to their results.
func (s *SlackTool) Limits() slacktool.Limits {
	return slacktool.Limits{
		MaxTextBytes:   s.maxTextBytes,
		MaxResultBytes: s.maxResultBytes,
	}
}
