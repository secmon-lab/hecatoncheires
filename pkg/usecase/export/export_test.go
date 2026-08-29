package export_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	domainconfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/bqexport"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// dateFieldValue is a fixed date-typed custom field value stored as time.Time
// (one of the two valid stored forms) used to assert it is exported as a STRING.
var dateFieldValue = time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)

// fakeSink captures the tables handed to it, keyed by "namespace.tableName". It
// can be told to fail on specific table names to exercise error aggregation.
type fakeSink struct {
	tables map[string]*export.Table
	failOn map[string]bool
	closed bool
}

func newFakeSink() *fakeSink {
	return &fakeSink{tables: map[string]*export.Table{}, failOn: map[string]bool{}}
}

func (f *fakeSink) WriteTable(_ context.Context, namespace string, t *export.Table) error {
	if f.failOn[t.Name] {
		return errors.New("injected sink failure for " + t.Name)
	}
	f.tables[namespace+"."+t.Name] = t
	return nil
}

func (f *fakeSink) Close() error { f.closed = true; return nil }

func (f *fakeSink) table(namespace, name string) *export.Table { return f.tables[namespace+"."+name] }

// findRow returns the first row whose column equals want, or nil.
func findRow(t *export.Table, column string, want any) map[string]any {
	if t == nil {
		return nil
	}
	for _, r := range t.Rows {
		if r[column] == want {
			return r
		}
	}
	return nil
}

// findEventRow returns the job_run_events row for the given case and sequence.
// Sequence alone is not unique across the table (every run numbers its events
// from 1), so both keys are needed to address one row.
func findEventRow(t *export.Table, caseID, sequence int64) map[string]any {
	if t == nil {
		return nil
	}
	for _, r := range t.Rows {
		if r["case_id"] == caseID && r["sequence"] == sequence {
			return r
		}
	}
	return nil
}

// hasColumn reports whether the table declares a column with the given name.
func hasColumn(t *export.Table, name string) bool {
	if t == nil {
		return false
	}
	for _, c := range t.Columns {
		if c.Name == name {
			return true
		}
	}
	return false
}

// seedJobRun files one finished agent run against the given case: a SUCCESS
// JobRunLog carrying the run totals, the three-event timeline under it, and the
// JobRun summary doc that JobRun().ListByCase surfaces (the export reaches the
// logs through the summary, and the events through the logs).
func seedJobRun(t *testing.T, repo interfaces.Repository, wsID string, caseID int64, jobID string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: caseID, JobID: jobID}
	runID := "run-" + jobID
	traceID := "trace-" + jobID
	log := &model.JobRunLog{
		WorkspaceID:    wsID,
		CaseID:         caseID,
		JobID:          jobID,
		RunID:          runID,
		TraceID:        traceID,
		Stage:          model.JobRunStageRunning,
		StartedAt:      now,
		ExecutorKind:   model.ExecutorKindPlanexec,
		EventType:      "case",
		EventTriggerAt: now,
		SystemPrompt:   "you are the job agent",
	}
	gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()

	// One event of each of the three kinds a successful run produces, so the
	// export's per-kind column mapping is exercised end to end.
	baseEvent := func(seq int64, kind model.JobRunEventKind) *model.JobRunEvent {
		return &model.JobRunEvent{
			WorkspaceID: wsID, CaseID: caseID, JobID: jobID, RunID: runID, TraceID: traceID,
			// Real event ids are UUIDv7, i.e. unique across cases; keep that
			// property so a test can address one row unambiguously.
			EventID:    fmt.Sprintf("%s-c%d-ev-%d", jobID, caseID, seq),
			Sequence:   seq,
			OccurredAt: now.Add(time.Duration(seq) * time.Second),
			Kind:       kind,
			Phase:      "execute",
			AgentLabel: "investigator",
		}
	}
	req := baseEvent(1, model.JobRunEventKindLLMRequest)
	req.LLMRequest = &model.LLMRequestPayload{
		Model: "claude-opus-4-7",
		Messages: []model.LLMMessage{{
			Role:     "user",
			Contents: []model.LLMContentBlock{{Type: "text", Text: "investigate the case"}},
		}},
		Tools: []model.LLMToolSpec{{Name: "slack_search", Description: "search slack"}},
	}
	gt.NoError(t, repo.JobRunEvent().Append(ctx, req)).Required()

	resp := baseEvent(2, model.JobRunEventKindLLMResponse)
	resp.LLMResponse = &model.LLMResponsePayload{
		Model:                    "claude-opus-4-7",
		Texts:                    []string{"let me search"},
		FunctionCalls:            []model.LLMFunctionCall{{ID: "fc-1", Name: "slack_search", ArgumentsJSON: `{"q":"foo"}`}},
		InputTokens:              1200,
		OutputTokens:             180,
		CacheCreationInputTokens: 400,
		CacheReadInputTokens:     700,
		DurationMs:               1450,
	}
	gt.NoError(t, repo.JobRunEvent().Append(ctx, resp)).Required()

	tool := baseEvent(3, model.JobRunEventKindToolCall)
	tool.ParentSequence = 2
	tool.ToolCall = &model.ToolCallPayload{
		ToolName:      "slack_search",
		ArgumentsJSON: `{"q":"foo"}`,
		ResultJSON:    `{"hits":2}`,
		StartedAt:     now.Add(3 * time.Second),
		EndedAt:       now.Add(4 * time.Second),
	}
	gt.NoError(t, repo.JobRunEvent().Append(ctx, tool)).Required()

	// A response that carried no text and requested no tool. Its empty payload
	// fields must stay distinguishable from "nothing was recorded".
	empty := baseEvent(4, model.JobRunEventKindLLMResponse)
	empty.LLMResponse = &model.LLMResponsePayload{
		Model:         "claude-opus-4-7",
		Texts:         []string{},
		FunctionCalls: []model.LLMFunctionCall{},
		InputTokens:   90,
	}
	gt.NoError(t, repo.JobRunEvent().Append(ctx, empty)).Required()

	log.Stage = model.JobRunStageSuccess
	log.EndedAt = now.Add(time.Minute)
	log.InputTokens = 1500
	log.OutputTokens = 210
	log.CacheCreationInputTokens = 400
	log.CacheReadInputTokens = 700
	log.LLMCallCount = 4
	log.ToolCallCount = 6
	log.CostNanoUSD = 2_550_000
	log.Model = "gemini-3.7-flash"
	gt.NoError(t, repo.JobRunLog().Finish(ctx, log)).Required()
	gt.NoError(t, repo.JobRun().RecordRun(ctx, key,
		model.JobRunStatusSuccess, log.EndedAt, runID, traceID, "")).Required()
}

