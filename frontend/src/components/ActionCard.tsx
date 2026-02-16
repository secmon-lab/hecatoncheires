import { useDraggable } from '@dnd-kit/core'
import styles from './ActionCard.module.css'

interface ActionCardProps {
  id: number
  title: string
  caseName: string
  assignees: Array<{ id: string; realName: string; imageUrl?: string }>
  onClick: () => void
  isOverlay?: boolean
}

export default function ActionCard({ id, title, caseName, assignees, onClick, isOverlay }: ActionCardProps) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id })

  const className = [
    styles.card,
    isDragging ? styles.dragging : '',
    isOverlay ? styles.dragOverlay : '',
  ].filter(Boolean).join(' ')

  return (
    <div
      ref={setNodeRef}
      className={className}
      onClick={onClick}
      data-testid="action-card"
      {...listeners}
      {...attributes}
    >
      <div className={styles.caseName}>{caseName}</div>
      <div className={styles.title}>{title}</div>
      {assignees.length > 0 && (
        <div className={styles.assignees}>
          {assignees.map((user) => (
            <div key={user.id} className={styles.avatar} title={user.realName}>
              {user.imageUrl ? (
                <img src={user.imageUrl} alt={user.realName} />
              ) : (
                <span>{user.realName.charAt(0)}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
