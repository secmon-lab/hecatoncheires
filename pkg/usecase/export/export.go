// Package export implements the `export` subcommand's core: reading the current
// state of every configured workspace out of the repository and writing it, one
// table per entity, to a pluggable Sink (BigQuery today). The Exporter is
// sink-agnostic — it describes a table as typed columns plus rows of natural Go
// values and streams those rows into the Sink, which owns all backend-specific
// concerns (schema evolution, full-refresh, encoding). This keeps a future Cloud
// Storage sink a drop-in without touching the read/normalize logic here.
//
// Rows are streamed rather than assembled: a table is opened with BeginTable,
// fed with AppendRows as the rows are read, and published with Commit. That is
// what keeps the exporter's peak memory bounded by one read's worth of rows
// instead of by the workspace's total volume — job_run_events alone reaches
// gigabytes on a busy workspace, which is enough to end the process.
package export

import (
	"context"
	"errors"
	"io"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// ColumnType is the backend-neutral logical type of a Table column. A Sink maps
// it to its own type system (e.g. BigQuery STRING/INT64/...).
type ColumnType string

const (
	TypeString    ColumnType = "STRING"
	TypeInt       ColumnType = "INT64"
	TypeFloat     ColumnType = "FLOAT64"
	TypeBool      ColumnType = "BOOL"
	TypeTimestamp ColumnType = "TIMESTAMP"
)

// Column describes one output column.
type Column struct {
	// Name is the column name (fixed columns are snake_case; custom fields are
	// "field_<id>").
	Name string
	// Type is the logical column type.
	Type ColumnType
	// Repeated marks an array column (ARRAY<Type>).
	Repeated bool
	// Nullable marks a nullable column. A non-nullable, non-repeated column is
	// REQUIRED in the sink's schema.
	Nullable bool
}

// TableWriter is one table's in-progress full refresh. Row values are natural
// Go types keyed by column name; a missing key is NULL. Backend-specific
// encoding (e.g. TIMESTAMP -> microseconds) is the Sink's job.
//
// A writer is used by a single goroutine and exactly once: append as many
// batches as the caller has, then either Commit or Abort.
type TableWriter interface {
	// AppendRows stages rows. It may flush to the backend at its discretion,
	// so the caller must not assume anything is durable until Commit.
	AppendRows(ctx context.Context, rows []map[string]any) error
	// Commit publishes everything staged as the table's new full contents.
	Commit(ctx context.Context) error
	// Abort discards what was staged and leaves the destination untouched.
	// It is a no-op after a successful Commit, so it is safe to defer.
	Abort(ctx context.Context)
}

// Sink is a destination that fully replaces (洗替) a table's schema and rows.
// Implementations MUST make each BeginTable/Commit pair a full refresh of the
// named table within the given namespace, and MUST leave the destination at its
// previous contents when the pair ends in Abort or in any error.
type Sink interface {
	// BeginTable starts a full refresh of namespace.name with the given schema.
	BeginTable(ctx context.Context, namespace, name string, columns []Column) (TableWriter, error)
	io.Closer
}

// WriteTable is the one-shot form of the streaming protocol: open the table,
// append every row, publish. It exists so a caller with the whole table already
// in hand does not have to repeat the abort-on-failure bookkeeping, which is
// what keeps a half-written refresh from ever reaching the destination.
func WriteTable(ctx context.Context, sink Sink, namespace, name string, columns []Column, rows []map[string]any) error {
	w, err := sink.BeginTable(ctx, namespace, name, columns)
	if err != nil {
		return goerr.Wrap(err, "failed to begin table",
			goerr.V("namespace", namespace), goerr.V("table", name))
	}
	defer w.Abort(ctx)

	if len(rows) > 0 {
		if err := w.AppendRows(ctx, rows); err != nil {
			return goerr.Wrap(err, "failed to append rows",
				goerr.V("namespace", namespace), goerr.V("table", name))
		}
	}
	if err := w.Commit(ctx); err != nil {
		return goerr.Wrap(err, "failed to commit table",
			goerr.V("namespace", namespace), goerr.V("table", name))
	}
	return nil
}

// Target binds one workspace to its destination namespace (a BigQuery dataset)
// and its per-workspace privacy policy.
type Target struct {
	Entry     *model.WorkspaceEntry
	Namespace string
	// ExcludePrivate, when true, omits this workspace's private Cases (and their
	// Actions / Memos). It is resolved per workspace by the caller.
	ExcludePrivate bool
}

// Exporter reads workspace data from the repository and writes it to a Sink.
// It holds no mutable state across a run.
type Exporter struct {
	repo        interfaces.Repository
	sink        Sink
	tablePrefix string
}

// Option customizes an Exporter.
type Option func(*Exporter)

// WithTablePrefix prepends prefix to every table name. It is empty in
// production (tables are named exactly cases/actions/...); tests use it to write
// uniquely-named tables into a shared dataset without recreating the dataset.
func WithTablePrefix(prefix string) Option {
	return func(e *Exporter) { e.tablePrefix = prefix }
}

// New builds an Exporter. repo and sink are required; opts are optional. The
// per-workspace privacy policy travels on each Target, not here.
func New(repo interfaces.Repository, sink Sink, opts ...Option) *Exporter {
	e := &Exporter{repo: repo, sink: sink}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// tableName applies the (optional) table-name prefix.
func (e *Exporter) tableName(name string) string { return e.tablePrefix + name }

// writeTable fully refreshes one table whose rows are already in hand.
func (e *Exporter) writeTable(ctx context.Context, namespace, name string, columns []Column, rows []map[string]any) error {
	return WriteTable(ctx, e.sink, namespace, e.tableName(name), columns, rows)
}

// Run exports every target. A failure on one target is logged and collected but
// does not stop the others; all collected failures are returned joined so the
// caller (and its error reporter) sees every one.
//
// Single-instance assumption: Run is designed to be invoked by ONE process at a
// time and takes no distributed lock. Each table's refresh is staged in full
// before it replaces the destination, so overlapping runs cannot interleave
// into a half-written table — every snapshot is complete, and the destination
// ends up holding whichever run swapped last. This is a deliberate, documented
// constraint (the export is a singly-run batch job, e.g. a scheduled task with
// no overlap), not an oversight — see docs/export.md.
func (e *Exporter) Run(ctx context.Context, targets []Target) error {
	logger := logging.From(ctx)
	var errs []error
	for _, t := range targets {
		logger.Info("exporting workspace",
			"workspace_id", t.Entry.Workspace.ID, "dataset", t.Namespace)
		if err := e.exportWorkspace(ctx, t); err != nil {
			logger.Warn("workspace export failed; continuing with remaining workspaces",
				"workspace_id", t.Entry.Workspace.ID, "dataset", t.Namespace)
			errs = append(errs, goerr.Wrap(err, "failed to export workspace",
				goerr.V("workspace_id", t.Entry.Workspace.ID),
				goerr.V("dataset", t.Namespace)))
		}
	}
	return errors.Join(errs...)
}

// exportWorkspace writes every entity table for one workspace. Per-table
// failures are collected (not fail-fast) so one bad table does not hide the
// rest, and are returned joined.
func (e *Exporter) exportWorkspace(ctx context.Context, t Target) error {
	wsID := t.Entry.Workspace.ID
	ns := t.Namespace
	var errs []error

	// Cases (drafts excluded by List; private filtered here when configured).
	// Archived cases ARE exported — archiving is a visibility change in the
	// product, not a reason to drop the row from analytics, and the
	// archived_at column carries the state. Actions and Memos are exported the
	// same way.
	cases, casesErr := e.repo.Case().List(ctx, wsID,
		interfaces.WithArchiveScope(interfaces.CaseArchiveScopeAll))
	if casesErr != nil {
		// Without the exported case set we cannot scope Actions / Memos; skip
		// those, but still export the workspace-level Knowledge / Tag below.
		errs = append(errs, goerr.Wrap(casesErr, "failed to list cases"))
	} else {
		if t.ExcludePrivate {
			cases = filterNonPrivate(cases)
		}
		// keptCaseIDs is the set of exported cases. Actions and Memos are scoped
		// to it so a Case that was excluded (draft, or private when configured)
		// never leaks its children — independent of the repository backend (the
		// memory repo does not implement ExcludePrivateCaseActions).
		keptCaseIDs := caseIDSet(cases)

		if err := e.writeTable(ctx, ns, "cases",
			caseColumns(t.Entry.FieldSchema), caseRows(ctx, t.Entry.FieldSchema, cases)); err != nil {
			errs = append(errs, goerr.Wrap(err, "failed to write cases table"))
		}

		// Memos are Case-scoped: iterating the kept cases naturally excludes the
		// memos of any excluded Case.
		memos, memoErr := e.collectMemos(ctx, wsID, cases)
		if memoErr != nil {
			errs = append(errs, memoErr)
		} else if err := e.writeTable(ctx, ns, "memos",
			memoColumns(t.Entry.MemoConfig), memoRows(ctx, t.Entry.MemoConfig, memos)); err != nil {
			errs = append(errs, goerr.Wrap(err, "failed to write memos table"))
		}

		// Actions: list all (archived included), then drop any whose parent Case
		// is not in the exported set.
		actions, actionsErr := e.repo.Action().List(ctx, wsID, interfaces.ActionListOptions{
			ArchiveScope: interfaces.ActionArchiveScopeAll,
		})
		if actionsErr != nil {
			errs = append(errs, goerr.Wrap(actionsErr, "failed to list actions"))
		} else {
			actions = filterActionsByCases(actions, keptCaseIDs)
			if err := e.writeTable(ctx, ns, "actions", actionColumns(), actionRows(actions)); err != nil {
				errs = append(errs, goerr.Wrap(err, "failed to write actions table"))
			}
		}

		errs = append(errs, e.exportJobRunHistory(ctx, wsID, ns, cases)...)
	}

	// Knowledge / Tag are workspace-level (not Case-scoped), always exported.
	knowledge, knowledgeErr := e.repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{})
	if knowledgeErr != nil {
		errs = append(errs, goerr.Wrap(knowledgeErr, "failed to list knowledge"))
	} else if err := e.writeTable(ctx, ns, "knowledge", knowledgeColumns(), knowledgeRows(knowledge)); err != nil {
		errs = append(errs, goerr.Wrap(err, "failed to write knowledge table"))
	}

	tags, tagsErr := e.repo.Tag().List(ctx, wsID)
	if tagsErr != nil {
		errs = append(errs, goerr.Wrap(tagsErr, "failed to list tags"))
	} else if err := e.writeTable(ctx, ns, "tags", tagColumns(), tagRows(tags)); err != nil {
		errs = append(errs, goerr.Wrap(err, "failed to write tags table"))
	}

	return errors.Join(errs...)
}

// collectMemos gathers every memo (archived included) across the given cases.
func (e *Exporter) collectMemos(ctx context.Context, wsID string, cases []*model.Case) ([]*model.Memo, error) {
	var memos []*model.Memo
	for _, c := range cases {
		ms, err := e.repo.Memo().List(ctx, wsID, c.ID, interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeAll,
		})
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list memos", goerr.V("case_id", c.ID))
		}
		memos = append(memos, ms...)
	}
	return memos, nil
}