// seededWorkspace builds a WorkspaceEntry and seeds a memory repository with a
// normal case, a private case, a draft case, one action per non-draft case, one
// memo per non-draft case, one finished job run per case (the draft included, so
// the tests can prove its runs are dropped rather than merely absent), one tag,
// and one knowledge entry. It returns the normal, private and draft case ids.
func seededWorkspace(t *testing.T) (interfaces.Repository, *model.WorkspaceEntry, string, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	repo := memory.New()
	wsID := "test-ws"

	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "Test WS"},
		FieldSchema: &domainconfig.FieldSchema{Fields: []domainconfig.FieldDefinition{
			{ID: "severity", Type: types.FieldTypeSelect},
			{ID: "score", Type: types.FieldTypeNumber},
			{ID: "labels", Type: types.FieldTypeMultiSelect},
			{ID: "when", Type: types.FieldTypeDate},
		}},
		MemoConfig: &domainconfig.MemoConfig{FieldSchema: &domainconfig.FieldSchema{Fields: []domainconfig.FieldDefinition{
			{ID: "note", Type: types.FieldTypeText},
		}}},
	}

	now := time.Now()

	normal, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title:       "Normal",
		Status:      types.CaseStatusOpen,
		ReporterID:  "U1",
		AssigneeIDs: []string{"U9"},
		IsPrivate:   false,
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"score":    {FieldID: "score", Type: types.FieldTypeNumber, Value: float64(4)},
			"labels":   {FieldID: "labels", Type: types.FieldTypeMultiSelect, Value: []string{"x", "y"}},
			// date stored as time.Time (a valid stored form) must become a STRING
			// cell, not silently NULL.
			"when": {FieldID: "when", Type: types.FieldTypeDate, Value: dateFieldValue},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	private, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title:      "Secret",
		Status:     types.CaseStatusOpen,
		ReporterID: "U2",
		IsPrivate:  true,
		CreatedAt:  now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	draft, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title:      "Draft",
		Status:     types.CaseStatusDraft,
		ReporterID: "U3",
		CreatedAt:  now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// One action per non-draft case.
	for _, cid := range []int64{normal.ID, private.ID} {
		_, err = repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: cid, Title: "act", Status: types.ActionStatusTodo,
			CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()
	}

	// One memo per non-draft case.
	for _, cid := range []int64{normal.ID, private.ID} {
		_, err = repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: model.NewMemoID(), WorkspaceID: wsID, CaseID: cid, Title: "memo",
			FieldValues: map[string]model.FieldValue{
				"note": {FieldID: "note", Type: types.FieldTypeText, Value: "hello"},
			},
			CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()
	}

	// One finished job run per case. The draft gets one too: without it, an empty
	// job_runs row set could mean either "the draft's runs were dropped" or
	// "the draft never ran".
	seedJobRun(t, repo, wsID, normal.ID, "triage", now)
	seedJobRun(t, repo, wsID, private.ID, "triage", now)
	seedJobRun(t, repo, wsID, draft.ID, "triage", now)

	tag, err := repo.Tag().Create(ctx, wsID, &model.Tag{
		ID: model.NewTagID(), WorkspaceID: wsID, Name: "urgent",
		CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	_, err = repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
		ID: model.NewKnowledgeID(), WorkspaceID: wsID, Title: "kb", Claim: "a fact",
		TagIDs: []model.TagID{tag.ID}, CreatorID: "U1",
		CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	return repo, entry, wsID, normal.ID, private.ID, draft.ID
}

func TestExporter_Run_full(t *testing.T) {
	ctx := context.Background()
	repo, entry, _, normalID, privateID, draftID := seededWorkspace(t)
	sink := newFakeSink()

	err := export.New(repo, sink).Run(ctx, []export.Target{{Entry: entry, Namespace: "ds"}})
	gt.NoError(t, err).Required()

	// Cases: normal + private (draft excluded).
	cases := sink.table("ds", "cases")
	gt.Array(t, cases.Rows).Length(2)
	gt.True(t, findRow(cases, "title", "Draft") == nil)
	gt.True(t, hasColumn(cases, "field_severity"))
	gt.True(t, hasColumn(cases, "field_score"))
	gt.True(t, hasColumn(cases, "field_labels"))

	normalRow := findRow(cases, "id", normalID)
	gt.Value(t, normalRow).NotNil().Required()
	gt.Value(t, normalRow["field_severity"]).Equal("high")
	gt.Value(t, normalRow["field_score"]).Equal(float64(4))
	gt.Array(t, normalRow["field_labels"].([]string)).Equal([]string{"x", "y"})
	gt.Value(t, normalRow["field_when"]).Equal(dateFieldValue.Format(time.RFC3339Nano))
	gt.Value(t, normalRow["is_private"]).Equal(false)
	gt.Value(t, normalRow["status"]).Equal("OPEN")

	privateRow := findRow(cases, "id", privateID)
	gt.Value(t, privateRow).NotNil().Required()
	gt.Value(t, privateRow["is_private"]).Equal(true)

	// Actions: one per non-draft case.
	actions := sink.table("ds", "actions")
	gt.Array(t, actions.Rows).Length(2)

	// Memos: one per non-draft case, with the memo custom field column.
	memos := sink.table("ds", "memos")
	gt.Array(t, memos.Rows).Length(2)
	gt.True(t, hasColumn(memos, "field_note"))
	gt.Value(t, memos.Rows[0]["field_note"]).Equal("hello")

	// Job runs: one summary + one log per non-draft case.
	jobRuns := sink.table("ds", "job_runs")
	gt.Array(t, jobRuns.Rows).Length(2)
	normalRunRow := findRow(jobRuns, "case_id", normalID)
	gt.Value(t, normalRunRow).NotNil().Required()
	gt.Value(t, normalRunRow["job_id"]).Equal("triage")
	gt.Value(t, normalRunRow["last_status"]).Equal("SUCCESS")
	gt.Value(t, normalRunRow["last_run_id"]).Equal("run-triage")
	gt.Value(t, findRow(jobRuns, "case_id", privateID)).NotNil()
	// The draft case ran a job too, and its run is excluded along with the case.
	gt.True(t, findRow(jobRuns, "case_id", draftID) == nil)

	jobRunLogs := sink.table("ds", "job_run_logs")
	gt.Array(t, jobRunLogs.Rows).Length(2)
	gt.True(t, findRow(jobRunLogs, "case_id", draftID) == nil)
	normalLogRow := findRow(jobRunLogs, "case_id", normalID)
	gt.Value(t, normalLogRow).NotNil().Required()
	gt.Value(t, normalLogRow["run_id"]).Equal("run-triage")
	gt.Value(t, normalLogRow["stage"]).Equal("SUCCESS")
	gt.Value(t, normalLogRow["executor_kind"]).Equal(model.ExecutorKindPlanexec)
	gt.Value(t, normalLogRow["event_type"]).Equal("case")
	gt.Value(t, normalLogRow["system_prompt"]).Equal("you are the job agent")
	gt.Value(t, normalLogRow["input_tokens"]).Equal(int64(1500))
	gt.Value(t, normalLogRow["output_tokens"]).Equal(int64(210))
	gt.Value(t, normalLogRow["cache_creation_input_tokens"]).Equal(int64(400))
	gt.Value(t, normalLogRow["cache_read_input_tokens"]).Equal(int64(700))
	gt.Value(t, normalLogRow["llm_call_count"]).Equal(int64(4))
	gt.Value(t, normalLogRow["tool_call_count"]).Equal(int64(6))
	// The cost is exported in nano-USD, the unit it is stored in, so a spend
	// query sums integers rather than accumulating float error.
	gt.Value(t, normalLogRow["cost_nano_usd"]).Equal(int64(2_550_000))
	gt.Value(t, normalLogRow["model"]).Equal("gemini-3.7-flash")

	// Job run events: the full timeline of each exported run (4 events x 2 cases).
	jobRunEvents := sink.table("ds", "job_run_events")
	gt.Array(t, jobRunEvents.Rows).Length(8)
	gt.True(t, findRow(jobRunEvents, "case_id", draftID) == nil)

	// Each kind populates only its own columns; the rest stay absent (NULL).
	reqRow := findEventRow(jobRunEvents, normalID, 1)
	gt.Value(t, reqRow).NotNil().Required()
	gt.Value(t, reqRow["kind"]).Equal("LLM_REQUEST")
	gt.Value(t, reqRow["run_id"]).Equal("run-triage")
	gt.Value(t, reqRow["phase"]).Equal("execute")
	gt.Value(t, reqRow["agent_label"]).Equal("investigator")
	gt.Value(t, reqRow["model"]).Equal("claude-opus-4-7")
	gt.String(t, reqRow["messages_json"].(string)).Contains("investigate the case")
	gt.String(t, reqRow["tools_json"].(string)).Contains("slack_search")
	gt.True(t, reqRow["tool_name"] == nil)

	respRow := findEventRow(jobRunEvents, normalID, 2)
	gt.Value(t, respRow).NotNil().Required()
	gt.Value(t, respRow["kind"]).Equal("LLM_RESPONSE")
	gt.String(t, respRow["texts_json"].(string)).Contains("let me search")
	gt.String(t, respRow["function_calls_json"].(string)).Contains("fc-1")
	gt.Value(t, respRow["input_tokens"]).Equal(int64(1200))
	gt.Value(t, respRow["output_tokens"]).Equal(int64(180))
	gt.Value(t, respRow["cache_creation_input_tokens"]).Equal(int64(400))
	gt.Value(t, respRow["cache_read_input_tokens"]).Equal(int64(700))
	gt.Value(t, respRow["duration_ms"]).Equal(int64(1450))
	gt.True(t, respRow["messages_json"] == nil)

	toolRow := findEventRow(jobRunEvents, normalID, 3)
	gt.Value(t, toolRow).NotNil().Required()
	gt.Value(t, toolRow["kind"]).Equal("TOOL_CALL")
	gt.Value(t, toolRow["parent_sequence"]).Equal(int64(2))
	gt.Value(t, toolRow["tool_name"]).Equal("slack_search")
	gt.Value(t, toolRow["tool_arguments_json"]).Equal(`{"q":"foo"}`)
	gt.Value(t, toolRow["tool_result_json"]).Equal(`{"hits":2}`)
	gt.Value(t, toolRow["tool_is_error"]).Equal(false)
	gt.True(t, toolRow["model"] == nil)

	// A response whose payload slices were empty exports them as NULL, because
	// both repository backends decode a stored empty array back into a nil slice
	// (see "Append + List returns an empty payload slice as nil" in
	// pkg/repository/job_run_test.go). The row itself and its scalars survive, so
	// the call is still visible in the timeline.
	emptyRow := findEventRow(jobRunEvents, normalID, 4)
	gt.Value(t, emptyRow).NotNil().Required()
	gt.Value(t, emptyRow["kind"]).Equal("LLM_RESPONSE")
	gt.True(t, emptyRow["texts_json"] == nil)
	gt.True(t, emptyRow["function_calls_json"] == nil)
	gt.Value(t, emptyRow["input_tokens"]).Equal(int64(90))
	// A provider that reported no prompt-cache usage exports zeros, not NULL:
	// the call did happen and its cache share was nil.
	gt.Value(t, emptyRow["cache_creation_input_tokens"]).Equal(int64(0))
	gt.Value(t, emptyRow["cache_read_input_tokens"]).Equal(int64(0))

	// Knowledge / Tag.
	knowledge := sink.table("ds", "knowledge")
	gt.Array(t, knowledge.Rows).Length(1)
	gt.Array(t, knowledge.Rows[0]["tag_ids"].([]string)).Length(1)

	tags := sink.table("ds", "tags")
	gt.Array(t, tags.Rows).Length(1)
	gt.Value(t, tags.Rows[0]["name"]).Equal("urgent")
}

func TestExporter_Run_excludePrivate(t *testing.T) {
	ctx := context.Background()
	repo, entry, _, normalID, privateID, draftID := seededWorkspace(t)
	sink := newFakeSink()

	err := export.New(repo, sink).Run(ctx, []export.Target{{Entry: entry, Namespace: "ds", ExcludePrivate: true}})
	gt.NoError(t, err).Required()

	// Cases: only the normal (non-private, non-draft) case.
	cases := sink.table("ds", "cases")
	gt.Array(t, cases.Rows).Length(1)
	gt.Value(t, findRow(cases, "id", normalID)).NotNil()
	gt.True(t, findRow(cases, "id", privateID) == nil)

	// Actions: the private case's action is dropped.
	actions := sink.table("ds", "actions")
	gt.Array(t, actions.Rows).Length(1)
	gt.Value(t, actions.Rows[0]["case_id"]).Equal(normalID)

	// Memos: only the non-private case's memo.
	memos := sink.table("ds", "memos")
	gt.Array(t, memos.Rows).Length(1)
	gt.Value(t, memos.Rows[0]["case_id"]).Equal(normalID)

	// Job runs / run logs: the private case's run is dropped, so neither its
	// summary nor its prompts and token counts reach BigQuery.
	jobRuns := sink.table("ds", "job_runs")
	gt.Array(t, jobRuns.Rows).Length(1)
	gt.Value(t, jobRuns.Rows[0]["case_id"]).Equal(normalID)
	gt.True(t, findRow(jobRuns, "case_id", privateID) == nil)
	gt.True(t, findRow(jobRuns, "case_id", draftID) == nil)

	jobRunLogs := sink.table("ds", "job_run_logs")
	gt.Array(t, jobRunLogs.Rows).Length(1)
	gt.Value(t, jobRunLogs.Rows[0]["case_id"]).Equal(normalID)
	gt.True(t, findRow(jobRunLogs, "case_id", privateID) == nil)
	gt.True(t, findRow(jobRunLogs, "case_id", draftID) == nil)

	// The excluded cases' timelines go too — prompts, tool results and all.
	jobRunEvents := sink.table("ds", "job_run_events")
	gt.Array(t, jobRunEvents.Rows).Length(4)
	gt.Value(t, findEventRow(jobRunEvents, normalID, 1)).NotNil()
	gt.True(t, findRow(jobRunEvents, "case_id", privateID) == nil)
	gt.True(t, findRow(jobRunEvents, "case_id", draftID) == nil)

	// Knowledge / Tag are workspace-level and always exported.
	gt.Array(t, sink.table("ds", "knowledge").Rows).Length(1)
	gt.Array(t, sink.table("ds", "tags").Rows).Length(1)
}

// Archiving is a visibility change in the product, not a reason to drop the
// row from analytics: the export asks for every archive slice and carries the
// state in the archived_at column, the way it already does for Actions.
func TestExporter_Run_includesArchivedCases(t *testing.T) {
	ctx := context.Background()
	repo, entry, wsID, normalID, _, _ := seededWorkspace(t)

	archivedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	archived, err := repo.Case().Create(ctx, wsID, &model.Case{
		ReporterID: "U-REPORTER",
		Title:      "Archived case",
		Status:     types.CaseStatusClosed,
		ArchivedAt: &archivedAt,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	})
	gt.NoError(t, err).Required()

	sink := newFakeSink()
	gt.NoError(t, export.New(repo, sink).Run(ctx, []export.Target{{Entry: entry, Namespace: "ds"}})).Required()

	cases := sink.table("ds", "cases")
	gt.True(t, hasColumn(cases, "archived_at"))

	archivedRow := findRow(cases, "id", archived.ID)
	gt.Value(t, archivedRow).NotNil().Required()
	gt.Value(t, archivedRow["status"]).Equal("CLOSED")
	gotArchivedAt, ok := archivedRow["archived_at"].(*time.Time)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gotArchivedAt).NotNil().Required()
	gt.Bool(t, gotArchivedAt.Equal(archivedAt)).True()

	// An active case still reports a null archived_at.
	normalRow := findRow(cases, "id", normalID)
	gt.Value(t, normalRow).NotNil().Required()
	gt.Value(t, normalRow["archived_at"]).Nil()
}

