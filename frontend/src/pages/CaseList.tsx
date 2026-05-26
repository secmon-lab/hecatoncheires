import { useState, useMemo, useEffect, useRef, useCallback } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { GET_CASES } from '../graphql/case'
import { GET_DRAFTS } from '../graphql/drafts'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import BulkSelectionBar from '../components/BulkSelectionBar'
import BulkDeleteConfirmDialog from '../components/BulkDeleteConfirmDialog'
import BulkResultDialog from '../components/BulkResultDialog'
import {
  IconPlus,
  IconSearch,
  IconLock,
  IconChevLeft,
  IconChevRight,
  IconDots,
  IconSettings,
} from '../components/Icons'
import { Avatar, AssigneeNamesStack, StatusBadge, SlackLink } from '../components/Primitives'
import CaseForm from './CaseForm'
import { displayName } from '../utils/user'
import {
  useBulkDraftAction,
  type BulkActionKind,
  type BulkActionResult,
} from '../hooks/useBulkDraftAction'

const PAGE_SIZE = 20

type StatusFilter = 'OPEN' | 'CLOSED' | 'ALL' | 'DRAFT'

// URL representation of the tab. Lower-case so the query string stays
// readable; OPEN is the implicit default and is never emitted.
type StatusQuery = 'closed' | 'draft' | 'all'
export const CASE_LIST_STATUS_PARAM = 'status'

function parseStatusFilter(raw: string | null): StatusFilter {
  switch ((raw ?? '').toLowerCase()) {
    case 'closed': return 'CLOSED'
    case 'draft': return 'DRAFT'
    case 'all': return 'ALL'
    case 'open': return 'OPEN'
    default: return 'OPEN'
  }
}

function statusToQuery(filter: StatusFilter): StatusQuery | undefined {
  switch (filter) {
    case 'CLOSED': return 'closed'
    case 'DRAFT': return 'draft'
    case 'ALL': return 'all'
    case 'OPEN': return undefined
  }
}

interface FieldOption {
  id: string
  name: string
  color?: string | null
}
interface FieldDef {
  id: string
  name: string
  type: string
  options?: FieldOption[] | null
}
interface CaseUser {
  id: string
  name: string
  realName: string
  imageUrl?: string
}
interface CaseRow {
  id: number
  title: string
  status: 'OPEN' | 'CLOSED' | 'DRAFT'
  isPrivate: boolean
  accessDenied: boolean
  reporterID?: string | null
  reporter?: CaseUser | null
  assignees: CaseUser[]
  slackChannelID: string
  createdAt: string
  fields: Array<{ fieldId: string; value: any }>
}

const BUILTIN_COLUMNS = [
  { key: 'status', labelKey: 'headerStatus' as const, width: 110 },
  { key: 'assignees', labelKey: 'headerAssignees' as const, width: 140 },
  { key: 'reporter', labelKey: 'labelReporter' as const, width: 140 },
  { key: 'created', labelKey: 'headerCreated' as const, width: 110 },
  { key: 'slack', labelKey: 'headerSlack' as const, width: 110 },
] as const

