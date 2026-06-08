// Package eval is the entry point of the offline eval harness: it loads
// scenario files, validates them (dry-run), runs each through its workflow
// driver, judges the produced artifact against the scenario checklist, dumps
// diagnostics for failing scenarios, and renders the aggregated report.
//
// The final OK/NG decision is left to a human reviewer: the harness never gates
// on check verdicts. The process exits non-zero only on execution errors.
package eval

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"

	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/driver"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/judge"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/llmrun"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/report"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/toolsim"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/usersim"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// Exit codes.
const (
	ExitOK    = 0
	ExitError = 2
)

// toolCatalogEntry describes one tool usable in a scenario.
type toolCatalogEntry struct {
	Name        string
	Description string
	ReadOnly    bool
	Simulatable bool
}

// ToolCatalog is the single source of truth for valid tool names (FR-16). The
// scenario validator and `--list-tools` both read from here.
func ToolCatalog() []toolCatalogEntry {
	return []toolCatalogEntry{
		{Name: toolsim.ToolSlackSearch, Description: "Search Slack messages", ReadOnly: true, Simulatable: true},
		{Name: toolsim.ToolNotionSearch, Description: "Search Notion pages", ReadOnly: true, Simulatable: true},
		{Name: toolsim.ToolGitHubSearch, Description: "Search GitHub issues/PRs (live-only in v1)", ReadOnly: true, Simulatable: false},
	}
}

func toolNames() []string {
	cat := ToolCatalog()
	out := make([]string, 0, len(cat))
	for _, e := range cat {
		out = append(out, e.Name)
	}
	return out
}

// Config carries the runner inputs assembled by the CLI.
type Config struct {
	// LLM is shared by all roles (agent, judge, simulators) — FR-7.
	LLM         gollem.LLMClient
	Concurrency int
	DryRun      bool
	DumpDir     string
	DumpAll     bool
	Language    string // output language for judge reasons / analysis.md (FR-14)
	ReportPath  string
	Quiet       bool
	Verbose     bool

	// Live tool clients, used only for tools marked live=true.
	LiveSlackSearch slacktool.SearchService
	LiveNotion      notiontool.Client
	GitHub          *githubtool.Client
}

// Run executes the harness over the given paths and writes the summary to
// stdout. It returns the process exit code.
func Run(ctx context.Context, paths []string, cfg Config, stdout io.Writer) (int, error) {
	logger := logging.From(ctx)

	files, err := collectScenarioFiles(paths)
	if err != nil {
		return ExitError, err
	}
	if len(files) == 0 {
		return ExitError, goerr.New("no scenario .toml files found", goerr.V("paths", paths))
	}

	registry := driver.Default()
	validateOpts := scenario.ValidateOptions{KnownWorkflows: registry.Kinds(), KnownTools: toolNames()}

	scenarios := make([]*scenario.Scenario, 0, len(files))
	for _, f := range files {
		sc, err := scenario.Load(f)
		if err != nil {
			return ExitError, goerr.Wrap(err, "load scenario", goerr.V("file", f))
		}
		if err := sc.Validate(validateOpts); err != nil {
			return ExitError, goerr.Wrap(err, "validate scenario", goerr.V("file", f))
		}
		scenarios = append(scenarios, sc)
	}

	if cfg.DryRun {
		logger.Info("eval dry-run: all scenarios valid", "count", len(scenarios))
		for _, sc := range scenarios {
			_, _ = io.WriteString(stdout, "ok  "+sc.Meta.ID+"  ["+sc.Meta.Workflow+"]\n")
		}
		return ExitOK, nil
	}

	if cfg.LLM == nil {
		return ExitError, goerr.New("LLM client is required to run scenarios (configure --llm-provider)")
	}

	logger.Info("eval suite starting", "scenarios", len(scenarios), "concurrency", cfg.Concurrency)
	results := runAll(ctx, scenarios, registry, cfg)

	if err := report.Summary(stdout, results, report.SummaryOptions{Quiet: cfg.Quiet, Verbose: cfg.Verbose}); err != nil {
		return ExitError, err
	}
	if cfg.ReportPath != "" {
		if err := report.WriteJSON(cfg.ReportPath, results); err != nil {
			return ExitError, err
		}
	}

	for _, r := range results {
		if r.Status == evaltype.StatusError {
			return ExitError, nil
		}
	}
	return ExitOK, nil
}