func TestExporter_Run_collectsErrorsAndContinues(t *testing.T) {
	ctx := context.Background()
	repo, entry, _, _, _, _ := seededWorkspace(t)
	sink := newFakeSink()
	sink.failOn["cases"] = true

	err := export.New(repo, sink).Run(ctx, []export.Target{{Entry: entry, Namespace: "ds"}})
	gt.Error(t, err) // the cases-table failure is surfaced

	// Remaining tables are still written despite the cases failure.
	gt.Value(t, sink.table("ds", "cases")).Nil()
	gt.Value(t, sink.table("ds", "actions")).NotNil()
	gt.Value(t, sink.table("ds", "job_runs")).NotNil()
	gt.Value(t, sink.table("ds", "job_run_logs")).NotNil()
	gt.Value(t, sink.table("ds", "job_run_events")).NotNil()
	gt.Value(t, sink.table("ds", "knowledge")).NotNil()
	gt.Value(t, sink.table("ds", "tags")).NotNil()
}

// eventReadFailingRepository is a Repository whose event timeline cannot be
// read. It stands in for a Firestore failure on the heaviest query the export
// makes; everything else behaves normally.
type eventReadFailingRepository struct {
	interfaces.Repository
}

func (r eventReadFailingRepository) JobRunEvent() interfaces.JobRunEventRepository {
	return failingJobRunEventRepository{}
}