const DEFAULT_VISIBLE = ['status', 'assignees', 'reporter', 'created', 'slack']

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}/${mm}/${dd}`
}

function renderFieldValue(value: any, def: FieldDef): React.ReactNode {
  if (value == null || value === '') return <span className="soft">—</span>
  switch (def.type) {
    case 'SELECT': {
      const opt = def.options?.find((o) => o.id === value || o.name === value)
      const text = opt?.name ?? String(value)
      return <span className="badge">{text}</span>
    }
    case 'MULTI_SELECT': {
      const arr: any[] = Array.isArray(value) ? value : [value]
      return (
        <div className="row" style={{ gap: 4, flexWrap: 'wrap' }}>
          {arr.map((v) => {
            const opt = def.options?.find((o) => o.id === v || o.name === v)
            return <span key={String(v)} className="chip" style={{ height: 20, fontSize: 11 }}>{opt?.name ?? String(v)}</span>
          })}
        </div>
      )
    }
    case 'DATE': {
      try { return <span className="mono soft" style={{ fontSize: 12 }}>{new Date(value).toLocaleDateString()}</span> } catch { return String(value) }
    }
    case 'NUMBER':
      return <span className="mono">{String(value)}</span>
    case 'URL':
      return (
        <a href={String(value)} target="_blank" rel="noreferrer noopener" style={{ color: 'var(--accent)' }} onClick={(e) => e.stopPropagation()}>
          {String(value)}
        </a>
      )
    case 'USER': {
      // value is a slackUserID; fall back to mono id since we only have id here
      return <span className="mono soft" style={{ fontSize: 12 }}>{String(value)}</span>
    }
    case 'MULTI_USER': {
      const arr: any[] = Array.isArray(value) ? value : [value]
      return <span className="mono soft" style={{ fontSize: 12 }}>{arr.length} users</span>
    }
    default:
      return <span className="truncate" style={{ display: 'inline-block', maxWidth: 220 }}>{String(value)}</span>
  }
}

export default function CaseList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const wsKey = currentWorkspace?.id || 'default'
  const storageKey = `caseListColumns:${wsKey}`

  // The selected tab lives in the URL (`?status=closed|draft|all`) so
  // that navigating away and back — via the case detail page, browser
  // back, or a shared link — restores whichever tab the user was on.
  // `OPEN` is the implicit default and is represented by the query
  // being absent.
  const [searchParams, setSearchParams] = useSearchParams()
  const statusFilter: StatusFilter = parseStatusFilter(searchParams.get(CASE_LIST_STATUS_PARAM))

  const setStatusFilter = useCallback(
    (next: StatusFilter) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev)
          const q = statusToQuery(next)
          if (q) params.set(CASE_LIST_STATUS_PARAM, q)
          else params.delete(CASE_LIST_STATUS_PARAM)
          return params
        },
        { replace: true },
      )
    },
    [setSearchParams],
  )

  const [searchText, setSearchText] = useState('')
  const [page, setPage] = useState(0)
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [columnsOpen, setColumnsOpen] = useState(false)
  const columnsBtnRef = useRef<HTMLDivElement>(null)

  const [visibleCols, setVisibleCols] = useState<string[]>(() => {
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) return JSON.parse(raw)
    } catch {}
    return DEFAULT_VISIBLE
  })

  useEffect(() => {
    try { localStorage.setItem(storageKey, JSON.stringify(visibleCols)) } catch {}
  }, [storageKey, visibleCols])

  useEffect(() => {
    if (!columnsOpen) return
    const onClick = (e: MouseEvent) => {
      if (columnsBtnRef.current && !columnsBtnRef.current.contains(e.target as Node)) {
        setColumnsOpen(false)
      }
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [columnsOpen])

  const { data: openData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' },
    skip: !currentWorkspace,
  })
  const { data: closedData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace?.id, status: 'CLOSED' },
    skip: !currentWorkspace,
  })
  // Drafts are workspace-wide on the server; this query drives both the
  // Drafts tab and the sidebar / header count.
  const { data: draftData, refetch: refetchDrafts } = useQuery(GET_DRAFTS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const openCount = openData?.cases?.length ?? 0
  const closedCount = closedData?.cases?.length ?? 0
  const draftCount = draftData?.drafts?.length ?? 0

  const cases: CaseRow[] = useMemo(() => {
    if (statusFilter === 'OPEN') return openData?.cases || []
    if (statusFilter === 'CLOSED') return closedData?.cases || []
    if (statusFilter === 'DRAFT') return draftData?.drafts || []
    return [...(openData?.cases || []), ...(closedData?.cases || [])]
  }, [statusFilter, openData, closedData, draftData])

  const filtered = useMemo(() => {
    if (!searchText.trim()) return cases
    const q = searchText.toLowerCase()
    return cases.filter((c) => !c.accessDenied && c.title.toLowerCase().includes(q))
  }, [cases, searchText])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageRows = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)

  const fieldDefs: FieldDef[] = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || t('navCases')

  // Bulk selection state — only used when the Drafts tab is active.
  // Storing as a Set keeps add/remove/lookup O(1) and survives across
  // pagination as long as the user stays on the Drafts tab.
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [resultDialog, setResultDialog] = useState<
    { open: boolean; kind: BulkActionKind; results: BulkActionResult[] }
  >({ open: false, kind: 'submit', results: [] })
  const { state: bulkState, run: runBulk } = useBulkDraftAction()

  // Leaving the Drafts tab (or switching workspace) drops the selection so
  // the user does not return to a mismatched state.
  useEffect(() => {
    if (statusFilter !== 'DRAFT') {
      setSelectedIds((prev) => (prev.size === 0 ? prev : new Set()))
    }
  }, [statusFilter, wsKey])

  // Drafts can disappear between renders (other tab's mutations, draft TTL
  // expiry). Drop selections for IDs that no longer exist so the action
  // count stays honest.
  //
  // Return null while the drafts query has not produced data yet — a
  // refetch / network blip can briefly null out `draftData`, and without
  // this guard we would interpret "no data" as "every draft is gone" and
  // wipe the user's selection.
  const draftIdSet = useMemo(() => {
    if (!draftData?.drafts) return null
    const ids = new Set<number>()
    for (const d of draftData.drafts) ids.add(d.id)
    return ids
  }, [draftData])
  useEffect(() => {
    if (!draftIdSet) return
    setSelectedIds((prev) => {
      let changed = false
      const next = new Set<number>()
      for (const id of prev) {
        if (draftIdSet.has(id)) next.add(id)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [draftIdSet])

  const allColumns = [
    ...BUILTIN_COLUMNS.map((c) => ({ key: c.key, label: t(c.labelKey), width: c.width, custom: false as const })),
    ...fieldDefs.map((f) => ({ key: `field:${f.id}`, label: f.name, width: 160, custom: true as const, def: f })),
  ]

  const isVisible = (key: string) => visibleCols.includes(key)
  const toggleColumn = (key: string) => {
    setVisibleCols((prev) => prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key])
  }

  const renderCell = (col: typeof allColumns[number], c: CaseRow) => {
    if (!col.custom) {
      switch (col.key) {
        case 'status':
          return <StatusBadge status={c.status} labelOpen={t('statusOpen')} labelClosed={t('statusClosed')} labelDraft={t('tabDrafts')} />
        case 'assignees':
          return <AssigneeNamesStack users={c.assignees ?? []} testId="case-row-assignees" />
        case 'reporter':
          if (c.reporter) {
            return (
              <div className="row" style={{ gap: 6, fontSize: 12 }}>
                <Avatar size="sm" name={c.reporter.name} realName={c.reporter.realName} imageUrl={c.reporter.imageUrl} />
                <span className="truncate" style={{ maxWidth: 100 }}>{displayName(c.reporter)}</span>
              </div>
            )
          }
          if (c.reporterID) {
            return (
              <div className="row" style={{ gap: 6, fontSize: 12 }}>
                <Avatar size="sm" name={c.reporterID} realName={c.reporterID} />
                <span className="truncate mono soft" style={{ maxWidth: 100, fontSize: 11 }}>{c.reporterID}</span>
              </div>
            )
          }
          return <span className="soft">—</span>
        case 'created':
          return <span className="mono soft" style={{ fontSize: 12 }}>{formatDate(c.createdAt)}</span>
        case 'slack':
          return c.slackChannelID
            ? <SlackLink name="" href={`slack://channel?id=${c.slackChannelID}`} />
            : <span className="soft">—</span>
      }
    } else {
      const fieldDef = col.def!
      const v = c.fields.find((cf) => cf.fieldId === fieldDef.id)?.value
      return renderFieldValue(v, fieldDef)
    }
    return null
  }

  const visibleColumns = allColumns.filter((c) => isVisible(c.key))

  const isDraftsTab = statusFilter === 'DRAFT'

  // Drafts the user is allowed to select. accessDenied rows have an
  // opaque title and we cannot act on them server-side either, so they
  // are excluded from select-all and from the per-row checkbox.
  const selectableDrafts = useMemo(() => {
    if (!isDraftsTab) return [] as CaseRow[]
    return filtered.filter((c) => !c.accessDenied)
  }, [isDraftsTab, filtered])

  // Three-state checkbox state for the header: all / some / none of the
  // selectable drafts (across pages) are selected.
  const allSelectableIds = useMemo(
    () => selectableDrafts.map((c) => c.id),
    [selectableDrafts],
  )
  const allSelected =
    allSelectableIds.length > 0 && allSelectableIds.every((id) => selectedIds.has(id))
  const someSelected =
    !allSelected && allSelectableIds.some((id) => selectedIds.has(id))

  const headerCheckboxRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (headerCheckboxRef.current) {
      headerCheckboxRef.current.indeterminate = someSelected
    }
  }, [someSelected])

  const toggleAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (allSelected) {
        // Clear only the IDs we own — keep any IDs from filtered-out
        // searches that the user may want to retain when search clears.
        const next = new Set(prev)
        for (const id of allSelectableIds) next.delete(id)
        return next
      }
      const next = new Set(prev)
      for (const id of allSelectableIds) next.add(id)
      return next
    })
  }, [allSelected, allSelectableIds])

  const toggleRow = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const clearSelection = useCallback(() => {
    setSelectedIds((prev) => (prev.size === 0 ? prev : new Set()))
  }, [])

  const selectedDrafts = useMemo(() => {
    if (!isDraftsTab) return [] as { id: number; title: string }[]
    return selectableDrafts
      .filter((c) => selectedIds.has(c.id))
      .map((c) => ({ id: c.id, title: c.title }))
  }, [isDraftsTab, selectableDrafts, selectedIds])

  const performBulk = useCallback(
    async (kind: BulkActionKind) => {
      if (!currentWorkspace || selectedDrafts.length === 0) return
      const results = await runBulk(kind, {
        workspaceId: currentWorkspace.id,
        drafts: selectedDrafts,
      })
      // Refetch so successful drafts disappear from the list (Submit
      // promotes to OPEN; Discard removes the row). Failed ones stay so
      // the user can edit and retry.
      void refetchDrafts()
      // Drop selections of drafts that succeeded; keep failures selected
      // so the user can re-act on them after fixing.
      setSelectedIds((prev) => {
        const next = new Set(prev)
        for (const r of results) if (r.ok) next.delete(r.id)
        return next
      })
      setResultDialog({ open: true, kind, results })
    },
    [currentWorkspace, selectedDrafts, runBulk, refetchDrafts],
  )

  const handleBulkSubmit = useCallback(() => {
    void performBulk('submit')
  }, [performBulk])

  const handleBulkDeleteRequest = useCallback(() => {
    if (selectedDrafts.length === 0) return
    setConfirmDeleteOpen(true)
  }, [selectedDrafts.length])

  const handleBulkDeleteConfirm = useCallback(() => {
    setConfirmDeleteOpen(false)
    void performBulk('discard')
  }, [performBulk])

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('titleCaseManagement', { caseLabel })}</h1>
          <div className="sub">{t('subtitleCaseManagement', { caseLabelLower: caseLabel.toLowerCase() })}</div>
        </div>
        <div className="actions">
          <div ref={columnsBtnRef} style={{ position: 'relative' }}>
            <Button
              icon={<IconSettings size={14} />}
              onClick={() => setColumnsOpen((v) => !v)}
              data-testid="column-selector-button"
            >
              {t('btnColumns')}
            </Button>
            {columnsOpen && (
              <div
                data-testid="column-selector-popover"
                style={{
                  position: 'absolute', right: 0, top: 'calc(100% + 6px)',
                  zIndex: 50, minWidth: 220,
                  background: 'var(--bg-elev)', border: '1px solid var(--line)',
                  borderRadius: 6, boxShadow: 'var(--shadow-md)', padding: 6,
                }}
              >
                <div className="soft" style={{ fontSize: 11, padding: '4px 8px', textTransform: 'uppercase', letterSpacing: '0.05em', fontWeight: 600 }}>
                  {t('titleColumnSelector')}
                </div>
                {allColumns.map((c) => (
                  <label
                    key={c.key}
                    data-testid={`column-toggle-${c.key}`}
                    className="row"
                    style={{ gap: 8, padding: '6px 8px', cursor: 'pointer', fontSize: 12.5, borderRadius: 4 }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-sunken)')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    <input
                      type="checkbox"
                      checked={isVisible(c.key)}
                      onChange={() => toggleColumn(c.key)}
                    />
                    <span>{c.label}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          <Button
            variant="secondary"
            onClick={() => navigate(`/ws/${currentWorkspace!.id}/imports/new`)}
          >
            {t('btnImport')}
          </Button>
          <Button variant="primary" icon={<IconPlus size={14} />} onClick={() => setIsFormOpen(true)}>
            {t('btnNewCase', { caseLabel })}
          </Button>
        </div>
      </div>

      <div className="row" style={{ marginBottom: 12, gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <div className="seg">
          <button
            className={statusFilter === 'OPEN' ? 'on' : ''}
            onClick={() => { setStatusFilter('OPEN'); setPage(0) }}
            data-testid="status-tab-open"
          >
            {t('tabOpen')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{openCount}</span>
          </button>
          <button
            className={statusFilter === 'CLOSED' ? 'on' : ''}
            onClick={() => { setStatusFilter('CLOSED'); setPage(0) }}
            data-testid="status-tab-closed"
          >
            {t('tabClosed')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{closedCount}</span>
          </button>
          <button
            className={statusFilter === 'DRAFT' ? 'on' : ''}
            onClick={() => { setStatusFilter('DRAFT'); setPage(0) }}
            data-testid="status-tab-draft"
          >
            {t('tabDrafts')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{draftCount}</span>
          </button>
          <button
            className={statusFilter === 'ALL' ? 'on' : ''}
            onClick={() => { setStatusFilter('ALL'); setPage(0) }}
          >
            {t('tabAll')}
          </button>
        </div>
        {isDraftsTab && (
          <BulkSelectionBar
            selectedCount={selectedDrafts.length}
            onSubmit={handleBulkSubmit}
            onDelete={handleBulkDeleteRequest}
            onClear={clearSelection}
            disabled={bulkState.loading}
            progressLabel={
              bulkState.loading
                ? t('bulkProgress', { done: bulkState.done, total: bulkState.total })
                : undefined
            }
          />
        )}
        <span className="spacer" />
        <div className="h-search" style={{ width: 260, marginLeft: 0 }}>
          <IconSearch size={13} />
          <input
            value={searchText}
            onChange={(e) => { setSearchText(e.target.value); setPage(0) }}
            placeholder={t('placeholderSearchByTitle')}
            data-testid="search-filter"
            style={{
              flex: 1, border: 'none', background: 'transparent', outline: 'none',
              fontFamily: 'inherit', color: 'var(--fg)', fontSize: 12.5,
            }}
          />
        </div>
      </div>

      <div className="card" style={{ overflow: 'hidden' }}>
        <table className="h-table">
          <thead>
            <tr>
              {isDraftsTab && (
                <th style={{ width: 36 }}>
                  <input
                    ref={headerCheckboxRef}
                    type="checkbox"
                    data-testid="bulk-header-checkbox"
                    aria-label={t('bulkSelectAllAria')}
                    checked={allSelected}
                    onChange={toggleAll}
                    disabled={allSelectableIds.length === 0 || bulkState.loading}
                  />
                </th>
              )}
              <th style={{ width: 64 }}>{t('labelId')}</th>
              <th>{t('headerTitle')}</th>
              {visibleColumns.map((c) => (
                <th key={c.key} style={{ width: c.width }}>{c.label}</th>
              ))}
              <th style={{ width: 38 }}></th>
            </tr>
          </thead>
          <tbody>
            {pageRows.length === 0 && (
              <tr>
                <td
                  colSpan={3 + visibleColumns.length + (isDraftsTab ? 1 : 0)}
                  style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}
                >
                  {t('noDataAvailable')}
                </td>
              </tr>
            )}
            {pageRows.map((c) => {
              const rowSelected = isDraftsTab && selectedIds.has(c.id)
              return (
                <tr
                  key={c.id}
                  onClick={() => {
                    if (c.accessDenied) return
                    // Drafts share the regular case detail page — Submit /
                    // Discard surface there based on status.
                    // Pass the active tab through location.state so that
                    // the detail page's back/delete/discard handlers can
                    // return to the same tab.
                    navigate(`/ws/${currentWorkspace!.id}/cases/${c.id}`, {
                      state: { fromStatus: statusToQuery(statusFilter) },
                    })
                  }}
                  style={{
                    cursor: c.accessDenied ? 'default' : 'pointer',
                    background: rowSelected ? 'var(--bg-highlight)' : undefined,
                  }}
                >
                  {isDraftsTab && (
                    <td style={{ width: 36 }} onClick={(e) => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        data-testid={`bulk-row-checkbox-${c.id}`}
                        aria-label={t('bulkSelectRowAria', { id: c.id })}
                        checked={selectedIds.has(c.id)}
                        onChange={() => toggleRow(c.id)}
                        disabled={c.accessDenied || bulkState.loading}
                      />
                    </td>
                  )}
                  <td className="id mono">#{c.id}</td>
                  <td>
                    <div className="row" style={{ gap: 8 }}>
                      {c.isPrivate && (
                        <span title={t('badgePrivate')} data-testid="private-lock-icon" style={{ color: 'var(--warn)', display: 'inline-flex' }}>
                          <IconLock size={12} sw={2} />
                        </span>
                      )}
                      {c.accessDenied ? (
                        <span data-testid="access-denied-label" className="muted" style={{ fontStyle: 'italic' }}>
                          {t('badgePrivate')}
                        </span>
                      ) : (
                        <span className="title truncate" style={{ maxWidth: 380 }}>{c.title}</span>
                      )}
                    </div>
                  </td>
                  {visibleColumns.map((col) => (
                    <td key={col.key}>{renderCell(col, c)}</td>
                  ))}
                  <td>
                    <button
                      className="h-icon-btn"
                      style={{ width: 24, height: 24 }}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <IconDots size={14} />
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        <div
          data-testid="pagination"
          style={{
            padding: '10px 16px', display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            fontSize: 12, color: 'var(--fg-muted)', borderTop: '1px solid var(--line)',
          }}
        >
          <span>
            {filtered.length === 0
              ? '0–0 / 0'
              : `${page * PAGE_SIZE + 1}–${Math.min((page + 1) * PAGE_SIZE, filtered.length)} / ${filtered.length}`}
          </span>
          <div className="row" style={{ gap: 6 }}>
            <Button
              size="sm"
              icon={<IconChevLeft size={12} />}
              disabled={page === 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              data-testid="pagination-prev"
            >
              {t('btnPrevious')}
            </Button>
            <span className="mono" data-testid="pagination-info">{page + 1} / {totalPages}</span>
            <Button
              size="sm"
              icon={<IconChevRight size={12} />}
              disabled={page >= totalPages - 1}
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              data-testid="pagination-next"
            >
              {t('btnNext')}
            </Button>
          </div>
        </div>
      </div>

      {isFormOpen && (
        <CaseForm caseItem={null} onClose={() => setIsFormOpen(false)} />
      )}

      <BulkDeleteConfirmDialog
        open={confirmDeleteOpen}
        count={selectedDrafts.length}
        previewTitles={selectedDrafts.map((d) => d.title)}
        onConfirm={handleBulkDeleteConfirm}
        onCancel={() => setConfirmDeleteOpen(false)}
        disabled={bulkState.loading}
      />

      <BulkResultDialog
        open={resultDialog.open}
        kind={resultDialog.kind}
        results={resultDialog.results}
        onClose={() => setResultDialog((prev) => ({ ...prev, open: false }))}
      />
    </div>
  )
}