func runAll(ctx context.Context, scenarios []*scenario.Scenario, registry *driver.Registry, cfg Config) []evaltype.ScenarioResult {
	concurrency := max(cfg.Concurrency, 1)
	results := make([]evaltype.ScenarioResult, len(scenarios))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range scenarios {
		// Stop acquiring slots once the context is cancelled (e.g. Ctrl-C) instead
		// of blocking on a full semaphore or spawning more work; mark the
		// remaining scenarios as errored so the report is complete.
		select {
		case <-ctx.Done():
			results[i] = evaltype.ScenarioResult{
				ScenarioID: scenarios[i].Meta.ID,
				Workflow:   scenarios[i].Meta.Workflow,
				Status:     evaltype.StatusError,
				Err:        ctx.Err().Error(),
			}
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = runOne(ctx, scenarios[idx], registry, cfg)
		}(i)
	}
	wg.Wait()
	return results
}

func runOne(ctx context.Context, sc *scenario.Scenario, registry *driver.Registry, cfg Config) evaltype.ScenarioResult {
	logger := logging.From(ctx)
	evalID := newEvalID()
	res := evaltype.ScenarioResult{
		ScenarioID: sc.Meta.ID,
		EvalID:     evalID,
		Workflow:   sc.Meta.Workflow,
		Status:     evaltype.StatusOK,
	}

	completer := llmrun.New(cfg.LLM)
	e, err := env.Build(ctx, sc, env.Options{
		LLM:             cfg.LLM,
		Completer:       completer,
		LiveSlackSearch: cfg.LiveSlackSearch,
		LiveNotion:      cfg.LiveNotion,
		GitHub:          cfg.GitHub,
	})
	if err != nil {
		return errorResult(res, err)
	}

	d, ok := registry.Lookup(sc.Meta.Workflow)
	if !ok {
		return errorResult(res, goerr.New("no driver for workflow", goerr.V("workflow", sc.Meta.Workflow)))
	}

	sim := usersim.New(completer, sc.Persona, sc.Meta.Language)
	logger.Info("eval scenario starting", "scenario", sc.Meta.ID, "eval_id", evalID)
	art, err := d.Run(ctx, e, sc, sim)
	if err != nil {
		return errorResult(res, goerr.Wrap(err, "run workflow"))
	}

	j := judge.New(completer, cfg.Language)
	verdicts, err := j.Evaluate(ctx, art, sc.Expect.Checks)
	if err != nil {
		return errorResult(res, goerr.Wrap(err, "judge"))
	}

	res.Artifact = art
	res.Checks = verdicts
	res.Score = report.ComputeScore(verdicts)

	if cfg.DumpDir != "" && (cfg.DumpAll || hasFailure(verdicts)) {
		dir, derr := report.Dump(cfg.DumpDir, res, e.Trace.Snapshot(), cfg.Language)
		if derr != nil {
			logger.Warn("eval: dump failed", "scenario", sc.Meta.ID, "error", derr.Error())
		} else {
			res.DumpDir = dir
		}
	}

	logger.Info("eval scenario done", "scenario", sc.Meta.ID, "score_passed", res.Score.Passed, "score_total", res.Score.Total)
	return res
}

func errorResult(res evaltype.ScenarioResult, err error) evaltype.ScenarioResult {
	res.Status = evaltype.StatusError
	res.Err = err.Error()
	return res
}

func hasFailure(verdicts []evaltype.CheckVerdict) bool {
	for _, v := range verdicts {
		if !v.Passed {
			return true
		}
	}
	return false
}

func newEvalID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}

// collectScenarioFiles expands file and directory paths into a sorted list of
// .toml files.
func collectScenarioFiles(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, goerr.Wrap(err, "stat path", goerr.V("path", p))
		}
		if info.IsDir() {
			walkErr := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(d.Name(), ".toml") {
					files = append(files, path)
				}
				return nil
			})
			if walkErr != nil {
				return nil, goerr.Wrap(walkErr, "walk dir", goerr.V("path", p))
			}
		} else {
			files = append(files, p)
		}
	}
	return files, nil
}
