import { ReactNode } from 'react'
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
}

export default function Table<T extends { id: number | string }>({
  columns,
  data,
  onRowClick,
}: TableProps<T>) {
  const isMobile = useIsMobile()

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
            {columns.map((column, index) => {
              const value = typeof column.accessor === 'function'
                ? column.accessor(row)
                : String(row[column.accessor])

              return (
                <div key={index} className={styles.cardRow}>
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

  return (
    <div className={styles.tableContainer}>
      <table className={styles.table}>
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th
                key={index}
                className={styles.th}
                style={{ width: column.width }}
              >
                {column.header}
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
                  {typeof column.accessor === 'function'
                    ? column.accessor(row)
                    : String(row[column.accessor])}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
