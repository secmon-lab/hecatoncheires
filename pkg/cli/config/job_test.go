package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/pelletier/go-toml/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestJobSection_Validate_RoundTrip(t *testing.T) {
	const src = `
[[job]]
id = "summarize_on_create"
name = "Summarize"
description = "Auto-summarize new cases"
prompt = "summarize: {{.Case.Title}}"
events.case = { on = ["created"] }

[[job]]
id = "daily_digest"
prompt = "post daily digest"
events.scheduled = { cron = "0 9 * * *" }

[[job]]
id = "stale_check"
prompt = "remind stale"
events.scheduled = { every = "1h" }

[[job]]
id = "lifecycle_and_stale_watcher"
prompt = "do both"
disabled = true
events.case = { on = ["created", "closed"] }
events.scheduled = { every = "30m" }
`
	var app config.AppConfig
	gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
	gt.NoError(t, app.Validate()).Required()

	gt.Array(t, app.Jobs).Length(4).Required()

	first, err := app.Jobs[0].Validate("")
	gt.NoError(t, err).Required()
	gt.Value(t, first.ID).Equal("summarize_on_create")
	gt.Value(t, first.Events.Case).NotNil()
	gt.Array(t, first.Events.Case.On).Length(1)
	gt.Value(t, first.Events.Case.On[0]).Equal(model.CaseLifecycleCreated)
	gt.Value(t, first.Events.Scheduled).Nil()

	second, err := app.Jobs[1].Validate("")
	gt.NoError(t, err).Required()
	gt.Value(t, second.Events.Scheduled).NotNil()
	gt.Value(t, second.Events.Scheduled.CronExpr).Equal("0 9 * * *")
	gt.Number(t, int64(second.Events.Scheduled.Every)).Equal(0)
	gt.Value(t, second.Events.Scheduled.Cron).NotNil()

	third, err := app.Jobs[2].Validate("")
	gt.NoError(t, err).Required()
	gt.Number(t, int64(third.Events.Scheduled.Every)).Equal(int64(time.Hour))

	fourth, err := app.Jobs[3].Validate("")
	gt.NoError(t, err).Required()
	gt.Bool(t, fourth.Disabled).True()
	gt.Array(t, fourth.Events.Case.On).Length(2)
	gt.Value(t, fourth.Events.Case.On[0]).Equal(model.CaseLifecycleCreated)
	gt.Value(t, fourth.Events.Case.On[1]).Equal(model.CaseLifecycleClosed)
	gt.Number(t, int64(fourth.Events.Scheduled.Every)).Equal(int64(30 * time.Minute))

	// Default Strategy normalisation: absence in TOML means "simple".
	gt.Value(t, first.Strategy).Equal(model.JobStrategySimple)
	gt.Value(t, second.Strategy).Equal(model.JobStrategySimple)
}

func TestJobSection_Quiet(t *testing.T) {
	const src = `
[[job]]
id = "quiet_job"
prompt = "x"
quiet = true
events.case = { on = ["created"] }

[[job]]
id = "loud_job"
prompt = "y"
events.case = { on = ["created"] }
`
	var app config.AppConfig
	gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
	gt.Array(t, app.Jobs).Length(2).Required()

	quiet, err := app.Jobs[0].Validate("")
	gt.NoError(t, err).Required()
	gt.Bool(t, quiet.Quiet).True()

	// Absent in TOML defaults to false.
	loud, err := app.Jobs[1].Validate("")
	gt.NoError(t, err).Required()
	gt.Bool(t, loud.Quiet).False()
}

func TestJobSection_Strategy(t *testing.T) {
	t.Run("explicit simple", func(t *testing.T) {
		const src = `
[[job]]
id = "j_simple"
prompt = "x"
strategy = "simple"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate()).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Value(t, j.Strategy).Equal(model.JobStrategySimple)
	})

	t.Run("planexec", func(t *testing.T) {
		const src = `
[[job]]
id = "j_pe"
prompt = "x"
strategy = "planexec"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate()).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Value(t, j.Strategy).Equal(model.JobStrategyPlanexec)
	})

	t.Run("empty falls back to simple", func(t *testing.T) {
		const src = `
[[job]]
id = "j_default"
prompt = "x"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate()).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Value(t, j.Strategy).Equal(model.JobStrategySimple)
	})

	t.Run("unknown is rejected", func(t *testing.T) {
		const src = `
[[job]]
id = "j_bad"
prompt = "x"
strategy = "ultra"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		// AppConfig.Validate runs resolveJobs which calls JobSection.Validate.
		gt.Error(t, app.Validate())
	})
}

