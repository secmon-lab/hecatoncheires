---
paths:
  - "**/prompts/**/*.go"
  - "**/prompts/**/*.md"
  - "**/*prompt*.go"
---

# LLM Prompts

- Place every LLM prompt as a `.md` file under a `prompts/`
  directory that sits alongside the Go file consuming it (e.g.
  `pkg/usecase/prompts/draft_materializer.md` next to
  `pkg/usecase/draft_materializer.go`).
- Embed the prompt files with `//go:embed` and parse them through
  `text/template` (`template.New(...).Parse(...)` or
  `template.ParseFS`). Cache the parsed `*template.Template` at package
  init / `sync.Once`; do not re-parse per request.
- Inject dynamic values exclusively via template parameters and
  `{{ . }}` actions. **Never** build prompts with `fmt.Sprintf`,
  `strings.Builder`, `strings.ReplaceAll`, `+` concatenation, or any
  other string-level manipulation. If a value needs conditional
  inclusion, use `{{ if }}` / `{{ range }}` inside the template — not
  Go-side string assembly.
- Pass a single typed struct (e.g. `type DraftMaterializerInput
  struct { ... }`) to `tmpl.Execute`. Keep the struct definition next
  to the embed so the template's expected fields are discoverable.
- Markdown is for humans first: keep prompts readable with headings,
  bullet lists, and code fences. The template engine treats it as
  plain text, so no escaping is required beyond `{{` / `}}`.
- Every prompt template must have a `*_test.go` covering
  `tmpl.Execute` with at least:
  1. A representative happy-path input that exercises every template
     action (`{{ .Field }}`, `{{ range }}`, `{{ if }}`).
  2. Edge cases for empty / nil collections so the rendered output
     stays well-formed.
  3. A golden assertion on the rendered string (use `gt.Value(...).
     Equal(...)` or `gt.S(t, got).Contains(...)`) so prompt
     regressions are caught at CI time.
- When updating a prompt, edit the `.md` file and adjust the input
  struct + test — do not patch the rendered string in Go code.

## Text that is posted to Slack

- An agent host whose run can post text to Slack MUST carry
  `slackfmt.Section()` (`pkg/agent/slackfmt`) in its system prompt.
  Slack renders mrkdwn, not Markdown: a heading, a pipe table, a
  horizontal rule or `**bold**` reaches the thread as literal
  characters. That is what happened on every host that had no such
  section — the rules existed only as hand-written copies in two of
  them.
- Put it in the fixed part of the prompt. Where a host ends its prompt
  with an operator-supplied one (`wsagent`'s
  `[slack.workspace_agent]` prompt, `threadcase`'s
  `[case.prompts].create`, `assist`'s assist prompt), the section goes
  BEFORE it so the operator's text stays the last word. Where the
  operator's text sits mid-prompt with fixed sections after it (the
  Job prompt's per-case operator notes, which its own Guardrails
  section already follows), append the section at the end with the
  other fixed sections.
- Do NOT write the rules out again in a prompt file, and do not put
  them into `planexec` or `react`: both strategies pass the host's
  system prompt to the terminal call and to the direct-reply child, so
  a host that carries the section covers every path its text can take.
- A tool that posts to Slack states the same rules on the argument the
  model writes, through `slackfmt.ToolHint()`.
