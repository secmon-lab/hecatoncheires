package usecase

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

//go:embed prompts/assist_system.md
var assistSystemPromptTmpl string

var assistSystemPrompt = template.Must(template.New("assist_system").Parse(assistSystemPromptTmpl))

// AssistOption holds options for the assist command
type AssistOption struct {
	WorkspaceID  string
	LogCount     int
	MessageCount int
}

// assistAgentVersion is the strategy state version stamped on every assist
// Process. Bump it only alongside a DecodeState that still reads the older shape.
const assistAgentVersion = 1

// AssistUseCase handles periodic AI-powered case assistance
type AssistUseCase struct {
	deps AssistDeps

	// agent and kernel are filled by Register and Bind. They cannot be
	// constructor arguments: registering the agent needs this UseCase as its
	// completion handler, and building the Kernel needs the registry that
	// registration fills.
	agent  agentkit.Agent[react.Input]
	kernel *agentkit.Kernel

	// serveOpts tunes the worker drain runs. Production leaves it empty and
	// takes agentkit's defaults; tests shorten the retry schedule so a
	// deliberately failing run does not spend its whole backoff.
	serveOpts []agentkit.ServeOption
}

// AssistDeps groups the dependencies AssistUseCase needs. All three are
// required.
//
// It carries no tool clients. The assist agent's tools are built per claim by
// the agent runtime's tool factory from agentkernel.ToolDeps, which the CLI
// assembles; listing them here as well would leave two wirings to keep in sync,
// and the one this usecase held would be the dead one.
type AssistDeps struct {
	Repo     interfaces.Repository
	Registry *model.WorkspaceRegistry
	// LLM generates the structured summary each finished run is logged as. The
	// run itself uses the Kernel's model.
	LLM gollem.LLMClient
}

// NewAssistUseCase creates a new AssistUseCase from a deps bundle. See AssistDeps.
func NewAssistUseCase(deps AssistDeps) *AssistUseCase {
	return &AssistUseCase{deps: deps}
}

// Register registers the assist agent and wires this UseCase as its completion
// handler. Call it before building the Kernel, and Bind after.
func (uc *AssistUseCase) Register(reg *agentkit.Registry, limiter agentkit.Limiter, store agentkit.HistoryStore) error {
	handle, err := react.Register(reg, agentkernel.AgentAssist, assistAgentVersion, limiter,
		agentkit.WithHistoryStore[react.Output](store),
		agentkit.WithOnFinish(uc.onFinish),
	)
	if err != nil {
		return goerr.Wrap(err, "register the assist agent")
	}
	uc.agent = handle
	return nil
}

// Bind hands over the Kernel the registered agent runs on.
func (uc *AssistUseCase) Bind(k *agentkit.Kernel) { uc.kernel = k }

// RunAssist iterates all workspaces and open cases, running the assist agent
// for each.
//
// It spawns one durable Process per case and then drives the worker in the
// foreground until every one of them has finished, because assist is a batch
// command: the command's exit is what tells the operator (or the scheduler that
// invoked it) that the pass is over.
func (uc *AssistUseCase) RunAssist(ctx context.Context, opts AssistOption) error {
	logger := logging.From(ctx)

	if err := uc.ready(); err != nil {
		return err
	}

	if opts.LogCount <= 0 {
		opts.LogCount = 7
	}
	if opts.MessageCount <= 0 {
		opts.MessageCount = 50
	}

	entries := uc.deps.Registry.List()

	// Filter by workspace if specified
	if opts.WorkspaceID != "" {
		entry, err := uc.deps.Registry.Get(opts.WorkspaceID)
		if err != nil {
			return goerr.Wrap(err, "specified workspace not found",
				goerr.V("workspaceID", opts.WorkspaceID),
			)
		}
		entries = []*model.WorkspaceEntry{entry}
	}

	var spawned []agentkit.ProcessID
	for _, entry := range entries {
		wsID := entry.Workspace.ID
		if entry.AssistPrompt == "" {
			logger.Info("skipping workspace without [assist] config", "workspaceID", wsID)
			continue
		}

		pids, err := uc.processWorkspace(ctx, entry, opts)
		spawned = append(spawned, pids...)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to process workspace", goerr.V("workspaceID", wsID)), "failed to process workspace")
		}
	}

	return uc.drain(ctx, spawned)
}

