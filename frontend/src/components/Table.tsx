import { ReactNode } from 'react'
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
          {data.length === 0 ? (
            <tr>
              <td colSpan={columns.length} className={styles.emptyCell}>
                No data available
              </td>
            </tr>
          ) : (
            data.map((row) => (
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
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
