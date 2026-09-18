package slackfmt_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/slackfmt"
)

func TestSectionNamesEveryForbiddenConstruct(t *testing.T) {
	got := slackfmt.Section()
	gt.String(t, got).NotEqual("")

	// Each entry is a construct that reached a Slack thread as literal characters
	// because Slack's mrkdwn does not define it. The section has to name the
	// construct itself, not merely say "use mrkdwn".
	for _, want := range []string{
		"`#`, `##`, `###`",
		"`**bold**`",
		"`~~struck~~`",
		"[label](https://example.com)",
		"`---`, `***`",
		"| a | b |",
	} {
		gt.String(t, got).Contains(want)
	}
}

func TestSectionStatesWhatItDoesNotCover(t *testing.T) {
	got := slackfmt.Section()

	// Case and Action content is shown in the web UI as Markdown as well, so the
	// section must exclude it outright. It must not prescribe a style for it
	// either: the case-draft prompt tells the model that Markdown is fine in a
	// description, and a second instruction here would contradict it.
	gt.String(t, got).Contains("case description")
	gt.String(t, got).Contains("web UI")
	gt.String(t, got).Contains("does not govern them")
	gt.Bool(t, strings.Contains(got, "plain prose")).False()

	// A question's text and its option labels render as Slack plain_text, where no
	// marker of any kind is applied.
	gt.String(t, got).Contains("question")
	gt.String(t, got).Contains("NO formatting at all")
}

func TestSectionStaysShortEnoughToAppendToEverySystemPrompt(t *testing.T) {
	// Six host prompts carry this verbatim, and each of them is sent with every
	// LLM call of a run. Growth here is paid for on all of them at once.
	gt.Number(t, strings.Count(slackfmt.Section(), "\n")+1).LessOrEqual(25)
}

func TestSectionCarriesNoPerTurnValue(t *testing.T) {
	// The section is appended to a system prompt that must stay byte-identical
	// across turns to remain a prompt-cache hit.
	gt.String(t, slackfmt.Section()).Equal(slackfmt.Section())
	gt.Bool(t, strings.Contains(slackfmt.Section(), "{{")).False()
}

func TestToolHintIsShortEnoughToRideOnEveryCall(t *testing.T) {
	got := slackfmt.ToolHint()
	gt.String(t, got).NotEqual("")

	// A tool spec is sent with every LLM call of a run, so the hint stays at two
	// sentences: one sentence separator and a closing period.
	gt.Number(t, strings.Count(got, ". ")).LessOrEqual(1)
	gt.Bool(t, strings.HasSuffix(got, ".")).True()

	gt.String(t, got).Contains("mrkdwn")
	gt.String(t, got).Contains("| a | b |")
}
