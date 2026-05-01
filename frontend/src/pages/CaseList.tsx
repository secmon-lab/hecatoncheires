import { useState, useMemo } from 'react'
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
  IconFilter,
  IconSettings,
} from '../components/Icons'
import { AvatarStack, StatusBadge, SlackLink } from '../components/Primitives'
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
interface CaseRow {
  id: number
  title: string
  status: 'OPEN' | 'CLOSED'
  isPrivate: boolean
  accessDenied: boolean
  reporter?: { id: string; name: string; realName: string; imageUrl?: string } | null
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackChannelID: string
  createdAt: string
  fields: Array<{ fieldId: string; value: any }>
}

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}/${mm}/${dd}`
}

function categoryFor(c: CaseRow, fields: FieldDef[]): string {
  // Pick the first SELECT-type field as "category" if present.
  const selectField = fields.find((f) => f.type === 'SELECT')
  if (!selectField) return ''
  const v = c.fields.find((cf) => cf.fieldId === selectField.id)?.value
  if (v == null) return ''
  const opt = selectField.options?.find((o) => o.id === v || o.name === v)
  return opt?.name ?? String(v)
}

export default function CaseList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const [statusFilter, setStatusFilter] = useState<'OPEN' | 'CLOSED' | 'ALL'>('OPEN')
  const [searchText, setSearchText] = useState('')
  const [page, setPage] = useState(0)
  const [isFormOpen, setIsFormOpen] = useState(false)

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

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('titleCaseManagement', { caseLabel })}</h1>
          <div className="sub">{t('subtitleCaseManagement', { caseLabelLower: caseLabel.toLowerCase() })}</div>
        </div>
        <div className="actions">
          <Button icon={<IconFilter size={14} />}>{t('btnFilter')}</Button>
          <Button icon={<IconSettings size={14} />}>{t('btnColumns')}</Button>
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
              <th style={{ width: 110 }}>{t('headerStatus')}</th>
              <th style={{ width: 130 }}>{t('headerCategory')}</th>
              <th style={{ width: 130 }}>{t('headerAssignees')}</th>
              <th style={{ width: 110 }}>{t('headerCreated')}</th>
              <th style={{ width: 180 }}>{t('headerSlack')}</th>
              <th style={{ width: 38 }}></th>
            </tr>
          </thead>
          <tbody>
            {pageRows.length === 0 && (
              <tr>
                <td colSpan={8} style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}>
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
                <td><StatusBadge status={c.status} labelOpen={t('statusOpen')} labelClosed={t('statusClosed')} /></td>
                <td><span className="muted">{categoryFor(c, fieldDefs) || '—'}</span></td>
                <td><AvatarStack users={c.assignees} /></td>
                <td className="mono soft" style={{ fontSize: 12 }}>{formatDate(c.createdAt)}</td>
                <td>
                  {c.slackChannelID ? (
                    <SlackLink name={c.slackChannelID} />
                  ) : (
                    <span className="soft">—</span>
                  )}
                </td>
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