func TestJobSection_Interactive(t *testing.T) {
	t.Run("interactive with planexec parses", func(t *testing.T) {
		const src = `
[[job]]
id = "j_interactive"
prompt = "x"
strategy = "planexec"
interactive = true
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate()).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Bool(t, j.Interactive).True()
		gt.Value(t, j.Strategy).Equal(model.JobStrategyPlanexec)
	})

	t.Run("interactive defaults to false", func(t *testing.T) {
		const src = `
[[job]]
id = "j_default"
prompt = "x"
strategy = "planexec"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate()).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Bool(t, j.Interactive).False()
	})

	t.Run("interactive with simple is rejected at load", func(t *testing.T) {
		const src = `
[[job]]
id = "j_bad_interactive"
prompt = "x"
strategy = "simple"
interactive = true
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Error(t, app.Validate())
	})

	t.Run("interactive without strategy (defaults to simple) is rejected", func(t *testing.T) {
		const src = `
[[job]]
id = "j_bad_default"
prompt = "x"
interactive = true
events.scheduled = { every = "1h" }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Error(t, app.Validate())
	})
}

func TestJobSection_Validate_Errors(t *testing.T) {
	cases := []struct {
		name string
		toml string
	}{
		{
			name: "missing id",
			toml: `[[job]]
prompt = "x"
events.case = { on = ["created"] }
`,
		},
		{
			name: "id not snake_case (uppercase)",
			toml: `[[job]]
id = "Bad_ID"
prompt = "x"
events.case = { on = ["created"] }
`,
		},
		{
			name: "id is kebab-case (rejected)",
			toml: `[[job]]
id = "bad-id"
prompt = "x"
events.case = { on = ["created"] }
`,
		},
		{
			name: "missing prompt",
			toml: `[[job]]
id = "ok"
events.case = { on = ["created"] }
`,
		},
		{
			name: "missing events entirely",
			toml: `[[job]]
id = "ok"
prompt = "x"
`,
		},
		{
			name: "empty case on",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.case = { on = [] }
`,
		},
		{
			name: "unknown case lifecycle",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.case = { on = ["updated"] }
`,
		},
		{
			name: "duplicate case lifecycle",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.case = { on = ["created", "created"] }
`,
		},
		{
			name: "scheduled both every and cron",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.scheduled = { every = "1h", cron = "0 9 * * *" }
`,
		},
		{
			name: "scheduled neither every nor cron",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.scheduled = {}
`,
		},
		{
			name: "scheduled bad every",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.scheduled = { every = "nonsense" }
`,
		},
		{
			name: "scheduled zero every",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.scheduled = { every = "0s" }
`,
		},
		{
			name: "scheduled bad cron",
			toml: `[[job]]
id = "ok"
prompt = "x"
events.scheduled = { cron = "not a cron" }
`,
		},
		{
			name: "duplicate job id",
			toml: `[[job]]
id = "dup"
prompt = "a"
events.case = { on = ["created"] }

[[job]]
id = "dup"
prompt = "b"
events.case = { on = ["closed"] }
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var app config.AppConfig
			gt.NoError(t, toml.Unmarshal([]byte(tc.toml), &app)).Required()
			gt.Error(t, app.Validate())
		})
	}
}

func TestLoadWorkspaceConfigs_SampleRiskTOML(t *testing.T) {
	// Sanity: the bundled examples/workspaces/risk.toml must load and
	// validate so the sample stays in sync with the schema.
	configs, err := config.LoadWorkspaceConfigs([]string{"../../../examples/workspaces/risk.toml"})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()
	gt.Array(t, configs[0].Jobs).Length(3) // summarize_on_create / post_close_retro / stale_check
}

func TestJobSection_SingleStringOnIsRejected(t *testing.T) {
	const src = `[[job]]
id = "single"
prompt = "x"
events.case = { on = "created" }
`
	var app config.AppConfig
	// TOML unmarshal into []string field rejects scalar — accept either
	// the unmarshal error or the downstream validate error. The point is
	// the schema does not silently coerce.
	if err := toml.Unmarshal([]byte(src), &app); err == nil {
		gt.Error(t, app.Validate())
	}
}

