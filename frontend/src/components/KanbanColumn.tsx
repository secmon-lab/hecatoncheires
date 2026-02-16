import type { ReactNode } from 'react'
import { useDroppable } from '@dnd-kit/core'
import styles from './KanbanColumn.module.css'

interface KanbanColumnProps {
  status: string
  title: string
  count: number
  children: ReactNode
}

export default function KanbanColumn({ status, title, count, children }: KanbanColumnProps) {
  const { isOver, setNodeRef } = useDroppable({ id: status })

  const className = [
    styles.column,
    isOver ? styles.dropTarget : '',
  ].filter(Boolean).join(' ')

  return (
    <div
      ref={setNodeRef}
      className={className}
      data-testid={`kanban-column-${title.toLowerCase().replace(/\s+/g, '-')}`}
    >
      <div className={styles.header}>
        <span className={styles.title}>{title}</span>
        <span className={styles.count}>{count}</span>
      </div>
      <div className={styles.body}>
        {children}
      </div>
    </div>
  )
}
