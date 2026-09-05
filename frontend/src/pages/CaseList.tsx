import { useState, useMemo, useEffect, useRef, useCallback } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import { Link, useNavigate, useSearchParams } from 'react-router'
import {
  BULK_ARCHIVE_CASES,
  BULK_UNARCHIVE_CASES,
  GET_CASES_WITH_SLACK_LINK,
} from '../graphql/case'
import { GET_DRAFTS } from '../graphql/drafts'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import BulkSelectionBar, { type BulkAction } from '../components/BulkSelectionBar'
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
import {
  Avatar,
  AssigneeNamesStack,
  BoardStatusBadge,
  StatusBadge,
  SlackLink,
  TestBadge,
} from '../components/Primitives'
import CaseForm from './CaseForm'
import { useCaseStatuses } from '../hooks/useCaseStatuses'
import { buildSlackCaseLink } from '../utils/slackLink'
import { displayName } from '../utils/user'
import {
  useBulkDraftAction,
  type BulkActionKind,
  type BulkActionResult,
} from '../hooks/useBulkDraftAction'
import styles from './CaseList.module.css'

// Rows rendered per page. 20 stays the default so the list opens as compactly
// as before; the larger options let a user scan a whole workspace at once.
const PAGE_SIZE_OPTIONS = [20, 50, 100, 200] as const
const DEFAULT_PAGE_SIZE = 20
// Not workspace-scoped: how many rows fit on screen is a property of the
// user's display, not of the workspace being viewed.
const PAGE_SIZE_STORAGE_KEY = 'caseListPageSize'

type StatusFilter = 'OPEN' | 'CLOSED' | 'ALL' | 'DRAFT' | 'ARCHIVED'

// URL representation of the tab. Lower-case so the query string stays
// readable; OPEN is the implicit default and is never emitted.
type StatusQuery = 'closed' | 'draft' | 'all' | 'archived'
export const CASE_LIST_STATUS_PARAM = 'status'
export const CASE_LIST_PAGE_PARAM = 'page'

// Tabs that offer row selection and a bulk action bar. ALL and OPEN are
// excluded: a bulk archive there would mix cases the operation cannot apply to
// (archiving is CLOSED-only) with ones it can.
const SELECTABLE_TABS: readonly StatusFilter[] = ['DRAFT', 'CLOSED', 'ARCHIVED']

function parsePageSize(raw: string | null): number {
  const n = Number(raw)
  return PAGE_SIZE_OPTIONS.some((o) => o === n) ? n : DEFAULT_PAGE_SIZE
}

// `page` is 1-based in the URL so a shared or bookmarked link reads naturally;
// internally the list works with a 0-based index.
function parsePageIndex(raw: string | null): number {
  if (raw === null) return 0
  const n = Number(raw)
  if (!Number.isInteger(n) || n < 1) return 0
  return n - 1
}

function parseStatusFilter(raw: string | null): StatusFilter {
  switch ((raw ?? '').toLowerCase()) {
    case 'closed': return 'CLOSED'
    case 'draft': return 'DRAFT'
    case 'archived': return 'ARCHIVED'
    case 'all': return 'ALL'
    case 'open': return 'OPEN'
    default: return 'OPEN'
  }
}