// exportJobRunHistory writes the three case-scoped agent run tables: the
// per-(case, job) summaries, the run logs filed under them, and each run's event
// timeline. Each level is only reachable through the level above it — a
// JobRunLog needs the summary's JobRunKey, and an event list needs the log's
// RunID — so a level whose parent failed is not attempted at all.
//
// Agent run history is Case-scoped and read per Case, so iterating the kept
// cases excludes an excluded Case's runs the same way memos are excluded.
//
// Each table is published only if its own level was read in full: every write is
// a full refresh, so publishing a partial read would delete rows that are still
// there. A read failure therefore returns before Commit, and the destination
// keeps the previous snapshot.
//
// This is the export's most read-heavy step: one subcollection scan per case,
// one log query per (case, job) pair, and one event query per run. Every
// mention-triggered run carries its own fresh JobID (see
// model.EventTypeMention), so a busy Case contributes one summary doc, one log
// query and one event query per mention turn. The queries run serially; if the
// volume grows enough for the round trips to dominate, bounded concurrency here
// is the lever.
func (e *Exporter) exportJobRunHistory(ctx context.Context, wsID, ns string, cases []*model.Case) []error {
	var errs []error

	var runs []*model.JobRun
	for _, c := range cases {
		rs, err := e.repo.JobRun().ListByCase(ctx, wsID, c.ID)
		if err != nil {
			return append(errs, goerr.Wrap(err, "failed to list job runs",
				goerr.V("case_id", c.ID)))
		}
		runs = append(runs, rs...)
	}
	if err := e.writeTable(ctx, ns, "job_runs", jobRunColumns(), jobRunRows(runs)); err != nil {
		errs = append(errs, goerr.Wrap(err, "failed to write job runs table"))
	}

	var logs []*model.JobRunLog
	for _, r := range runs {
		// limit 0 means no limit: the export is a full snapshot, so it must not
		// silently drop a Job's older runs.
		ls, err := e.repo.JobRunLog().List(ctx, r.Key(), 0)
		if err != nil {
			return append(errs, goerr.Wrap(err, "failed to list job run logs",
				goerr.V("case_id", r.CaseID), goerr.V("job_id", r.JobID)))
		}
		logs = append(logs, ls...)
	}
	if err := e.writeTable(ctx, ns, "job_run_logs", jobRunLogColumns(), jobRunLogRows(logs)); err != nil {
		errs = append(errs, goerr.Wrap(err, "failed to write job run logs table"))
	}

	if err := e.exportJobRunEvents(ctx, ns, logs); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// exportJobRunEvents streams the job_run_events table: one repository read per
// run, appended and released before the next run is read.
//
// This is the one table read at sub-table granularity, and deliberately so. It
// is the only one whose volume is unbounded by anything the product limits — a
// single workspace's timeline reaches gigabytes, two orders of magnitude past
// every other table here — so it is the only one worth the extra shape. The
// others stay in memory.
//
// A failed read returns without Commit, so the deferred Abort drops the staging
// data and the destination is left holding the previous snapshot: a partial
// timeline is never published as a full refresh.
func (e *Exporter) exportJobRunEvents(ctx context.Context, ns string, logs []*model.JobRunLog) error {
	w, err := e.sink.BeginTable(ctx, ns, e.tableName("job_run_events"), jobRunEventColumns())
	if err != nil {
		return goerr.Wrap(err, "failed to begin job run events table")
	}
	defer w.Abort(ctx)

	for _, l := range logs {
		key := model.JobRunKey{WorkspaceID: l.WorkspaceID, CaseID: l.CaseID, JobID: l.JobID}
		evs, err := e.repo.JobRunEvent().List(ctx, key, l.RunID)
		if err != nil {
			return goerr.Wrap(err, "failed to list job run events",
				goerr.V("case_id", l.CaseID), goerr.V("job_id", l.JobID),
				goerr.V("run_id", l.RunID))
		}
		if len(evs) == 0 {
			continue
		}
		if err := w.AppendRows(ctx, jobRunEventRows(ctx, evs)); err != nil {
			return goerr.Wrap(err, "failed to append job run event rows",
				goerr.V("case_id", l.CaseID), goerr.V("job_id", l.JobID),
				goerr.V("run_id", l.RunID))
		}
	}
	if err := w.Commit(ctx); err != nil {
		return goerr.Wrap(err, "failed to write job run events table")
	}
	return nil
}

// filterNonPrivate returns only the cases that are not private.
func filterNonPrivate(cases []*model.Case) []*model.Case {
	out := make([]*model.Case, 0, len(cases))
	for _, c := range cases {
		if !c.IsPrivate {
			out = append(out, c)
		}
	}
	return out
}

// caseIDSet indexes the given cases by ID.
func caseIDSet(cases []*model.Case) map[int64]bool {
	s := make(map[int64]bool, len(cases))
	for _, c := range cases {
		s[c.ID] = true
	}
	return s
}

// filterActionsByCases keeps only the actions whose parent Case is in keep.
func filterActionsByCases(actions []*model.Action, keep map[int64]bool) []*model.Action {
	out := make([]*model.Action, 0, len(actions))
	for _, a := range actions {
		if keep[a.CaseID] {
			out = append(out, a)
		}
	}
	return out
}
