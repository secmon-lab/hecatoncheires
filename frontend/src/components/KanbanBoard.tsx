import { useState } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragStartEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import KanbanColumn from './KanbanColumn'
import ActionCard from './ActionCard'
import styles from './KanbanBoard.module.css'

interface Action {
  id: number
  caseID: number
  case?: { id: number; title: string }
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackMessageTS: string
  status: string
  createdAt: string
  updatedAt: string
}

interface KanbanBoardProps {
  actions: Action[]
  onCardClick: (action: Action) => void
  onStatusChange?: (actionId: number, newStatus: string) => void
}

const COLUMNS = [
  { status: 'BACKLOG', label: 'Backlog' },
  { status: 'TODO', label: 'To Do' },
  { status: 'IN_PROGRESS', label: 'In Progress' },
  { status: 'BLOCKED', label: 'Blocked' },
  { status: 'COMPLETED', label: 'Completed' },
]

export default function KanbanBoard({ actions, onCardClick, onStatusChange }: KanbanBoardProps) {
  const [activeAction, setActiveAction] = useState<Action | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    })
  )

  const handleDragStart = (event: DragStartEvent) => {
    const action = actions.find((a) => a.id === event.active.id)
    if (action) setActiveAction(action)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setActiveAction(null)

    if (!over) return

    const actionId = active.id as number
    const newStatus = over.id as string
    const action = actions.find((a) => a.id === actionId)

    if (action && action.status !== newStatus && onStatusChange) {
      onStatusChange(actionId, newStatus)
    }
  }

  const grouped = COLUMNS.map((col) => ({
    ...col,
    actions: actions.filter((a) => a.status === col.status),
  }))

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      <div className={styles.board} data-testid="kanban-board">
        {grouped.map((col) => (
          <KanbanColumn key={col.status} status={col.status} title={col.label} count={col.actions.length}>
            {col.actions.map((action) => (
              <ActionCard
                key={action.id}
                id={action.id}
                title={action.title}
                caseName={action.case?.title || `Case #${action.caseID}`}
                assignees={action.assignees}
                onClick={() => onCardClick(action)}
              />
            ))}
          </KanbanColumn>
        ))}
      </div>
      <DragOverlay>
        {activeAction ? (
          <ActionCard
            id={activeAction.id}
            title={activeAction.title}
            caseName={activeAction.case?.title || `Case #${activeAction.caseID}`}
            assignees={activeAction.assignees}
            onClick={() => {}}
            isOverlay
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  )
}