type failingJobRunEventRepository struct{}

func (failingJobRunEventRepository) Append(context.Context, *model.JobRunEvent) error { return nil }

func (failingJobRunEventRepository) AppendNext(context.Context, *model.JobRunEvent) error { return nil }

func (failingJobRunEventRepository) LatestLLMResponseSequence(context.Context, model.JobRunKey, string) (int64, error) {
	return 0, nil
}

func (failingJobRunEventRepository) List(context.Context, model.JobRunKey, string) ([]*model.JobRunEvent, error) {
	return nil, errors.New("injected job run event read failure")
}

// TestExporter_Run_eventFailureKeepsJobRunTables pins the failure granularity of
// the agent-run tables. The event timeline is the largest and most failure-prone
// read the export makes; when it breaks, the summaries and logs — which were
// collected in full — must still be refreshed rather than left as a stale
// snapshot in BigQuery.
func TestExporter_Run_eventFailureKeepsJobRunTables(t *testing.T) {
	ctx := context.Background()
	repo, entry, _, normalID, _, _ := seededWorkspace(t)
	sink := newFakeSink()

	err := export.New(eventReadFailingRepository{Repository: repo}, sink).
		Run(ctx, []export.Target{{Entry: entry, Namespace: "ds"}})
	// The event failure is surfaced, not swallowed.
	gt.Error(t, err)

	// The two levels that completed are written.
	jobRuns := sink.table("ds", "job_runs")
	gt.Value(t, jobRuns).NotNil().Required()
	gt.Array(t, jobRuns.Rows).Length(2)
	jobRunLogs := sink.table("ds", "job_run_logs")
	gt.Value(t, jobRunLogs).NotNil().Required()
	gt.Array(t, jobRunLogs.Rows).Length(2)
	gt.Value(t, findRow(jobRunLogs, "case_id", normalID)).NotNil()

	// The incomplete level is skipped entirely: a partial slice must never be
	// published, because every write is a full refresh.
	gt.Value(t, sink.table("ds", "job_run_events")).Nil()

	// The unrelated tables are unaffected.
	gt.Value(t, sink.table("ds", "cases")).NotNil()
	gt.Value(t, sink.table("ds", "actions")).NotNil()
	gt.Value(t, sink.table("ds", "memos")).NotNil()
	gt.Value(t, sink.table("ds", "knowledge")).NotNil()
	gt.Value(t, sink.table("ds", "tags")).NotNil()
}

