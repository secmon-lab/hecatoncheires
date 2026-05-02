import { useState, useMemo, useEffect, useRef } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import {
  IconPlus,
  IconSearch,
  IconLock,
  IconChevLeft,
  IconChevRight,
  IconDots,
  IconSettings,
} from '../components/Icons'
import { Avatar, AvatarStack, StatusBadge, SlackLink } from '../components/Primitives'
import CaseForm from './CaseForm'

const PAGE_SIZE = 20

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
  status: 'OPEN' | 'CLOSED'
  isPrivate: boolean
  accessDenied: boolean
  reporter?: CaseUser | null
  assignees: CaseUser[]
  slackChannelID: string
  slackChannelName?: string | null
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

const DEFAULT_VISIBLE = ['status', 'assignees', 'created', 'slack']

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

  const [statusFilter, setStatusFilter] = useState<'OPEN' | 'CLOSED' | 'ALL'>('OPEN')
  const [searchText, setSearchText] = useState('')
  const [page, setPage] = useState(0)
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [columnsOpen, setColumnsOpen] = useState(false)
  const columnsBtnRef = useRef<HTMLDivElement>(null)

  const wsKey = currentWorkspace?.id || 'default'
  const storageKey = `caseListColumns:${wsKey}`

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
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const openCount = openData?.cases?.length ?? 0
  const closedCount = closedData?.cases?.length ?? 0

  const cases: CaseRow[] = useMemo(() => {
    if (statusFilter === 'OPEN') return openData?.cases || []
    if (statusFilter === 'CLOSED') return closedData?.cases || []
    return [...(openData?.cases || []), ...(closedData?.cases || [])]
  }, [statusFilter, openData, closedData])

  const filtered = useMemo(() => {
    if (!searchText.trim()) return cases
    const q = searchText.toLowerCase()
    return cases.filter((c) => !c.accessDenied && c.title.toLowerCase().includes(q))
  }, [cases, searchText])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageRows = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)

  const fieldDefs: FieldDef[] = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || t('navCases')

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
          return <StatusBadge status={c.status} labelOpen={t('statusOpen')} labelClosed={t('statusClosed')} />
        case 'assignees':
          return c.assignees && c.assignees.length > 0 ? <AvatarStack users={c.assignees} /> : <span className="soft">—</span>
        case 'reporter':
          return c.reporter ? (
            <div className="row" style={{ gap: 6, fontSize: 12 }}>
              <Avatar size="sm" name={c.reporter.name} realName={c.reporter.realName} imageUrl={c.reporter.imageUrl} />
              <span className="truncate" style={{ maxWidth: 100 }}>{c.reporter.realName}</span>
            </div>
          ) : <span className="soft">—</span>
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
          <Button variant="primary" icon={<IconPlus size={14} />} onClick={() => setIsFormOpen(true)}>
            {t('btnNewCase', { caseLabel })}
          </Button>
        </div>
      </div>

      <div className="row" style={{ marginBottom: 12, gap: 12, flexWrap: 'wrap' }}>
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
            className={statusFilter === 'ALL' ? 'on' : ''}
            onClick={() => { setStatusFilter('ALL'); setPage(0) }}
          >
            {t('tabAll')}
          </button>
        </div>
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
                <td colSpan={3 + visibleColumns.length} style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}>
                  {t('noDataAvailable')}
                </td>
              </tr>
            )}
            {pageRows.map((c) => (
              <tr
                key={c.id}
                onClick={() => {
                  if (c.accessDenied) return
                  navigate(`/ws/${currentWorkspace!.id}/cases/${c.id}`)
                }}
                style={{ cursor: c.accessDenied ? 'default' : 'pointer' }}
              >
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
            ))}
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
    </div>
  )
}
