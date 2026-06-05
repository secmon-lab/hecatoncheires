import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import { GET_CASES } from '../graphql/case'
import { UPDATE_CASE_STATUS } from '../graphql/caseStatus'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { useCaseStatuses } from '../hooks/useCaseStatuses'
import { actionStatusColorStyle, actionStatusSlug } from '../utils/actionStatusStyle'
import Button from '../components/Button'
import { IconSearch } from '../components/Icons'
import { activateOnEnterOrSpace } from '../utils/keyboard'
import styles from './ActionList.module.css'

interface CaseRow {
  id: number
  title: string
  boardStatus: string | null
}

// CaseKanban renders the thread-mode Kanban: one column per configurable Case
// status, cards are Cases, drag-and-drop moves a Case between statuses
// (closing it when dropped into a closed column, handled server-side).
export default function CaseKanban() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const [search, setSearch] = useState('')
  const [draggingId, setDraggingId] = useState<number | null>(null)
  const [dragOverCol, setDragOverCol] = useState<string | null>(null)

  const { statuses, isClosed, label } = useCaseStatuses(currentWorkspace?.id)

  const { data: casesData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const [updateCaseStatus] = useMutation(UPDATE_CASE_STATUS, {
    refetchQueries: [{ query: GET_CASES, variables: { workspaceId: currentWorkspace?.id } }],
  })

  const cases: CaseRow[] = useMemo(
    () =>
      (casesData?.cases ?? []).map((c: { id: number; title: string; boardStatus: string | null }) => ({
        id: c.id,
        title: c.title,
        boardStatus: c.boardStatus,
      })),
    [casesData],
  )

  const filtered = useMemo(() => {
    if (!search.trim()) return cases
    const q = search.toLowerCase()
    return cases.filter((c) => c.title.toLowerCase().includes(q))
  }, [cases, search])

  const grouped = useMemo(() => {
    const map: Record<string, CaseRow[]> = {}
    statuses.forEach((s) => { map[s.id] = [] })
    filtered.forEach((c) => {
      const key = c.boardStatus ?? ''
      if (!map[key]) map[key] = []
      map[key].push(c)
    })
    return map
  }, [filtered, statuses])

  const handleDrop = async (target: string) => {
    const id = draggingId
    setDraggingId(null)
    setDragOverCol(null)
    if (id == null) return
    const c = cases.find((x) => x.id === id)
    if (!c || c.boardStatus === target) return
    try {
      await updateCaseStatus({
        variables: { workspaceId: currentWorkspace!.id, input: { id, status: target } },
        optimisticResponse: {
          updateCaseStatus: {
            id,
            title: c.title,
            status: isClosed(target) ? 'CLOSED' : 'OPEN',
            boardStatus: target,
            isThreadBound: true,
            __typename: 'Case',
          },
        },
      })
    } catch (e) {
      console.error('Failed to move case', e)
    }
  }

  const openCount = cases.filter((c) => !isClosed(c.boardStatus ?? '')).length

  return (
    <div className="h-main-inner" style={{ display: 'flex', flexDirection: 'column' }}>
      <div className="h-page-h">
        <div>
          <h1>{t('titleCaseBoard', { workspaceName: currentWorkspace?.name || '' })}</h1>
          <div className="sub">{t('subtitleCaseBoard')} · {openCount} open</div>
        </div>
      </div>

      <div className="row" style={{ marginBottom: 12, gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <div className="h-search" style={{ width: 280, marginLeft: 0 }}>
          <IconSearch size={13} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('placeholderSearchCaseBoard')}
            data-testid="case-board-search-input"
            style={{
              flex: 1, border: 'none', background: 'transparent', outline: 'none',
              fontFamily: 'inherit', color: 'var(--fg)', fontSize: 12.5,
            }}
          />
        </div>
        {search && (
          <Button size="sm" variant="ghost" onClick={() => setSearch('')} data-testid="case-board-filter-clear">
            {t('btnClear')}
          </Button>
        )}
      </div>

      <div data-testid="case-kanban-board" className={`kanban ${styles.kanbanWrap}`}>
        {statuses.map((col) => (
          <div
            key={col.id}
            className="kan-col"
            data-testid={`case-kanban-column-${actionStatusSlug(label(col.id))}`}
            onDragOver={(e) => { e.preventDefault(); if (dragOverCol !== col.id) setDragOverCol(col.id) }}
            onDragLeave={() => { if (dragOverCol === col.id) setDragOverCol(null) }}
            onDrop={(e) => { e.preventDefault(); void handleDrop(col.id) }}
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
              {(grouped[col.id] ?? []).map((c) => {
                const openCase = () => navigate(`/ws/${currentWorkspace!.id}/cases/${c.id}`)
                return (
                  <div
                    key={c.id}
                    role="button"
                    tabIndex={0}
                    className="kan-card"
                    data-testid="case-card"
                    draggable
                    onDragStart={(e) => {
                      setDraggingId(c.id)
                      e.dataTransfer.effectAllowed = 'move'
                      e.dataTransfer.setData('text/plain', String(c.id))
                    }}
                    onDragEnd={() => { setDraggingId(null); setDragOverCol(null) }}
                    onClick={openCase}
                    onKeyDown={activateOnEnterOrSpace(openCase)}
                    style={{ textAlign: 'left', opacity: draggingId === c.id ? 0.4 : 1, cursor: draggingId === c.id ? 'grabbing' : 'grab' }}
                  >
                    <span className={`title ${styles.titleText}`}>#{c.id} {c.title}</span>
                  </div>
                )
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
