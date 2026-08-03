package export_test

import (
	"context"
	"errors"
	"fmt"
	"os"
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
// JobRunLog carrying token totals plus the JobRun summary doc that
// JobRun().ListByCase surfaces (the export reaches the logs through it).
func seedJobRun(t *testing.T, repo interfaces.Repository, wsID string, caseID int64, jobID string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: caseID, JobID: jobID}
	log := &model.JobRunLog{
		WorkspaceID:    wsID,
		CaseID:         caseID,
		JobID:          jobID,
		RunID:          "run-" + jobID,
		TraceID:        "trace-" + jobID,
		Stage:          model.JobRunStageRunning,
		StartedAt:      now,
		ExecutorKind:   model.ExecutorKindPlanexec,
		EventType:      "case",
		EventTriggerAt: now,
		SystemPrompt:   "you are the job agent",
	}
	gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()

	log.Stage = model.JobRunStageSuccess
	log.EndedAt = now.Add(time.Minute)
	log.InputTokens = 1500
	log.OutputTokens = 210
	log.LLMCallCount = 4
	gt.NoError(t, repo.JobRunLog().Finish(ctx, log)).Required()
	gt.NoError(t, repo.JobRun().RecordRun(ctx, key,
		model.JobRunStatusSuccess, log.EndedAt, log.RunID, log.TraceID, "")).Required()
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
	gt.Value(t, normalLogRow["llm_call_count"]).Equal(int64(4))

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

	// Knowledge / Tag are workspace-level and always exported.
	gt.Array(t, sink.table("ds", "knowledge").Rows).Length(1)
	gt.Array(t, sink.table("ds", "tags").Rows).Length(1)
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
	gt.Value(t, sink.table("ds", "knowledge")).NotNil()
	gt.Value(t, sink.table("ds", "tags")).NotNil()
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
	for _, name := range []string{"cases", "actions", "memos", "job_runs", "job_run_logs", "knowledge", "tags"} {
		table := tbl(name)
		t.Cleanup(func() { _ = client.Dataset(dataset).Table(table).Delete(ctx) })
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
		gt.Value(t, r["llm_call_count"]).Equal(int64(4))
	}

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

	// Schema evolution: add a field, set it on the normal case, re-run. The new
	// column must appear (evolved in place) and carry the value. The append right
	// after Table.Update may hit SCHEMA_MISMATCH and is retried internally.
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
}