// ready reports whether the agent runtime has been wired.
func (uc *AssistUseCase) ready() error {
	if uc.kernel == nil || uc.agent.Name() == "" {
		return goerr.New("assist usecase is not bound to an agent runtime")
	}
	return nil
}

// processWorkspace spawns one run per open case and returns their ids. A case
// that cannot be prepared is reported and skipped, so one bad case does not
// cost the rest of the workspace its pass.
func (uc *AssistUseCase) processWorkspace(ctx context.Context, entry *model.WorkspaceEntry, opts AssistOption) ([]agentkit.ProcessID, error) {
	logger := logging.From(ctx)
	wsID := entry.Workspace.ID

	openStatus := types.CaseStatusOpen
	cases, err := uc.deps.Repo.Case().List(ctx, wsID, interfaces.WithStatus(openStatus))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list open cases",
			goerr.V("workspaceID", wsID),
		)
	}

	logger.Info("processing workspace", "workspaceID", wsID, "openCases", len(cases))

	pids := make([]agentkit.ProcessID, 0, len(cases))
	for _, c := range cases {
		pid, err := uc.processCase(ctx, entry, c, opts)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to process case",
				goerr.V("workspaceID", wsID),
				goerr.V("caseID", c.ID),
				goerr.V("caseTitle", c.Title),
			), "failed to process case")
			// Continue processing remaining cases
			continue
		}
		pids = append(pids, pid)
	}

	return pids, nil
}

// processCase spawns one assist run for one case. It returns as soon as the run
// is recorded; the agent worker drives it and onFinish writes its log.
func (uc *AssistUseCase) processCase(ctx context.Context, entry *model.WorkspaceEntry, c *model.Case, opts AssistOption) (agentkit.ProcessID, error) {
	logger := logging.From(ctx)
	wsID := entry.Workspace.ID

	logger.Info("processing case", "workspaceID", wsID, "caseID", c.ID, "caseTitle", c.Title)

	systemPrompt, err := uc.buildAssistSystemPrompt(ctx, entry, c, opts)
	if err != nil {
		return "", goerr.Wrap(err, "failed to build system prompt")
	}

	// The tools are no longer assembled here: the agent runtime's tool factory
	// builds the assist palette from this scope on every claim (see
	// agent.KnownToolSetIDsAssist). The Slack posting tool's channel comes from
	// the case the run is pinned to.
	scope := agentkernel.Scope{
		WorkspaceID: wsID,
		CaseID:      c.ID,
		ToolSets:    []string{agentkernel.ToolSetsAll},
		PrivateCase: c.IsPrivate,
	}
	if err := agentkernel.ValidateSpawn(agentkernel.AgentAssist, scope); err != nil {
		return "", goerr.Wrap(err, "validate the assist scope", goerr.V("caseID", c.ID))
	}

	// No subject and no idempotency key: assist takes no per-case lock today, and
	// two concurrent passes over the same case already both run. Adding a lock
	// here would change behaviour beyond moving the runtime.
	pid, err := uc.agent.Spawn(ctx, uc.kernel,
		react.Input{SystemPrompt: systemPrompt, Prompt: entry.AssistPrompt},
		agentkit.WithMetadata(scope.Metadata()),
	)
	if err != nil {
		return "", goerr.Wrap(err, "spawn the assist agent", goerr.V("caseID", c.ID))
	}
	return pid, nil
}