// deleteTableOnCleanup registers dataset.table for deletion and fails the test
// if the delete does not go through — a live test that quietly leaves tables
// behind in the shared test dataset would keep passing while it litters. A 404
// is the one tolerated outcome: the run can fail before a given table is ever
// created, and turning that into a cleanup failure would bury the real one.
func deleteTableOnCleanup(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) {
	t.Helper()
	t.Cleanup(func() {
		err := client.Dataset(dataset).Table(table).Delete(ctx)
		if err == nil {
			return
		}
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == 404 {
			return
		}
		gt.NoError(t, err)
	})
}

// readAllRows reads every row of dataset.table back from BigQuery.
func readAllRows(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) []map[string]bq.Value {
	t.Helper()
	it := client.Dataset(dataset).Table(table).Read(ctx)
	var rows []map[string]bq.Value
	for {
		var row map[string]bq.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		gt.NoError(t, err).Required()
		rows = append(rows, row)
	}
	return rows
}

func findRowByID(rows []map[string]bq.Value, id int64) map[string]bq.Value {
	for _, r := range rows {
		if v, ok := r["id"].(int64); ok && v == id {
			return r
		}
	}
	return nil
}

// TestExporter_LiveBigQuery drives the whole Exporter against a real BigQuery
// dataset and reads the result back. It is the operational-path verification the
// sink-level unit tests cannot provide. Gated on TEST_BIGQUERY_PROJECT_ID /
// TEST_BIGQUERY_DATASET_ID. The dataset is used as-is (never created/dropped);
// the tables are given a unique per-run prefix so repeated/concurrent runs never
// collide, and are dropped on cleanup.
func TestExporter_LiveBigQuery(t *testing.T) {
	project := os.Getenv("TEST_BIGQUERY_PROJECT_ID")
	dataset := os.Getenv("TEST_BIGQUERY_DATASET_ID")
	if project == "" || dataset == "" {
		t.Skip("TEST_BIGQUERY_PROJECT_ID / TEST_BIGQUERY_DATASET_ID not set; skipping live BigQuery export test")
	}
	location := os.Getenv("TEST_BIGQUERY_LOCATION")
	ctx := context.Background()

	sink, err := bqexport.New(ctx, project, location)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { safe.Close(ctx, sink) })

	client, err := bq.NewClient(ctx, project)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { safe.Close(ctx, client) })

	// Unique per-run table names within the shared, pre-provisioned dataset.
	prefix := fmt.Sprintf("export_it_%d_", time.Now().UnixNano())
	tbl := func(name string) string { return prefix + name }
	for _, name := range []string{
		"cases", "actions", "memos",
		"job_runs", "job_run_logs", "job_run_events",
		"knowledge", "tags",
	} {
		deleteTableOnCleanup(t, ctx, client, dataset, tbl(name))
	}

	repo, entry, wsID, normalID, privateID, _ := seededWorkspace(t)
	targets := []export.Target{{Entry: entry, Namespace: dataset}}
	exporter := export.New(repo, sink, export.WithTablePrefix(prefix))

	// First run: create tables + append.
	gt.NoError(t, exporter.Run(ctx, targets)).Required()
	caseRows := readAllRows(t, ctx, client, dataset, tbl("cases"))
	gt.Array(t, caseRows).Length(2) // normal + private, draft excluded
	normalRow := findRowByID(caseRows, normalID)
	gt.Value(t, normalRow).NotNil().Required()
	gt.Value(t, normalRow["field_severity"]).Equal("high")
	gt.Value(t, normalRow["field_score"]).Equal(float64(4))
	gt.Value(t, normalRow["title"]).Equal("Normal")
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("actions"))).Length(2)
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("memos"))).Length(2)
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("tags"))).Length(1)

	// Job runs: one summary + one log per non-draft case, with the token totals
	// read back through the real BigQuery INT64 columns.
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("job_runs"))).Length(2)
	jobLogRows := readAllRows(t, ctx, client, dataset, tbl("job_run_logs"))
	gt.Array(t, jobLogRows).Length(2).Required()
	for _, r := range jobLogRows {
		gt.Value(t, r["stage"]).Equal("SUCCESS")
		gt.Value(t, r["run_id"]).Equal("run-triage")
		gt.Value(t, r["input_tokens"]).Equal(int64(1500))
		gt.Value(t, r["output_tokens"]).Equal(int64(210))
		gt.Value(t, r["cache_creation_input_tokens"]).Equal(int64(400))
		gt.Value(t, r["cache_read_input_tokens"]).Equal(int64(700))
		gt.Value(t, r["llm_call_count"]).Equal(int64(4))
		gt.Value(t, r["tool_call_count"]).Equal(int64(6))
		gt.Value(t, r["cost_nano_usd"]).Equal(int64(2_550_000))
		gt.Value(t, r["model"]).Equal("gemini-3.7-flash")
	}

	// The event timeline round-trips through the real BigQuery schema, including
	// the JSON payload columns and the per-kind NULLs.
	jobEventRows := readAllRows(t, ctx, client, dataset, tbl("job_run_events"))
	gt.Array(t, jobEventRows).Length(8).Required()
	var sawRequest, sawTool, sawEmptyPayload bool
	for _, r := range jobEventRows {
		switch r["kind"] {
		case "LLM_REQUEST":
			sawRequest = true
			gt.Value(t, r["model"]).Equal("claude-opus-4-7")
			gt.String(t, r["messages_json"].(string)).Contains("investigate the case")
			gt.Value(t, r["tool_name"]).Nil()
		case "TOOL_CALL":
			sawTool = true
			gt.Value(t, r["tool_name"]).Equal("slack_search")
			gt.Value(t, r["tool_result_json"]).Equal(`{"hits":2}`)
			gt.Value(t, r["parent_sequence"]).Equal(int64(2))
			gt.Value(t, r["messages_json"]).Nil()
		case "LLM_RESPONSE":
			// The sequence-4 response carried no text and requested no tool. Its
			// payload columns are NULL (the repository decodes a stored empty
			// array as a nil slice) while its scalars still identify the call.
			if r["sequence"] == int64(4) {
				sawEmptyPayload = true
				gt.Value(t, r["texts_json"]).Nil()
				gt.Value(t, r["function_calls_json"]).Nil()
				gt.Value(t, r["input_tokens"]).Equal(int64(90))
				gt.Value(t, r["cache_read_input_tokens"]).Equal(int64(0))
			}
		}
	}
	gt.Bool(t, sawRequest).True()
	gt.Bool(t, sawTool).True()
	gt.Bool(t, sawEmptyPayload).True()

	// Second run: full refresh — the row count must not double.
	gt.NoError(t, exporter.Run(ctx, targets)).Required()
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("cases"))).Length(2)

	// exclude-private run: only the normal case (and its children) remain.
	targetsExclude := []export.Target{{Entry: entry, Namespace: dataset, ExcludePrivate: true}}
	gt.NoError(t, export.New(repo, sink, export.WithTablePrefix(prefix)).Run(ctx, targetsExclude)).Required()
	privateFiltered := readAllRows(t, ctx, client, dataset, tbl("cases"))
	gt.Array(t, privateFiltered).Length(1)
	gt.Value(t, findRowByID(privateFiltered, privateID)).Nil()
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("actions"))).Length(1)
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("job_runs"))).Length(1)
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("job_run_logs"))).Length(1)
	gt.Array(t, readAllRows(t, ctx, client, dataset, tbl("job_run_events"))).Length(4)

	// Schema evolution: add a field, set it on the normal case, re-run. The new
	// column must appear and carry the value.
	entry.FieldSchema.Fields = append(entry.FieldSchema.Fields,
		domainconfig.FieldDefinition{ID: "newfield", Type: types.FieldTypeText})
	normalCase, err := repo.Case().Get(ctx, wsID, normalID)
	gt.NoError(t, err).Required()
	normalCase.FieldValues["newfield"] = model.FieldValue{
		FieldID: "newfield", Type: types.FieldTypeText, Value: "evolved",
	}
	_, err = repo.Case().Update(ctx, wsID, normalCase)
	gt.NoError(t, err).Required()

	gt.NoError(t, export.New(repo, sink, export.WithTablePrefix(prefix)).Run(ctx, targets)).Required()
	md, err := client.Dataset(dataset).Table(tbl("cases")).Metadata(ctx)
	gt.NoError(t, err).Required()
	hasNewColumn := false
	for _, f := range md.Schema {
		if f.Name == "field_newfield" {
			hasNewColumn = true
		}
	}
	gt.Bool(t, hasNewColumn).True()
	evolved := findRowByID(readAllRows(t, ctx, client, dataset, tbl("cases")), normalID)
	gt.Value(t, evolved).NotNil().Required()
	gt.Value(t, evolved["field_newfield"]).Equal("evolved")

	// Field retyped from single-value to multi-select. This is the shape of the
	// production outage: the live column is STRING NULLABLE and the export now
	// wants ARRAY<STRING>. The run must complete and the column must end up
	// REPEATED, with no operator step in between.
	for i := range entry.FieldSchema.Fields {
		if entry.FieldSchema.Fields[i].ID == "severity" {
			entry.FieldSchema.Fields[i].Type = types.FieldTypeMultiSelect
		}
	}
	retyped, err := repo.Case().Get(ctx, wsID, normalID)
	gt.NoError(t, err).Required()
	retyped.FieldValues["severity"] = model.FieldValue{
		FieldID: "severity", Type: types.FieldTypeMultiSelect, Value: []string{"high", "urgent"},
	}
	_, err = repo.Case().Update(ctx, wsID, retyped)
	gt.NoError(t, err).Required()

	gt.NoError(t, export.New(repo, sink, export.WithTablePrefix(prefix)).Run(ctx, targets)).Required()

	mdRetyped, err := client.Dataset(dataset).Table(tbl("cases")).Metadata(ctx)
	gt.NoError(t, err).Required()
	var severityField *bq.FieldSchema
	for _, f := range mdRetyped.Schema {
		if f.Name == "field_severity" {
			severityField = f
		}
	}
	gt.Value(t, severityField).NotNil().Required()
	gt.Bool(t, severityField.Repeated).True()
	gt.Value(t, severityField.Type).Equal(bq.StringFieldType)

	multi := findRowByID(readAllRows(t, ctx, client, dataset, tbl("cases")), normalID)
	gt.Value(t, multi).NotNil().Required()
	gt.Value(t, multi["field_severity"]).Equal([]bq.Value{"high", "urgent"})

	// A field removed from the workspace schema must disappear from the table:
	// the destination carries exactly the columns the export produces.
	remaining := make([]domainconfig.FieldDefinition, 0, len(entry.FieldSchema.Fields))
	for _, f := range entry.FieldSchema.Fields {
		if f.ID != "newfield" {
			remaining = append(remaining, f)
		}
	}
	entry.FieldSchema.Fields = remaining

	gt.NoError(t, export.New(repo, sink, export.WithTablePrefix(prefix)).Run(ctx, targets)).Required()

	mdDropped, err := client.Dataset(dataset).Table(tbl("cases")).Metadata(ctx)
	gt.NoError(t, err).Required()
	for _, f := range mdDropped.Schema {
		gt.Value(t, f.Name).NotEqual("field_newfield")
	}

	// The staging tables are throwaway: none may outlive the run that made them.
	stagingIt := client.Dataset(dataset).Tables(ctx)
	for {
		tb, err := stagingIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.HasPrefix(tb.TableID, prefix) && strings.Contains(tb.TableID, "_stg_")).False()
	}
}
