import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import { GET_OPEN_CASE_ACTIONS, UPDATE_ACTION } from '../graphql/action'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { useActionStatuses } from '../hooks/useActionStatuses'
import {
  actionStatusColorStyle,
  actionStatusSlug,
} from '../utils/actionStatusStyle'
import Button from '../components/Button'
import {
  IconPlus,
  IconSearch,
} from '../components/Icons'
import { Avatar } from '../components/Primitives'
import ActionForm from './ActionForm'
import ActionModal from './ActionModal'
import styles from './ActionList.module.css'

interface ActionRow {
  id: number
  caseID: number
  case?: { id: number; title: string }
  title: string
  description: string
  assigneeID: string | null
  assignee: { id: string; name: string; realName: string; imageUrl?: string } | null
  status: string
  dueDate?: string | null
  createdAt: string
}

function formatDue(iso?: string | null) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const today = new Date()
  const diff = Math.round((d.getTime() - today.getTime()) / (1000 * 60 * 60 * 24))
  if (diff < 0) return 'Overdue'
  if (diff === 0) return 'Today'
  if (diff === 1) return 'Tomorrow'
  return d.toLocaleDateString()
}

export default function ActionList() {
  const navigate = useNavigate()
  const { actionId } = useParams<{ actionId?: string }>()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [draggingId, setDraggingId] = useState<number | null>(null)
  const [dragOverCol, setDragOverCol] = useState<string | null>(null)
  const { statuses, isClosed, label } = useActionStatuses(currentWorkspace?.id)

  const { data } = useQuery(GET_OPEN_CASE_ACTIONS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const [updateAction] = useMutation(UPDATE_ACTION, {
    refetchQueries: [{ query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace?.id } }],
  })

  const actions: ActionRow[] = data?.openCaseActions || []
  const filtered = useMemo(() => {
    if (!search.trim()) return actions
    const q = search.toLowerCase()
    return actions.filter((a) =>
      a.title.toLowerCase().includes(q) ||
      (a.description || '').toLowerCase().includes(q) ||
      (a.case?.title || '').toLowerCase().includes(q),
    )
  }, [actions, search])

  const grouped = useMemo(() => {
    const map: Record<string, ActionRow[]> = {}
    statuses.forEach((s) => { map[s.id] = [] })
    filtered.forEach((a) => {
      if (!map[a.status]) map[a.status] = []
      map[a.status].push(a)
    })
    return map
  }, [filtered, statuses])

  const detailActionId = actionId ? Number(actionId) : null

  const handleDrop = async (target: string) => {
    const id = draggingId
    setDraggingId(null)
    setDragOverCol(null)
    if (id == null) return
    const a = actions.find((x) => x.id === id)
    if (!a || a.status === target) return
    try {
      await updateAction({
        variables: { workspaceId: currentWorkspace!.id, input: { id, status: target } },
        optimisticResponse: {
          updateAction: { ...a, status: target, __typename: 'Action' },
        },
      })
    } catch (e) {
      console.error('Failed to move action', e)
    }
  }

  const openCount = actions.filter((a) => !isClosed(a.status)).length

  return (
    <div className="h-main-inner" style={{ display: 'flex', flexDirection: 'column' }}>
      <div className="h-page-h">
        <div>
          <h1>{t('titleActions', { workspaceName: currentWorkspace?.name || '' })}</h1>
          <div className="sub">{t('subtitleActions')} · {openCount} open</div>
        </div>
        <div className="actions">
          <Button variant="primary" icon={<IconPlus size={14} />} onClick={() => setShowCreate(true)}>
            {t('btnNewAction')}
          </Button>
        </div>
      </div>

      <div className="row" style={{ marginBottom: 12, gap: 12, flexWrap: 'wrap' }}>
        <div className="h-search" style={{ width: 280, marginLeft: 0 }}>
          <IconSearch size={13} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('placeholderSearchActions')}
            data-testid="action-search-input"
            style={{
              flex: 1, border: 'none', background: 'transparent', outline: 'none',
              fontFamily: 'inherit', color: 'var(--fg)', fontSize: 12.5,
            }}
          />
        </div>
        {search && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setSearch('')}
            data-testid="action-filter-clear"
          >
            {t('btnClear')}
          </Button>
        )}
      </div>

      <div
        data-testid="kanban-board"
        className={`kanban ${styles.kanbanWrap}`}
      >
        {statuses.map((col) => (
          <div
            key={col.id}
            className="kan-col"
            data-testid={`kanban-column-${actionStatusSlug(label(col.id))}`}
            onDragOver={(e) => { e.preventDefault(); if (dragOverCol !== col.id) setDragOverCol(col.id) }}
            onDragLeave={() => { if (dragOverCol === col.id) setDragOverCol(null) }}
            onDrop={(e) => { e.preventDefault(); handleDrop(col.id) }}
            style={dragOverCol === col.id ? { outline: '2px dashed var(--accent)', outlineOffset: -2 } : undefined}
          >
            <div className="kan-h">
              <span
                className="pip"
                style={{ width: 8, height: 8, borderRadius: '50%', ...actionStatusColorStyle(col.color) }}
              />
              {label(col.id)}
              <span className="count">{(grouped[col.id] ?? []).length}</span>
            </div>
            <div className="kan-list">
              {(grouped[col.id] ?? []).map((a) => (
                <button
                  key={a.id}
                  type="button"
                  className="kan-card"
                  data-testid="action-card"
                  draggable
                  onDragStart={(e) => {
                    setDraggingId(a.id)
                    e.dataTransfer.effectAllowed = 'move'
                    e.dataTransfer.setData('text/plain', String(a.id))
                  }}
                  onDragEnd={() => { setDraggingId(null); setDragOverCol(null) }}
                  onClick={() => navigate(`/ws/${currentWorkspace!.id}/actions/${a.id}`)}
                  style={{ textAlign: 'left', opacity: draggingId === a.id ? 0.4 : 1, cursor: draggingId === a.id ? 'grabbing' : 'grab' }}
                >
                  {a.case && (
                    <span className="case-link">#{a.case.id} {a.case.title}</span>
                  )}
                  <span className={`title ${styles.titleText}`}>{a.title}</span>
                  <div className="meta">
                    {a.assignee
                      ? <Avatar size="sm" name={a.assignee.name} realName={a.assignee.realName} imageUrl={a.assignee.imageUrl} />
                      : <span style={{ width: 20 }} />
                    }
                    <span className="spacer" />
                    <span className="mono" style={{ fontSize: 11 }}>{formatDue(a.dueDate)}</span>
                  </div>
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>

      {showCreate && (
        <ActionForm onClose={() => setShowCreate(false)} action={null} />
      )}

      {detailActionId && (
        <ActionModal
          actionId={detailActionId}
          onClose={() => navigate(`/ws/${currentWorkspace!.id}/actions`)}
        />
      )}
    </div>
  )
}