// drain runs the agent worker in the foreground until every spawned run has
// reached a terminal state.
//
// The command owns this loop because it is a batch pass: nothing else would ever
// execute these Processes. The kernel it drives is built on an in-process store
// (see cmdAssist), so a run cannot be claimed by a serve instance that has no
// assist agent registered — which would fail it as an unknown agent.
func (uc *AssistUseCase) drain(ctx context.Context, pids []agentkit.ProcessID) error {
	if len(pids) == 0 {
		return nil
	}
	logger := logging.From(ctx)
	logger.Info("draining assist runs", "count", len(pids))

	serveCtx, stop := context.WithCancel(ctx)
	defer stop()
	served := make(chan error, 1)
	async.DispatchCancelable(serveCtx, func(c context.Context) error {
		err := agentkernel.Serve(c, uc.kernel, uc.serveOpts...)
		served <- err
		// Reported to the caller through `served`, not returned: the normal exit
		// here is the cancellation drain performs itself, and returning that would
		// have the async helper report every assist pass as a failure.
		return nil
	})

	pending := make(map[agentkit.ProcessID]bool, len(pids))
	for _, pid := range pids {
		pending[pid] = true
	}
	failed := 0
	for len(pending) > 0 {
		for pid := range pending {
			proc, err := uc.kernel.GetProcess(ctx, pid)
			if err != nil {
				// A run whose row cannot be read will never be observed to
				// finish, so stop waiting on it rather than spinning forever.
				errutil.Handle(ctx, goerr.Wrap(err, "read an assist run",
					goerr.V("process", pid)), "read an assist run")
				delete(pending, pid)
				failed++
				continue
			}
			if proc.Status.Terminal() {
				delete(pending, pid)
				if uc.reportIfUnsuccessful(ctx, proc) {
					failed++
				}
			}
		}
		if len(pending) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			stop()
			<-served
			return goerr.Wrap(ctx.Err(), "assist pass interrupted while runs were still going",
				goerr.V("unfinished", len(pending)))
		case <-time.After(assistDrainPollInterval):
		}
	}

	stop()
	// Serve returns the context error it stopped on; that is the expected exit
	// here, not a failure of the pass.
	if err := <-served; err != nil && !errors.Is(err, context.Canceled) {
		return goerr.Wrap(err, "run the assist agent worker")
	}
	logger.Info("assist runs finished", "count", len(pids), "failed", failed)
	return nil
}

// reportIfUnsuccessful reports a run that did not succeed and says whether it
// counted as a failure.
//
// The pass itself still succeeds: assist walks every open case, and one case the
// agent could not get through must not cost the rest of the workspace its pass —
// which is what the pre-agentkit loop did by routing each case's error through
// errutil.Handle. What must NOT happen is the failure going unmentioned, leaving
// the command to report a clean pass over runs that produced nothing.
func (uc *AssistUseCase) reportIfUnsuccessful(ctx context.Context, proc *agentkit.Process) bool {
	switch proc.Status {
	case agentkit.ProcessSucceeded:
		return false
	case agentkit.ProcessFailed:
		sc := agentkernel.ScopeFrom(proc.Metadata)
		err := goerr.New("assist run failed",
			goerr.V("process", proc.ID),
			goerr.V("workspace_id", sc.WorkspaceID),
			goerr.V("case_id", sc.CaseID))
		if proc.Failure != nil {
			err = goerr.With(err,
				goerr.V("failure_code", string(proc.Failure.Code)),
				goerr.V("failure_message", proc.Failure.Message))
		}
		errutil.Handle(ctx, err, "assist run failed")
		return true
	default:
		// Cancelled, or any terminal status added later: someone or something
		// stopped this run deliberately, so it is reported without a stack.
		sc := agentkernel.ScopeFrom(proc.Metadata)
		errutil.Handle(ctx, goerr.New("assist run did not complete",
			goerr.V("process", proc.ID),
			goerr.V("status", string(proc.Status)),
			goerr.V("workspace_id", sc.WorkspaceID),
			goerr.V("case_id", sc.CaseID),
			goerr.T(errutil.TagBenign)), "assist run did not complete")
		return true
	}
}

// assistDrainPollInterval is how often drain re-reads the runs it is waiting on.
// It bounds only the delay between the last run finishing and the command
// exiting, so a coarse value costs nothing.
const assistDrainPollInterval = 200 * time.Millisecond

// onFinish writes the run's assist log. agentkit calls it once, after the
// terminal transition committed.
func (uc *AssistUseCase) onFinish(ctx context.Context, pid agentkit.ProcessID, res agentkit.FinishResult[react.Output]) error {
	if res.Status != agentkit.ProcessSucceeded || res.Output == nil {
		// A failed or cancelled run has nothing to summarise — a log records what
		// a pass concluded, and this one concluded nothing. drain reports the
		// failure itself (reportIfUnsuccessful), reading it off the Process rather
		// than from here, so a lost completion callback cannot swallow it.
		return nil
	}
	proc, err := uc.kernel.GetProcess(ctx, pid)
	if err != nil {
		return goerr.Wrap(err, "read the finished assist run", goerr.V("process", pid))
	}
	sc := agentkernel.ScopeFrom(proc.Metadata)

	language := ""
	if entry, err := uc.deps.Registry.Get(sc.WorkspaceID); err == nil && entry != nil {
		language = entry.AssistLanguage
	}

	if err := uc.saveAssistLog(ctx, sc.WorkspaceID, sc.CaseID, language, res.Output.Text()); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to save assist log",
			goerr.V("workspaceID", sc.WorkspaceID),
			goerr.V("caseID", sc.CaseID),
		), "failed to save assist log")
	}
	return nil
}

