// Package export implements the `export` subcommand's core: reading the current
// state of every configured workspace out of the repository and writing it, one
// table per entity, to a pluggable Sink (BigQuery today). The Exporter is
// sink-agnostic — it builds a generic Table (typed columns + rows of natural Go
// values) and hands it to the Sink, which owns all backend-specific concerns
// (schema evolution, full-refresh, encoding). This keeps a future Cloud Storage
// sink a drop-in without touching the read/normalize logic here.
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

// Table is a full-refresh unit: a named table with a typed schema and its rows.
// Row values are natural Go types keyed by column name; a missing key is NULL.
// Backend-specific encoding (e.g. TIMESTAMP -> microseconds) is the Sink's job.
type Table struct {
	Name    string
	Columns []Column
	Rows    []map[string]any
}

// Sink is a destination that fully replaces (洗替) a table's schema and rows.
// Implementations MUST make each WriteTable a full refresh of the named table
// within the given namespace.
type Sink interface {
	// WriteTable replaces the table's schema and data within namespace.
	WriteTable(ctx context.Context, namespace string, table *Table) error
	io.Closer
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

// writeTable applies the (optional) table-name prefix and hands the table to the
// sink.
func (e *Exporter) writeTable(ctx context.Context, namespace string, t *Table) error {
	if e.tablePrefix != "" {
		t.Name = e.tablePrefix + t.Name
	}
	return e.sink.WriteTable(ctx, namespace, t)
}

// Run exports every target. A failure on one target is logged and collected but
// does not stop the others; all collected failures are returned joined so the
// caller (and its error reporter) sees every one.
//
// Single-instance assumption: Run is designed to be invoked by ONE process at a
// time. The per-table refresh (TRUNCATE then append via the sink) is not atomic
// and takes no distributed lock, so two overlapping exports of the same
// destination can interleave and duplicate rows. This is a deliberate,
// documented constraint (the export is a singly-run batch job, e.g. a scheduled
// task with no overlap), not an oversight — see docs/export.md.
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

// exportWorkspace writes all five entity tables for one workspace. Per-table
// failures are collected (not fail-fast) so one bad table does not hide the
// rest, and are returned joined.
func (e *Exporter) exportWorkspace(ctx context.Context, t Target) error {
	wsID := t.Entry.Workspace.ID
	ns := t.Namespace
	var errs []error

	// Cases (drafts excluded by List; private filtered here when configured).
	cases, casesErr := e.repo.Case().List(ctx, wsID)
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

		if err := e.writeTable(ctx, ns, buildCaseTable(ctx, t.Entry.FieldSchema, cases)); err != nil {
			errs = append(errs, goerr.Wrap(err, "failed to write cases table"))
		}

		// Memos are Case-scoped: iterating the kept cases naturally excludes the
		// memos of any excluded Case.
		memos, memoErr := e.collectMemos(ctx, wsID, cases)
		if memoErr != nil {
			errs = append(errs, memoErr)
		} else if err := e.writeTable(ctx, ns, buildMemoTable(ctx, t.Entry.MemoConfig, memos)); err != nil {
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
			if err := e.writeTable(ctx, ns, buildActionTable(actions)); err != nil {
				errs = append(errs, goerr.Wrap(err, "failed to write actions table"))
			}
		}
	}

	// Knowledge / Tag are workspace-level (not Case-scoped), always exported.
	knowledge, knowledgeErr := e.repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{})
	if knowledgeErr != nil {
		errs = append(errs, goerr.Wrap(knowledgeErr, "failed to list knowledge"))
	} else if err := e.writeTable(ctx, ns, buildKnowledgeTable(knowledge)); err != nil {
		errs = append(errs, goerr.Wrap(err, "failed to write knowledge table"))
	}

	tags, tagsErr := e.repo.Tag().List(ctx, wsID)
	if tagsErr != nil {
		errs = append(errs, goerr.Wrap(tagsErr, "failed to list tags"))
	} else if err := e.writeTable(ctx, ns, buildTagTable(tags)); err != nil {
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
