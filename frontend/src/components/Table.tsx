import { ReactNode, useState, useCallback, useRef, useEffect } from 'react'
import { useIsMobile } from '../hooks/useMediaQuery'
import styles from './Table.module.css'

interface Column<T> {
  header: string
  accessor: keyof T | ((row: T) => ReactNode)
  width?: string
}

interface TableProps<T> {
  columns: Column<T>[]
  data: T[]
  onRowClick?: (row: T) => void
  resizable?: boolean
}

export default function Table<T extends { id: number | string }>({
  columns,
  data,
  onRowClick,
  resizable = false,
}: TableProps<T>) {
  const isMobile = useIsMobile()

  // Track column widths for resizing
  const [columnWidths, setColumnWidths] = useState<number[]>([])
  const tableRef = useRef<HTMLTableElement>(null)
  const resizingRef = useRef<{ index: number; startX: number; startWidth: number } | null>(null)

  // Initialize column widths from rendered table
  useEffect(() => {
    if (!resizable || !tableRef.current) return
    const ths = tableRef.current.querySelectorAll('thead th')
    const widths = Array.from(ths).map((th) => (th as HTMLElement).offsetWidth)
    setColumnWidths(widths)
  }, [resizable, columns.length])

  const handleResizeStart = useCallback(
    (index: number, e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (!tableRef.current) return
      const th = tableRef.current.querySelectorAll('thead th')[index] as HTMLElement
      if (!th) return
      resizingRef.current = { index, startX: e.clientX, startWidth: th.offsetWidth }

      const handleMouseMove = (ev: MouseEvent) => {
        if (!resizingRef.current) return
        const diff = ev.clientX - resizingRef.current.startX
        const newWidth = Math.max(40, resizingRef.current.startWidth + diff)
        setColumnWidths((prev) => {
          const next = [...prev]
          next[resizingRef.current!.index] = newWidth
          return next
        })
      }

      const handleMouseUp = () => {
        resizingRef.current = null
        document.removeEventListener('mousemove', handleMouseMove)
        document.removeEventListener('mouseup', handleMouseUp)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }

      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      document.addEventListener('mousemove', handleMouseMove)
      document.addEventListener('mouseup', handleMouseUp)
    },
    []
  )

  if (data.length === 0) {
    return (
      <div className={styles.emptyState}>
        <p>No data available</p>
      </div>
    )
  }

  if (isMobile) {
    return (
      <div className={styles.cardContainer}>
        {data.map((row) => (
          <div
            key={row.id}
            className={`${styles.card} ${onRowClick ? styles.clickable : ''}`}
            onClick={() => onRowClick?.(row)}
          >
            {columns.map((column) => {
              const value = typeof column.accessor === 'function'
                ? column.accessor(row)
                : String(row[column.accessor])

              return (
                <div key={column.header} className={styles.cardRow}>
                  <div className={styles.cardLabel}>{column.header}</div>
                  <div className={styles.cardValue}>{value}</div>
                </div>
              )
            })}
          </div>
        ))}
      </div>
    )
  }

  const useFixedLayout = resizable && columnWidths.length === columns.length

  return (
    <div className={styles.tableContainer}>
      <table
        ref={tableRef}
        className={styles.table}
        style={useFixedLayout ? { tableLayout: 'fixed' } : undefined}
      >
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th
                key={index}
                className={`${styles.th} ${resizable ? styles.thResizable : ''}`}
                style={{
                  width: useFixedLayout ? `${columnWidths[index]}px` : column.width,
                }}
              >
                <span className={styles.thContent}>{column.header}</span>
                {resizable && (
                  <div
                    className={styles.resizeHandle}
                    onMouseDown={(e) => handleResizeStart(index, e)}
                  />
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row) => (
            <tr
              key={row.id}
              className={onRowClick ? styles.clickableRow : ''}
              onClick={() => onRowClick?.(row)}
            >
              {columns.map((column, index) => (
                <td key={index} className={styles.td}>
                  <div className={styles.cellContent}>
                    {typeof column.accessor === 'function'
                      ? column.accessor(row)
                      : String(row[column.accessor])}
                  </div>
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