// assistPromptMessage represents a message for the assist system prompt template
type assistPromptMessage struct {
	Timestamp   string
	ThreadTS    string
	DisplayName string
	Text        string
}

// assistPromptAction represents an action for the assist system prompt template
type assistPromptAction struct {
	ID          int64
	Title       string
	Status      string
	StatusEmoji string
	Assignees   string
	DueDate     string
}

// assistPromptAssistLog represents a previous assist log for the template
type assistPromptAssistLog struct {
	CreatedAt string
	Summary   string
	Actions   string
	Reasoning string
	NextSteps string
}

// promptField represents a case field for template rendering.
type promptField struct {
	Name  string
	Value any
}

// assistPromptData holds all data for the assist system prompt template
type assistPromptData struct {
	CurrentTime  string
	Case         *model.Case
	Fields       []promptField
	Actions      []assistPromptAction
	Messages     []assistPromptMessage
	AssistLogs   []assistPromptAssistLog
	AssistPrompt string
	Language     string
}

func (uc *AssistUseCase) buildAssistSystemPrompt(ctx context.Context, entry *model.WorkspaceEntry, c *model.Case, opts AssistOption) (string, error) {
	wsID := entry.Workspace.ID

	data := assistPromptData{
		CurrentTime:  time.Now().UTC().Format(time.RFC3339),
		Case:         c,
		AssistPrompt: entry.AssistPrompt,
		Language:     entry.AssistLanguage,
	}

	// Build field values
	if entry.FieldSchema != nil && len(c.FieldValues) > 0 {
		fieldNames := make(map[string]string)
		for _, fd := range entry.FieldSchema.Fields {
			fieldNames[fd.ID] = fd.Name
		}
		for fieldID, fv := range c.FieldValues {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			data.Fields = append(data.Fields, promptField{Name: name, Value: fv.Value})
		}
	}

	// Fetch actions (archived actions are intentionally excluded — the
	// assist prompt summarises the active state of a case)
	actions, err := uc.deps.Repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{})
	if err != nil {
		return "", goerr.Wrap(err, "failed to get actions for case")
	}
	statusSet := resolveActionStatusSet(uc.deps.Registry, wsID)
	for _, a := range actions {
		dueDate := ""
		if a.DueDate != nil {
			dueDate = a.DueDate.Format("2006-01-02")
		}
		data.Actions = append(data.Actions, assistPromptAction{
			ID:          a.ID,
			Title:       a.Title,
			Status:      a.Status.String(),
			StatusEmoji: statusSet.Emoji(string(a.Status)),
			Assignees:   a.AssigneeID,
			DueDate:     dueDate,
		})
	}

	// Fetch recent messages from CaseMessageRepository
	msgs, _, err := uc.deps.Repo.CaseMessage().List(ctx, wsID, c.ID, opts.MessageCount, "")
	if err != nil {
		return "", goerr.Wrap(err, "failed to get case messages")
	}
	// Messages are returned newest-first; reverse for oldest-first in prompt
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		displayName := m.UserName()
		if displayName == "" {
			displayName = m.UserID()
		}
		data.Messages = append(data.Messages, assistPromptMessage{
			Timestamp:   m.EventTS(),
			ThreadTS:    m.ThreadTS(),
			DisplayName: displayName,
			Text:        m.Text(),
		})
	}

	// Fetch recent assist logs
	assistLogs, _, err := uc.deps.Repo.AssistLog().List(ctx, wsID, c.ID, opts.LogCount, 0)
	if err != nil {
		return "", goerr.Wrap(err, "failed to get assist logs")
	}
	for _, l := range assistLogs {
		data.AssistLogs = append(data.AssistLogs, assistPromptAssistLog{
			CreatedAt: l.CreatedAt.Format(time.RFC3339),
			Summary:   l.Summary,
			Actions:   l.Actions,
			Reasoning: l.Reasoning,
			NextSteps: l.NextSteps,
		})
	}

	var buf bytes.Buffer
	if err := assistSystemPrompt.Execute(&buf, data); err != nil {
		return "", goerr.Wrap(err, "failed to execute assist system prompt template")
	}

	return buf.String(), nil
}