func TestJobSection_PromptFile(t *testing.T) {
	parseJob := func(t *testing.T, src string) *config.JobSection {
		t.Helper()
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Array(t, app.Jobs).Length(1).Required()
		return &app.Jobs[0]
	}

	t.Run("reads prompt from file relative to baseDir", func(t *testing.T) {
		dir := t.TempDir()
		// A trailing newline (what editors add) must be trimmed so the
		// resolved prompt matches an equivalent inline value.
		gt.NoError(t, os.WriteFile(filepath.Join(dir, "summary.md"),
			[]byte("summarize: {{.Case.Title}}\n"), 0600)).Required()

		job := parseJob(t, `
[[job]]
id = "summary_job"
prompt_file = "summary.md"
events.case = { on = ["created"] }
`)
		resolved, err := job.Validate(dir)
		gt.NoError(t, err).Required()
		gt.Value(t, resolved.ID).Equal("summary_job")
		gt.String(t, resolved.Prompt).Equal("summarize: {{.Case.Title}}")
	})

	t.Run("reads prompt from nested relative path", func(t *testing.T) {
		dir := t.TempDir()
		gt.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0750)).Required()
		gt.NoError(t, os.WriteFile(filepath.Join(dir, "prompts", "nested.md"),
			[]byte("nested body"), 0600)).Required()

		job := parseJob(t, `
[[job]]
id = "nested_job"
prompt_file = "prompts/nested.md"
events.case = { on = ["created"] }
`)
		resolved, err := job.Validate(dir)
		gt.NoError(t, err).Required()
		gt.String(t, resolved.Prompt).Equal("nested body")
	})

	t.Run("absolute prompt_file is honoured as-is", func(t *testing.T) {
		dir := t.TempDir()
		absPath := filepath.Join(dir, "abs.md")
		gt.NoError(t, os.WriteFile(absPath, []byte("absolute body"), 0600)).Required()

		// baseDir is a different, unrelated directory: an absolute path must
		// not be joined against it.
		job := parseJob(t, `
[[job]]
id = "abs_job"
prompt_file = "`+absPath+`"
events.case = { on = ["created"] }
`)
		resolved, err := job.Validate(t.TempDir())
		gt.NoError(t, err).Required()
		gt.String(t, resolved.Prompt).Equal("absolute body")
	})

	t.Run("prompt and prompt_file are mutually exclusive", func(t *testing.T) {
		dir := t.TempDir()
		gt.NoError(t, os.WriteFile(filepath.Join(dir, "p.md"), []byte("body"), 0600)).Required()
		job := parseJob(t, `
[[job]]
id = "both_job"
prompt = "inline"
prompt_file = "p.md"
events.case = { on = ["created"] }
`)
		_, err := job.Validate(dir)
		gt.Error(t, err)
	})

	t.Run("neither prompt nor prompt_file is rejected", func(t *testing.T) {
		job := parseJob(t, `
[[job]]
id = "empty_job"
events.case = { on = ["created"] }
`)
		_, err := job.Validate("")
		gt.Error(t, err)
	})

	t.Run("missing prompt file fails", func(t *testing.T) {
		job := parseJob(t, `
[[job]]
id = "missing_job"
prompt_file = "does_not_exist.md"
events.case = { on = ["created"] }
`)
		_, err := job.Validate(t.TempDir())
		gt.Error(t, err)
	})

	t.Run("whitespace-only prompt file fails", func(t *testing.T) {
		dir := t.TempDir()
		gt.NoError(t, os.WriteFile(filepath.Join(dir, "blank.md"),
			[]byte("  \n\t\n"), 0600)).Required()
		job := parseJob(t, `
[[job]]
id = "blank_job"
prompt_file = "blank.md"
events.case = { on = ["created"] }
`)
		_, err := job.Validate(dir)
		gt.Error(t, err)
	})

	t.Run("structural mode accepts prompt_file without reading it", func(t *testing.T) {
		// baseDir == "" means AppConfig.Validate's structural pass, which must
		// not touch the filesystem. A prompt_file reference is accepted and the
		// prompt stays unresolved (empty) until the real load reads it.
		const src = `
[[job]]
id = "deferred_job"
prompt_file = "anywhere.md"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.NoError(t, app.Validate())

		resolved, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Value(t, resolved.ID).Equal("deferred_job")
		gt.String(t, resolved.Prompt).Equal("")
	})
}

func TestJobSection_Reflection(t *testing.T) {
	t.Run("reflection=true is propagated to model.Job", func(t *testing.T) {
		const src = `
[[job]]
id = "reflect_on"
prompt = "x"
reflection = true
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Array(t, app.Jobs).Length(1).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Bool(t, j.Reflection).True()
	})

	t.Run("reflection omitted defaults to false", func(t *testing.T) {
		const src = `
[[job]]
id = "reflect_default"
prompt = "x"
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Array(t, app.Jobs).Length(1).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Bool(t, j.Reflection).False()
	})

	t.Run("reflection=false is propagated to model.Job", func(t *testing.T) {
		const src = `
[[job]]
id = "reflect_off"
prompt = "x"
reflection = false
events.case = { on = ["created"] }
`
		var app config.AppConfig
		gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
		gt.Array(t, app.Jobs).Length(1).Required()
		j, err := app.Jobs[0].Validate("")
		gt.NoError(t, err).Required()
		gt.Bool(t, j.Reflection).False()
	})
}

func TestLoadWorkspaceConfigs_PromptFileRelativeToConfig(t *testing.T) {
	// End-to-end: prompt_file must resolve relative to the config file's own
	// directory, not the process working directory.
	dir := t.TempDir()
	gt.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0750)).Required()
	gt.NoError(t, os.WriteFile(filepath.Join(dir, "prompts", "job.md"),
		[]byte("do the thing: {{.Case.Title}}\n"), 0600)).Required()

	const cfg = `
[workspace]
id = "ws-promptfile"
name = "PromptFile WS"

[[job]]
id = "file_job"
prompt_file = "prompts/job.md"
events.case = { on = ["created"] }
`
	cfgPath := filepath.Join(dir, "config.toml")
	gt.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{cfgPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()
	gt.Array(t, configs[0].Jobs).Length(1).Required()
	gt.Value(t, configs[0].Jobs[0].ID).Equal("file_job")
	gt.String(t, configs[0].Jobs[0].Prompt).Equal("do the thing: {{.Case.Title}}")
}