function statusToQuery(filter: StatusFilter): StatusQuery | undefined {
  switch (filter) {
    case 'CLOSED': return 'closed'
    case 'DRAFT': return 'draft'
    case 'ARCHIVED': return 'archived'
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
  isTest: boolean
  accessDenied: boolean
  reporterID?: string | null
  reporter?: CaseUser | null
  assignees: CaseUser[]
  slackChannelID: string | null
  // Absent on Drafts-tab rows: GET_DRAFTS does not select these. A draft has
  // no Slack channel until it is submitted and no board status until then
  // either, so both fall back to the lifecycle rendering.
  slackChannelURL?: string | null
  slackThreadTS?: string | null
  boardStatus?: string | null
  // Null for an active case. There is no derived `archived` boolean on the
  // wire; a row is archived when this is non-null.
  archivedAt?: string | null
  createdAt: string
  updatedAt: string
  fields: Array<{ fieldId: string; value: any }>
}

const BUILTIN_COLUMNS = [
  { key: 'status', labelKey: 'headerStatus' as const, width: 110 },
  { key: 'assignees', labelKey: 'headerAssignees' as const, width: 140 },
  { key: 'reporter', labelKey: 'labelReporter' as const, width: 140 },
  { key: 'created', labelKey: 'headerCreated' as const, width: 110 },
  { key: 'updated', labelKey: 'headerUpdated' as const, width: 110 },
  { key: 'slack', labelKey: 'headerSlack' as const, width: 110 },
] as const

const DEFAULT_VISIBLE = ['status', 'assignees', 'reporter', 'created', 'updated', 'slack']

// The calendar day an RFC3339 timestamp falls on in the viewer's own timezone,
// as `YYYY-MM-DD` — the format <input type="date"> exchanges. Rendering and
// filtering share it so a row displaying "today" always matches today's filter
// value, which a UTC-based comparison would break either side of midnight.
function toLocalDateKey(iso: string) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}-${mm}-${dd}`
}

function formatDate(iso: string) {
  const key = toLocalDateKey(iso)
  return key ? key.replace(/-/g, '/') : '—'
}

// The Slack destination for a row. A thread-mode Case's channel is the
// monitored channel it was raised in, not a channel of its own, so linking to
// the channel alone lands the reader somewhere unrelated — the thread
// timestamp is what identifies the Case. Shared with the row-link logic so the
// two cannot disagree about whether the cell holds an <a>.
function slackCaseLink(c: CaseRow): string | null {
  return buildSlackCaseLink(c.slackChannelURL, c.slackChannelID, c.slackThreadTS)
}

// A blank value renders as a dash for every field type, which also means the
// cell holds no <a> even for a URL field. Shared with the row-link logic so
// the two cannot disagree about what "blank" means.
function isBlankFieldValue(value: any): boolean {
  return value == null || value === ''
}

function renderFieldValue(value: any, def: FieldDef): React.ReactNode {
  if (isBlankFieldValue(value)) return <span className="soft">—</span>
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

// The assignee filter's stand-in for "nobody is on this". A Slack user id can
// never collide with it, and "unassigned" is the state a triage view is most
// often looking for, so it belongs in the same list as the people.
const UNASSIGNED_FILTER_ID = '__unassigned__'

interface FilterOption {
  id: string
  label: string
  // Rendered before the label — a status dot, an avatar — so the row reads the
  // same way the column it filters does.
  swatch?: React.ReactNode
}

// MultiSelectFilter is one toolbar filter: a button that opens a checkbox list.
// Empty selection means "no filter", which is why the button shows a count only
// once something is picked — an unfiltered list should not look filtered.
function MultiSelectFilter({
  label, options, selected, onChange, testId,
}: {
  label: string
  options: FilterOption[]
  selected: string[]
  onChange: (next: string[]) => void
  testId: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  const toggle = (id: string) =>
    onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id])

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <Button
        variant={selected.length > 0 ? 'primary' : 'secondary'}
        onClick={() => setOpen((v) => !v)}
        data-testid={`${testId}-button`}
      >
        {selected.length > 0 ? `${label} · ${selected.length}` : label}
      </Button>
      {open && (
        <div
          data-testid={`${testId}-popover`}
          style={{
            position: 'absolute', left: 0, top: 'calc(100% + 6px)',
            zIndex: 50, minWidth: 200, maxHeight: 320, overflowY: 'auto',
            background: 'var(--bg-elev)', border: '1px solid var(--line)',
            borderRadius: 6, boxShadow: 'var(--shadow-md)', padding: 6,
          }}
        >
          {options.length === 0 ? (
            <div className="soft" style={{ fontSize: 12, padding: '6px 8px' }}>{t('filterNone')}</div>
          ) : (
            <>
              {options.map((o) => (
                <label
                  key={o.id}
                  data-testid={`${testId}-option-${o.id}`}
                  className="row"
                  style={{ gap: 8, padding: '6px 8px', cursor: 'pointer', fontSize: 12.5, borderRadius: 4 }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-sunken)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <input
                    type="checkbox"
                    checked={selected.includes(o.id)}
                    onChange={() => toggle(o.id)}
                  />
                  {o.swatch}
                  <span className="truncate">{o.label}</span>
                </label>
              ))}
              {selected.length > 0 && (
                <button
                  type="button"
                  data-testid={`${testId}-clear`}
                  onClick={() => onChange([])}
                  style={{
                    width: '100%', marginTop: 4, padding: '6px 8px', fontSize: 12,
                    background: 'transparent', border: 'none', borderTop: '1px solid var(--line)',
                    color: 'var(--fg-muted)', cursor: 'pointer', textAlign: 'left',
                    fontFamily: 'inherit',
                  }}
                >
                  {t('filterClear')}
                </button>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

export default function CaseList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const wsKey = currentWorkspace?.id || 'default'
  const storageKey = `caseListColumns:${wsKey}`

  // The selected tab and the current page live in the URL
  // (`?status=closed|draft|all&page=3`) so that navigating away and back —
  // via the case detail page, browser back, or a shared link — restores
  // both. `OPEN` and page 1 are the implicit defaults and are represented
  // by the query being absent.
  const [searchParams, setSearchParams] = useSearchParams()
  const statusFilter: StatusFilter = parseStatusFilter(searchParams.get(CASE_LIST_STATUS_PARAM))
  const requestedPage = parsePageIndex(searchParams.get(CASE_LIST_PAGE_PARAM))

  const setStatusFilter = useCallback(
    (next: StatusFilter) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev)
          const q = statusToQuery(next)
          if (q) params.set(CASE_LIST_STATUS_PARAM, q)
          else params.delete(CASE_LIST_STATUS_PARAM)
          // Row offsets are per-tab, so carrying the page across a tab
          // switch would drop the user at an unrelated offset.
          params.delete(CASE_LIST_PAGE_PARAM)
          return params
        },
        { replace: true },
      )
    },
    [setSearchParams],
  )

  // Paging replaces the history entry rather than pushing one: browser Back
  // should leave the list, not walk back through the pages the user flipped
  // through. The rewritten entry still carries `?page=N`, which is what
  // makes Back from a case detail land on the page it was opened from.
  const setPageIndex = useCallback(
    (next: number) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev)
          if (next > 0) params.set(CASE_LIST_PAGE_PARAM, String(next + 1))
          else params.delete(CASE_LIST_PAGE_PARAM)
          return params
        },
        { replace: true },
      )
    },
    [setSearchParams],
  )

  const [searchText, setSearchText] = useState('')
  // `YYYY-MM-DD` (the <input type="date"> value), or '' for no date filter.
  const [updatedOn, setUpdatedOn] = useState('')
  // Board status ids and assignee ids to keep. Empty means "no filter" for
  // both — a selection narrows, it never excludes on its own.
  const [boardStatusFilter, setBoardStatusFilter] = useState<string[]>([])
  const [assigneeFilter, setAssigneeFilter] = useState<string[]>([])
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [columnsOpen, setColumnsOpen] = useState(false)
  const columnsBtnRef = useRef<HTMLDivElement>(null)

  const [pageSize, setPageSize] = useState<number>(() => {
    try {
      return parsePageSize(localStorage.getItem(PAGE_SIZE_STORAGE_KEY))
    } catch {
      return DEFAULT_PAGE_SIZE
    }
  })

  const handlePageSizeChange = useCallback(
    (next: number) => {
      setPageSize(next)
      try { localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(next)) } catch {}
      // The page index means a different row range at the new size, so the
      // only offset that stays meaningful is the first one.
      setPageIndex(0)
    },
    [setPageIndex],
  )

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

  // The list queries use cache-and-network so navigating back to the Cases
  // page (e.g. after a YAML import created drafts, or a case was created /
  // closed elsewhere) always revalidates against the server instead of
  // showing a stale cached list. Without this, the cached result from an
  // earlier visit is returned verbatim and the freshly-created rows never
  // appear until a hard reload.
  //
  // errorPolicy 'all' keeps the rows when a nullable field resolver fails.
  // slackChannelURL resolves through Slack's auth.test, whose failure is
  // cached for the life of the process — under the default 'none' policy a
  // misconfigured Slack would blank the entire Case list instead of just
  // dropping the per-row Slack link.
  //
  // returnPartialData covers the other half of that split: everything else
  // refetches the narrower GET_CASES, which writes Cases carrying no
  // slackChannelURL into the same normalised entries this query watches. The
  // resulting cache read is incomplete, and without this the list would blank
  // out until the network round-trip lands. Partial rows simply fall back to
  // the slack.com link form.
  const { data: openData, refetch: refetchOpen } = useQuery(GET_CASES_WITH_SLACK_LINK, {
    variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' },
    skip: !currentWorkspace,
    fetchPolicy: 'cache-and-network',
    errorPolicy: 'all',
    returnPartialData: true,
  })
  const { data: closedData, refetch: refetchClosed } = useQuery(GET_CASES_WITH_SLACK_LINK, {
    variables: { workspaceId: currentWorkspace?.id, status: 'CLOSED' },
    skip: !currentWorkspace,
    fetchPolicy: 'cache-and-network',
    errorPolicy: 'all',
    returnPartialData: true,
  })
  // The Archived tab asks for the archived slice and no lifecycle status:
  // every archived case is CLOSED by construction, so a status filter would
  // only be a second way to say the same thing.
  const { data: archivedData, refetch: refetchArchived } = useQuery(GET_CASES_WITH_SLACK_LINK, {
    variables: { workspaceId: currentWorkspace?.id, filter: 'ARCHIVED' },
    skip: !currentWorkspace,
    fetchPolicy: 'cache-and-network',
    errorPolicy: 'all',
    returnPartialData: true,
  })
  // Drafts are workspace-wide on the server; this query drives both the
  // Drafts tab and the sidebar / header count.
  const { data: draftData, refetch: refetchDrafts } = useQuery(GET_DRAFTS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
    fetchPolicy: 'cache-and-network',
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  // Thread-mode workspaces track a configurable board status per Case; the
  // lifecycle OPEN/CLOSED is only its synced shadow. Null config (channel
  // mode, still loading, or a failed query) leaves the lifecycle rendering.
  const caseStatuses = useCaseStatuses(currentWorkspace?.id)

  const openCount = openData?.cases?.length ?? 0
  const closedCount = closedData?.cases?.length ?? 0
  const draftCount = draftData?.drafts?.length ?? 0
  const archivedCount = archivedData?.cases?.length ?? 0

  // Ids whose bulk archive / restore the server has accepted but not yet
  // finished. The mutation returns the accepted ids and processes them in the
  // background, so a refetch straight after cannot reflect completion — the
  // rows are removed here instead and the next natural load reconciles.
  //
  // Scoped to the tab the action was taken on, and cleared when the user
  // leaves it. Without the scope the same ids would also be subtracted from
  // the DESTINATION tab, so a case archived from Closed would be missing from
  // Archived (where it has just arrived) until a full page reload.
  const [pendingIds, setPendingIds] = useState<Set<number>>(() => new Set())

  const cases: CaseRow[] = useMemo(() => {
    if (statusFilter === 'OPEN') return openData?.cases || []
    if (statusFilter === 'CLOSED') return closedData?.cases || []
    if (statusFilter === 'DRAFT') return draftData?.drafts || []
    if (statusFilter === 'ARCHIVED') return archivedData?.cases || []
    // ALL view merges two separately-fetched lists, so re-sort the combined
    // result newest-first; otherwise all OPEN cases would precede all CLOSED.
    // createdAt is an RFC3339 UTC string, so lexicographic compare equals
    // chronological order without per-comparison Date allocation.
    //
    // Archived cases stay out: ALL means "every case the tabs beside it show",
    // and it already excludes drafts for the same reason.
    return [...(openData?.cases || []), ...(closedData?.cases || [])].sort(
      (a, b) => b.createdAt.localeCompare(a.createdAt)
    )
  }, [statusFilter, openData, closedData, draftData, archivedData])

  // Board statuses come from the workspace configuration rather than from the
  // rows, so a status nobody is currently in is still offered — "show me what
  // is waiting on us" must be answerable with zero as the answer. Channel-mode
  // workspaces have no board statuses and get no filter (the array is empty,
  // and the toolbar drops the control).
  const statusOptions: FilterOption[] = useMemo(
    () => (caseStatuses.config?.statuses ?? []).map((s) => ({ id: s.id, label: s.name })),
    [caseStatuses.config],
  )

  // The people to offer in the assignee filter: exactly those who appear on the
  // rows in view. Offering the whole workspace would list names that can only
  // ever return an empty page.
  const assigneeOptions = useMemo(() => {
    const seen = new Map<string, CaseUser>()
    for (const c of cases) {
      for (const a of c.assignees) if (!seen.has(a.id)) seen.set(a.id, a)
    }
    const people = [...seen.values()].sort((a, b) => displayName(a).localeCompare(displayName(b)))
    const anyUnassigned = cases.some((c) => c.assignees.length === 0)
    const options: FilterOption[] = people.map((u) => ({
      id: u.id,
      label: displayName(u),
      swatch: <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />,
    }))
    // Offered only when some row is actually unassigned, for the same reason.
    return anyUnassigned
      ? [{ id: UNASSIGNED_FILTER_ID, label: t('filterUnassigned') }, ...options]
      : options
  }, [cases, t])

  const filtered = useMemo(() => {
    const visible = pendingIds.size === 0 ? cases : cases.filter((c) => !pendingIds.has(c.id))
    const q = searchText.trim().toLowerCase()
    const byStatus = boardStatusFilter.length > 0
    const byAssignee = assigneeFilter.length > 0
    if (!q && !updatedOn && !byStatus && !byAssignee) return visible
    return visible.filter((c) => {
      // A restricted row carries no readable title, so the title search drops
      // it rather than matching against the redacted value. The other filters
      // have no such problem and leave those rows to stand on their own values.
      if (q && (c.accessDenied || !c.title.toLowerCase().includes(q))) return false
      if (updatedOn && toLocalDateKey(c.updatedAt) !== updatedOn) return false
      if (byStatus && !boardStatusFilter.includes(c.boardStatus ?? '')) return false
      if (byAssignee) {
        // Within a filter the selections are OR'd — picking two people means
        // "either of them" — while the filters themselves AND together.
        const hit = assigneeFilter.some((id) =>
          id === UNASSIGNED_FILTER_ID
            ? c.assignees.length === 0
            : c.assignees.some((a) => a.id === id),
        )
        if (!hit) return false
      }
      return true
    })
  }, [cases, searchText, updatedOn, boardStatusFilter, assigneeFilter, pendingIds])

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  // The URL can name a page that does not exist right now — the page size was
  // lowered, rows were filtered out, or the link was hand-edited. Clamp for
  // display instead of rewriting the query, so a list that has not finished
  // loading (0 rows, 1 page) does not overwrite a still-valid `?page=3`.
  const page = Math.min(requestedPage, totalPages - 1)
  const pageRows = filtered.slice(page * pageSize, (page + 1) * pageSize)

  const fieldDefs: FieldDef[] = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || t('navCases')

  // Bulk selection state — used on the Drafts, Closed and Archived tabs.
  // Storing as a Set keeps add/remove/lookup O(1) and survives across
  // pagination as long as the user stays on the same tab.
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [resultDialog, setResultDialog] = useState<
    { open: boolean; kind: BulkActionKind; results: BulkActionResult[] }
  >({ open: false, kind: 'submit', results: [] })
  const { state: bulkState, run: runBulk } = useBulkDraftAction()

  const [bulkArchiveCases, { loading: bulkArchiving }] = useMutation(BULK_ARCHIVE_CASES)
  const [bulkUnarchiveCases, { loading: bulkUnarchiving }] = useMutation(BULK_UNARCHIVE_CASES)

  const isDraftsTab = statusFilter === 'DRAFT'
  const isClosedTab = statusFilter === 'CLOSED'
  const isArchivedTab = statusFilter === 'ARCHIVED'
  const selectionEnabled = SELECTABLE_TABS.includes(statusFilter)
  const bulkBusy = bulkState.loading || bulkArchiving || bulkUnarchiving

  // Switching tab (or workspace) drops the selection: each tab selects from a
  // different set of rows and offers a different action, so carrying a
  // selection across would leave the count describing rows the user can no
  // longer see.
  useEffect(() => {
    setSelectedIds((prev) => (prev.size === 0 ? prev : new Set()))
    setPendingIds((prev) => (prev.size === 0 ? prev : new Set()))
  }, [statusFilter, wsKey])

  // Revalidate the tab the user just switched to. All five queries mount once
  // and stay mounted, so cache-and-network only revalidates them on the first
  // render — without this, a case moved between slices (archived from the
  // Closed tab, restored from the Archived tab, closed in another window) is
  // missing from the destination tab until a full page reload.
  //
  // Skipped on the first render: cache-and-network is already fetching every
  // query then, and refetching would issue a second identical request.
  //
  // Bulk archive / restore is asynchronous server-side, so a refetch fired
  // immediately after a large batch can still land mid-flight; the next visit
  // to the tab reconciles. Single-row changes are effectively complete by the
  // time the user switches.
  const tabRevalidateMounted = useRef(false)
  useEffect(() => {
    if (!currentWorkspace) return
    if (!tabRevalidateMounted.current) {
      tabRevalidateMounted.current = true
      return
    }
    switch (statusFilter) {
      case 'OPEN':
        void refetchOpen()
        break
      case 'CLOSED':
        void refetchClosed()
        break
      case 'ARCHIVED':
        void refetchArchived()
        break
      case 'DRAFT':
        void refetchDrafts()
        break
      case 'ALL':
        void refetchOpen()
        void refetchClosed()
        break
    }
    // Deliberately keyed on the tab and the workspace only: including the
    // refetch callbacks would re-run this on every Apollo state change.
  }, [statusFilter, wsKey])

  // Rows can disappear between renders (another tab's mutation, a draft TTL
  // expiry, someone else archiving). Drop selections for ids that no longer
  // exist so the action count stays honest.
  //
  // Return null while the active tab's query has not produced data yet — a
  // refetch / network blip can briefly null out the payload, and without this
  // guard we would read "no data" as "every row is gone" and wipe the user's
  // selection.
  const visibleIdSet = useMemo(() => {
    let rows: CaseRow[] | undefined
    if (statusFilter === 'DRAFT') rows = draftData?.drafts
    else if (statusFilter === 'CLOSED') rows = closedData?.cases
    else if (statusFilter === 'ARCHIVED') rows = archivedData?.cases
    if (!rows) return null
    const ids = new Set<number>()
    for (const r of rows) ids.add(r.id)
    return ids
  }, [statusFilter, draftData, closedData, archivedData])
  useEffect(() => {
    if (!visibleIdSet) return
    setSelectedIds((prev) => {
      let changed = false
      const next = new Set<number>()
      for (const id of prev) {
        if (visibleIdSet.has(id)) next.add(id)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [visibleIdSet])

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
          if (caseStatuses.isThreadMode && c.boardStatus) {
            return (
              <BoardStatusBadge
                label={caseStatuses.label(c.boardStatus)}
                color={caseStatuses.get(c.boardStatus)?.color}
              />
            )
          }
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
        case 'updated':
          return <span className="mono soft" style={{ fontSize: 12 }}>{formatDate(c.updatedAt)}</span>
        case 'slack': {
          const href = slackCaseLink(c)
          return href
            ? <SlackLink name="" href={href} />
            : <span className="soft">—</span>
        }
      }
    } else {
      const fieldDef = col.def!
      const v = c.fields.find((cf) => cf.fieldId === fieldDef.id)?.value
      return renderFieldValue(v, fieldDef)
    }
    return null
  }

  const visibleColumns = allColumns.filter((c) => isVisible(c.key))

  // Whether this cell already renders its own <a> (the Slack deep link, a URL
  // field value). Such a cell must not get a row link on top: nesting anchors
  // is invalid HTML and an overlay would swallow the inner link's clicks.
  //
  // Decided per row, not per column: both cells fall back to a dash when the
  // value is missing, and a dash is no link. Judging by column type alone left
  // every Slack-less Case with a dead cell in the always-visible Slack column.
  const hasOwnLink = (col: typeof allColumns[number], c: CaseRow) => {
    if (!col.custom) return col.key === 'slack' && Boolean(slackCaseLink(c))
    const def = col.def!
    if (def.type !== 'URL') return false
    return !isBlankFieldValue(c.fields.find((cf) => cf.fieldId === def.id)?.value)
  }

  // Carried to the case detail page so its Back / delete / discard handlers
  // return to the tab AND page the row was opened from.
  const rowLinkState = {
    fromStatus: statusToQuery(statusFilter),
    fromPage: page > 0 ? page + 1 : undefined,
  }

  // Rows the user is allowed to select. accessDenied rows have an opaque
  // title and we cannot act on them server-side either, so they are excluded
  // from select-all and from the per-row checkbox.
  const selectableRows = useMemo(() => {
    if (!selectionEnabled) return [] as CaseRow[]
    return filtered.filter((c) => !c.accessDenied)
  }, [selectionEnabled, filtered])

  // Three-state checkbox state for the header: all / some / none of the
  // selectable rows (across pages) are selected.
  const allSelectableIds = useMemo(
    () => selectableRows.map((c) => c.id),
    [selectableRows],
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

  const selectedRows = useMemo(
    () =>
      selectableRows
        .filter((c) => selectedIds.has(c.id))
        .map((c) => ({ id: c.id, title: c.title })),
    [selectableRows, selectedIds],
  )

  const selectedDrafts = useMemo(
    () => (isDraftsTab ? selectedRows : ([] as { id: number; title: string }[])),
    [isDraftsTab, selectedRows],
  )

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

  // Bulk archive / restore go through one mutation that returns the ACCEPTED
  // ids and does the work in the background. No refetch follows: it could not
  // reflect completion, and it would put the rows back on screen. The accepted
  // ids are removed locally instead, and the next natural load reconciles.
  //
  // No confirmation dialog: archiving is reversible from the Archived tab, and
  // the rows were picked explicitly with checkboxes. The delete confirmation
  // beside it exists because deletion is not reversible.
  const runBulkArchive = useCallback(
    async (kind: 'archive' | 'unarchive') => {
      if (!currentWorkspace || selectedRows.length === 0) return
      const ids = selectedRows.map((r) => r.id)
      const mutate = kind === 'archive' ? bulkArchiveCases : bulkUnarchiveCases
      const field = kind === 'archive' ? 'bulkArchiveCases' : 'bulkUnarchiveCases'
      try {
        const res = await mutate({
          variables: { workspaceId: currentWorkspace.id, ids },
        })
        const accepted: number[] = res.data?.[field] ?? []
        setPendingIds((prev) => {
          const next = new Set(prev)
          for (const id of accepted) next.add(id)
          return next
        })
        setSelectedIds((prev) => {
          const next = new Set(prev)
          for (const id of accepted) next.delete(id)
          return next
        })
      } catch (e) {
        // The rows stay on screen and stay selected, so the user can retry.
        console.error('Failed to bulk change case archive state', e)
      }
    },
    [currentWorkspace, selectedRows, bulkArchiveCases, bulkUnarchiveCases],
  )

  const handleBulkArchive = useCallback(() => {
    void runBulkArchive('archive')
  }, [runBulkArchive])

  const handleBulkUnarchive = useCallback(() => {
    void runBulkArchive('unarchive')
  }, [runBulkArchive])

  // The bar's verbs are per-tab; everything else about it (count label,
  // progress label, disabled handling) is shared.
  const bulkActions: BulkAction[] = useMemo(() => {
    if (isDraftsTab) {
      return [
        {
          key: 'submit',
          label: t('bulkSelectionBarSubmit'),
          variant: 'primary',
          testId: 'bulk-submit-button',
          onClick: handleBulkSubmit,
        },
        {
          key: 'delete',
          label: t('bulkSelectionBarDelete'),
          variant: 'danger',
          testId: 'bulk-delete-button',
          onClick: handleBulkDeleteRequest,
        },
      ]
    }
    if (isClosedTab) {
      return [
        {
          key: 'archive',
          label: t('bulkSelectionBarArchive'),
          variant: 'primary',
          testId: 'bulk-archive-button',
          onClick: handleBulkArchive,
        },
      ]
    }
    if (isArchivedTab) {
      return [
        {
          key: 'unarchive',
          label: t('bulkSelectionBarUnarchive'),
          variant: 'primary',
          testId: 'bulk-unarchive-button',
          onClick: handleBulkUnarchive,
        },
      ]
    }
    return []
  }, [
    isDraftsTab,
    isClosedTab,
    isArchivedTab,
    t,
    handleBulkSubmit,
    handleBulkDeleteRequest,
    handleBulkArchive,
    handleBulkUnarchive,
  ])

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
            onClick={() => setStatusFilter('OPEN')}
            data-testid="status-tab-open"
          >
            {t('tabOpen')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{openCount}</span>
          </button>
          <button
            className={statusFilter === 'CLOSED' ? 'on' : ''}
            onClick={() => setStatusFilter('CLOSED')}
            data-testid="status-tab-closed"
          >
            {t('tabClosed')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{closedCount}</span>
          </button>
          <button
            className={statusFilter === 'DRAFT' ? 'on' : ''}
            onClick={() => setStatusFilter('DRAFT')}
            data-testid="status-tab-draft"
          >
            {t('tabDrafts')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{draftCount}</span>
          </button>
          <button
            className={statusFilter === 'ARCHIVED' ? 'on' : ''}
            onClick={() => setStatusFilter('ARCHIVED')}
            data-testid="status-tab-archived"
          >
            {t('tabArchived')}
            <span style={{ marginLeft: 6, opacity: 0.7 }}>{archivedCount}</span>
          </button>
          <button
            className={statusFilter === 'ALL' ? 'on' : ''}
            onClick={() => setStatusFilter('ALL')}
            data-testid="status-tab-all"
          >
            {t('tabAll')}
          </button>
        </div>
        {selectionEnabled && (
          <BulkSelectionBar
            selectedCount={selectedRows.length}
            actions={bulkActions}
            onClear={clearSelection}
            disabled={bulkBusy}
            progressLabel={
              bulkState.loading
                ? t('bulkProgress', { done: bulkState.done, total: bulkState.total })
                : undefined
            }
          />
        )}
        <span className="spacer" />
        {statusOptions.length > 0 && (
          <MultiSelectFilter
            label={t('filterStatus')}
            options={statusOptions}
            selected={boardStatusFilter}
            onChange={(next) => { setBoardStatusFilter(next); setPageIndex(0) }}
            testId="status-filter"
          />
        )}
        <MultiSelectFilter
          label={t('filterAssignee')}
          options={assigneeOptions}
          selected={assigneeFilter}
          onChange={(next) => { setAssigneeFilter(next); setPageIndex(0) }}
          testId="assignee-filter"
        />
        <div className="h-search" style={{ width: 160, marginLeft: 0 }}>
          <input
            type="date"
            value={updatedOn}
            onChange={(e) => { setUpdatedOn(e.target.value); setPageIndex(0) }}
            aria-label={t('filterUpdatedOn')}
            title={t('filterUpdatedOn')}
            data-testid="updated-on-filter"
            style={{
              flex: 1, border: 'none', background: 'transparent', outline: 'none',
              fontFamily: 'inherit', fontSize: 12.5,
              color: updatedOn ? 'var(--fg)' : 'var(--fg-soft)',
            }}
          />
        </div>
        <div className="h-search" style={{ width: 260, marginLeft: 0 }}>
          <IconSearch size={13} />
          <input
            value={searchText}
            onChange={(e) => { setSearchText(e.target.value); setPageIndex(0) }}
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
              {selectionEnabled && (
                <th style={{ width: 36 }}>
                  <input
                    ref={headerCheckboxRef}
                    type="checkbox"
                    data-testid="bulk-header-checkbox"
                    aria-label={t('bulkSelectAllAria')}
                    checked={allSelected}
                    onChange={toggleAll}
                    disabled={allSelectableIds.length === 0 || bulkBusy}
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
                  colSpan={3 + visibleColumns.length + (selectionEnabled ? 1 : 0)}
                  style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}
                >
                  {t('noDataAvailable')}
                </td>
              </tr>
            )}
            {pageRows.map((c) => {
              const rowSelected = selectionEnabled && selectedIds.has(c.id)
              // Drafts share the regular case detail page — Submit / Discard
              // surface there based on status.
              const caseHref = `/ws/${currentWorkspace!.id}/cases/${c.id}`
              // accessDenied rows expose no detail page, so they stay inert.
              const linkable = !c.accessDenied
              // Real anchors instead of a row onClick handler: only an <a> gives
              // the browser's own behaviour — Cmd/Ctrl-click and middle-click
              // open a new tab, "Open in new tab" appears in the context menu,
              // and the target URL shows in the status bar.
              const cellOverlay = linkable ? (
                <Link
                  className={styles.cellOverlay}
                  to={caseHref}
                  state={rowLinkState}
                  tabIndex={-1}
                  aria-hidden="true"
                />
              ) : null
              const withRowLink = (content: React.ReactNode) => (
                <div className={styles.cellInner}>
                  {cellOverlay}
                  {content}
                </div>
              )
              const titleContent = (
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
                    /* title= so the truncated tail stays readable on hover: the
                       cap is what forces the ellipsis, and no width the table
                       can afford fits every case title. */
                    <span className="title truncate" title={c.title} style={{ maxWidth: 420 }}>{c.title}</span>
                  )}
                  {!c.accessDenied && c.isTest && (
                    <span data-testid="test-badge"><TestBadge label={t('badgeTest')} /></span>
                  )}
                </div>
              )
              return (
                <tr
                  key={c.id}
                  style={{
                    background: rowSelected ? 'var(--bg-highlight)' : undefined,
                  }}
                >
                  {selectionEnabled && (
                    <td style={{ width: 36 }}>
                      <input
                        type="checkbox"
                        data-testid={`bulk-row-checkbox-${c.id}`}
                        aria-label={t('bulkSelectRowAria', { id: c.id })}
                        checked={selectedIds.has(c.id)}
                        onChange={() => toggleRow(c.id)}
                        disabled={c.accessDenied || bulkBusy}
                      />
                    </td>
                  )}
                  <td className="id mono">{withRowLink(<>#{c.id}</>)}</td>
                  <td>
                    {linkable ? (
                      // The title cell holds the row's one focusable link: it
                      // wraps the title text, so it needs no aria-label and the
                      // title stays selectable.
                      <Link
                        className={styles.titleLink}
                        to={caseHref}
                        state={rowLinkState}
                        data-testid={`case-row-link-${c.id}`}
                      >
                        {titleContent}
                      </Link>
                    ) : titleContent}
                  </td>
                  {visibleColumns.map((col) => (
                    <td key={col.key}>
                      {hasOwnLink(col, c) ? renderCell(col, c) : withRowLink(renderCell(col, c))}
                    </td>
                  ))}
                  <td>
                    <button className="h-icon-btn" style={{ width: 24, height: 24 }}>
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
          <div className="row" style={{ gap: 16 }}>
            <span>
              {filtered.length === 0
                ? '0–0 / 0'
                : `${page * pageSize + 1}–${Math.min((page + 1) * pageSize, filtered.length)} / ${filtered.length}`}
            </span>
            <label className={styles.pageSizeLabel}>
              {t('paginationPageSize')}
              <select
                className={styles.pageSizeSelect}
                data-testid="page-size-select"
                value={pageSize}
                onChange={(e) => handlePageSizeChange(Number(e.target.value))}
              >
                {PAGE_SIZE_OPTIONS.map((n) => (
                  <option key={n} value={n}>{n}</option>
                ))}
              </select>
            </label>
          </div>
          <div className="row" style={{ gap: 6 }}>
            <Button
              size="sm"
              icon={<IconChevLeft size={12} />}
              disabled={page === 0}
              onClick={() => setPageIndex(Math.max(0, page - 1))}
              data-testid="pagination-prev"
            >
              {t('btnPrevious')}
            </Button>
            <span className="mono" data-testid="pagination-info">{page + 1} / {totalPages}</span>
            <Button
              size="sm"
              icon={<IconChevRight size={12} />}
              disabled={page >= totalPages - 1}
              onClick={() => setPageIndex(Math.min(totalPages - 1, page + 1))}
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