// assistLogSummary is the JSON structure for summarizing the agent session
type assistLogSummary struct {
	Summary   string `json:"summary"`
	Actions   string `json:"actions"`
	Reasoning string `json:"reasoning"`
	NextSteps string `json:"next_steps"`
}

// saveAssistLog summarises one finished assist run into an AssistLog.
//
// agentOutput is the run's final text. It arrives as an already-joined string
// rather than the runtime's own response type because the run is durable: the
// summary is produced after the terminal transition committed, from what the
// Process stored, not from a live agent handle.
func (uc *AssistUseCase) saveAssistLog(ctx context.Context, wsID string, caseID int64, language string, agentOutput string) error {
	// Create a new session with JSON response schema to generate structured summary
	schema := &gollem.Parameter{
		Title:       "AssistLogSummary",
		Description: "Structured summary of an assist agent session",
		Type:        gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"summary": {
				Type:        gollem.TypeString,
				Description: "One-line plain text summary of this session (no markdown). Keep it short and descriptive.",
				Required:    true,
			},
			"actions": {
				Type:        gollem.TypeString,
				Description: "Bulleted list of side-effecting actions taken in this session in markdown format. Only include actions that modified data, sent messages/mentions, or changed state. Do NOT include read-only or reference operations. Use '- ' prefix for each item. Empty string if no side-effecting actions were taken.",
				Required:    true,
			},
			"reasoning": {
				Type:        gollem.TypeString,
				Description: "Rationale behind decisions made in markdown format.",
				Required:    true,
			},
			"next_steps": {
				Type:        gollem.TypeString,
				Description: "Bulleted list of items to address in future sessions with clear action criteria for each in markdown format. Empty string if nothing to carry over.",
				Required:    true,
			},
		},
	}

	session, err := uc.deps.LLM.NewSession(ctx,
		gollem.WithSessionContentType(gollem.ContentTypeJSON),
		gollem.WithSessionResponseSchema(schema),
		gollem.WithSessionPromptCache(true),
	)
	if err != nil {
		return goerr.Wrap(err, "failed to create session for assist log summary")
	}

	languageInstruction := ""
	if language != "" {
		languageInstruction = fmt.Sprintf("\nYou MUST write all output in %s.\n", language)
	}

	prompt := fmt.Sprintf(`Summarize the following assist agent session output into four parts.
All output for actions, reasoning, and next_steps MUST be in markdown format.
%s
1. summary: A single-line plain text summary of the session. No markdown.
2. actions: Bulleted list of side-effecting actions only (data changes, messages sent, mentions, state modifications). Do NOT include read-only or reference operations (e.g. reading data, checking status). Use "- " prefix. If no side-effecting actions were taken, return an empty string "".
3. reasoning: Why these actions were taken.
4. next_steps: Bulleted list of items to carry over to future sessions. Each item MUST include a clear action criteria (what condition triggers the action). If there is nothing to carry over, return an empty string "".

Agent output:
%s`, languageInstruction, agentOutput)

	summaryResp, err := session.Generate(ctx, []gollem.Input{gollem.Text(prompt)})
	if err != nil {
		return goerr.Wrap(err, "failed to generate assist log summary")
	}

	if len(summaryResp.Texts) == 0 {
		return fmt.Errorf("assist log summary generation returned empty result")
	}

	var summary assistLogSummary
	if err := json.Unmarshal([]byte(summaryResp.Texts[0]), &summary); err != nil {
		return goerr.Wrap(err, "failed to parse assist log summary JSON",
			goerr.V("response", summaryResp.Texts[0]),
		)
	}

	log := &model.AssistLog{
		CaseID:    caseID,
		Summary:   summary.Summary,
		Actions:   summary.Actions,
		Reasoning: summary.Reasoning,
		NextSteps: summary.NextSteps,
		CreatedAt: time.Now().UTC(),
	}

	if _, err := uc.deps.Repo.AssistLog().Create(ctx, wsID, caseID, log); err != nil {
		return goerr.Wrap(err, "failed to save assist log")
	}

	return nil
}
