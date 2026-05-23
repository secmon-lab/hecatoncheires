package config_test

import (
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
id = "summarize-on-create"
name = "Summarize"
description = "Auto-summarize new cases"
prompt = "summarize: {{.Case.Title}}"
events.case = { on = ["created"] }

[[job]]
id = "daily-digest"
prompt = "post daily digest"
events.scheduled = { cron = "0 9 * * *" }

[[job]]
id = "stale-check"
prompt = "remind stale"
events.scheduled = { every = "1h" }

[[job]]
id = "lifecycle-and-stale-watcher"
prompt = "do both"
disabled = true
events.case = { on = ["created", "closed"] }
events.scheduled = { every = "30m" }
`
	var app config.AppConfig
	gt.NoError(t, toml.Unmarshal([]byte(src), &app)).Required()
	gt.NoError(t, app.Validate()).Required()

	gt.Array(t, app.Jobs).Length(4).Required()

	first, err := app.Jobs[0].Validate()
	gt.NoError(t, err).Required()
	gt.Value(t, first.ID).Equal("summarize-on-create")
	gt.Value(t, first.Events.Case).NotNil()
	gt.Array(t, first.Events.Case.On).Length(1)
	gt.Value(t, first.Events.Case.On[0]).Equal(model.CaseLifecycleCreated)
	gt.Value(t, first.Events.Scheduled).Nil()

	second, err := app.Jobs[1].Validate()
	gt.NoError(t, err).Required()
	gt.Value(t, second.Events.Scheduled).NotNil()
	gt.Value(t, second.Events.Scheduled.CronExpr).Equal("0 9 * * *")
	gt.Number(t, int64(second.Events.Scheduled.Every)).Equal(0)
	gt.Value(t, second.Events.Scheduled.Cron).NotNil()

	third, err := app.Jobs[2].Validate()
	gt.NoError(t, err).Required()
	gt.Number(t, int64(third.Events.Scheduled.Every)).Equal(int64(time.Hour))

	fourth, err := app.Jobs[3].Validate()
	gt.NoError(t, err).Required()
	gt.Bool(t, fourth.Disabled).True()
	gt.Array(t, fourth.Events.Case.On).Length(2)
	gt.Value(t, fourth.Events.Case.On[0]).Equal(model.CaseLifecycleCreated)
	gt.Value(t, fourth.Events.Case.On[1]).Equal(model.CaseLifecycleClosed)
	gt.Number(t, int64(fourth.Events.Scheduled.Every)).Equal(int64(30 * time.Minute))
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
			name: "id not kebab-case",
			toml: `[[job]]
id = "Bad_ID"
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
	gt.Array(t, configs[0].Jobs).Length(3) // summarize-on-create / post-close-retro / stale-check
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
