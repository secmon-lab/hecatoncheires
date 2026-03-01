import { useState, useEffect, useMemo, useRef } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus, Settings, ChevronLeft, ChevronRight, Search, Lock } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import CaseForm from './CaseForm'
import { GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './CaseList.module.css'
import type { ReactElement } from 'react'

type CaseStatus = 'OPEN' | 'CLOSED'

const PAGE_SIZE = 20

const STORAGE_KEY = 'hecatoncheires-caselist-columns'

interface Case {
  id: number
  title: string
  description: string
  status: CaseStatus
  isPrivate: boolean
  accessDenied: boolean
  assigneeIDs: string[]
  assignees: Array<{ id: string; realName: string; imageUrl?: string }>
  slackChannelID: string
  slackChannelName: string
  createdAt: string
  updatedAt: string
  fields: Array<{ fieldId: string; value: any }>
}

const DEFAULT_HIDDEN_COLUMNS: Record<string, boolean> = {
  description: false,
}

function loadColumnVisibility(): Record<string, boolean> {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) return { ...DEFAULT_HIDDEN_COLUMNS, ...JSON.parse(stored) }
  } catch { /* ignore */ }
  return { ...DEFAULT_HIDDEN_COLUMNS }
}

function saveColumnVisibility(visibility: Record<string, boolean>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(visibility))
}

export default function CaseList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [statusFilter, setStatusFilter] = useState<CaseStatus>('OPEN')
  const [searchText, setSearchText] = useState('')
  const [currentPage, setCurrentPage] = useState(0)
  const [columnVisibility, setColumnVisibility] = useState<Record<string, boolean>>(loadColumnVisibility)
  const [isColumnSelectorOpen, setIsColumnSelectorOpen] = useState(false)
  const columnSelectorRef = useRef<HTMLDivElement>(null)

  const { data, loading, error } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace!.id, status: statusFilter },
    skip: !currentWorkspace,
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })

  // Reset page when filter changes
  useEffect(() => {
    setCurrentPage(0)
  }, [statusFilter, searchText])

  // Close column selector on outside click
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (columnSelectorRef.current && !columnSelectorRef.current.contains(event.target as Node)) {
        setIsColumnSelectorOpen(false)
      }
    }

    if (isColumnSelectorOpen) {
      document.addEventListener('mousedown', handleClickOutside)
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isColumnSelectorOpen])

  const handleFormClose = () => {
    setIsFormOpen(false)
  }

  const handleRowClick = (caseItem: Case) => {
    if (caseItem.accessDenied) return
    navigate(`/ws/${currentWorkspace!.id}/cases/${caseItem.id}`)
  }

  const renderAssignees = (assignees: Array<{ id: string; realName: string; imageUrl?: string }>) => {
    if (!assignees || assignees.length === 0) return null

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        {assignees.map((user) => (
          <div key={user.id} style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            {user.imageUrl && (
              <img
                src={user.imageUrl}
                alt={user.realName}
                style={{ width: '24px', height: '24px', borderRadius: '4px' }}
              />
            )}
            <span>{user.realName}</span>
          </div>
        ))}
      </div>
    )
  }

  const renderFieldValue = (caseItem: Case, fieldId: string) => {
    const fieldValue = caseItem.fields.find((f) => f.fieldId === fieldId)
    if (!fieldValue) return '-'

    const fieldDef = configData?.fieldConfiguration?.fields?.find((f: any) => f.id === fieldId)
    if (!fieldDef) return String(fieldValue.value)

    switch (fieldDef.type) {
      case 'TEXT':
      case 'NUMBER':
      case 'DATE':
      case 'URL':
        return String(fieldValue.value || '-')

      case 'SELECT': {
        const option = fieldDef.options?.find((opt: any) => opt.id === fieldValue.value)
        return option ? option.name : fieldValue.value
      }

      case 'MULTI_SELECT': {
        const selectedOptions = (fieldValue.value || [])
          .map((id: string) => fieldDef.options?.find((opt: any) => opt.id === id)?.name)
          .filter(Boolean)
        return selectedOptions.length > 0 ? selectedOptions.join(', ') : '-'
      }

      case 'USER':
      case 'MULTI_USER':
        return '-'

      default:
        return String(fieldValue.value || '-')
    }
  }

  const isColumnVisible = (columnKey: string) => {
    return columnVisibility[columnKey] !== false
  }

  const toggleColumn = (columnKey: string) => {
    const newVisibility = {
      ...columnVisibility,
      [columnKey]: !isColumnVisible(columnKey),
    }
    setColumnVisibility(newVisibility)
    saveColumnVisibility(newVisibility)
  }

  const allColumns = useMemo(() => {
    const cols: Array<{ key: string; header: string; accessor: any; width: string }> = [
      {
        key: 'id',
        header: 'ID',
        accessor: 'id' as keyof Case,
        width: '48px',
      },
      {
        key: 'title',
        header: 'Title',
        accessor: ((caseItem: Case) => (
          <div className={styles.privateTitleCell}>
            {caseItem.isPrivate && <Lock size={14} className={styles.privateTitleLock} data-testid="private-lock-icon" />}
            <span
              className={caseItem.accessDenied ? styles.accessDenied : ''}
              data-testid={caseItem.accessDenied ? 'access-denied-label' : undefined}
            >
              {caseItem.accessDenied ? 'Private' : caseItem.title}
            </span>
          </div>
        )) as (row: Case) => ReactElement,
        width: '200px',
      },
      {
        key: 'description',
        header: 'Description',
        accessor: 'description' as keyof Case,
        width: '250px',
      },
      {
        key: 'assignees',
        header: 'Assignees',
        accessor: ((caseItem: Case) => renderAssignees(caseItem.assignees)) as (row: Case) => ReactElement | null,
        width: '200px',
      },
      {
        key: 'created',
        header: 'Created',
        accessor: ((caseItem: Case) => new Date(caseItem.createdAt).toLocaleDateString()) as (row: Case) => string,
        width: '120px',
      },
    ]

    if (configData?.fieldConfiguration?.fields) {
      configData.fieldConfiguration.fields.forEach((field: any) => {
        cols.push({
          key: `field-${field.id}`,
          header: field.name,
          accessor: ((caseItem: Case) => renderFieldValue(caseItem, field.id)) as (row: Case) => string,
          width: '150px',
        })
      })
    }

    return cols
  }, [configData])

  const visibleColumns = allColumns
    .filter((col) => isColumnVisible(col.key))
    .map(({ header, accessor, width }) => ({ header, accessor, width }))

  // Helper to extract text value from a column for filtering
  const getColumnTextValue = (caseItem: Case, col: { key: string; accessor: any }): string => {
    if (typeof col.accessor === 'string') {
      const val = caseItem[col.accessor as keyof Case]
      return val != null ? String(val) : ''
    }
    // For function accessors, extract text from rendered value
    const rendered = col.accessor(caseItem)
    if (typeof rendered === 'string') return rendered
    return ''
  }

  // Filter cases by search text across all visible columns
  const allCases: Case[] = data?.cases || []
  const filteredCases = useMemo(() => {
    if (!searchText.trim()) return allCases
    const query = searchText.toLowerCase()
    const visibleCols = allColumns.filter((col) => isColumnVisible(col.key))
    return allCases.filter((c) =>
      visibleCols.some((col) => getColumnTextValue(c, col).toLowerCase().includes(query))
    )
  }, [allCases, searchText, allColumns, columnVisibility])

  // Pagination
  const totalPages = Math.max(1, Math.ceil(filteredCases.length / PAGE_SIZE))
  const paginatedCases = filteredCases.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE)

  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>Error: {error.message}</div>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div>
          <h2 className={styles.title}>{caseLabel} Management</h2>
          <p className={styles.subtitle}>Manage and track {caseLabel.toLowerCase()}s</p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={20} />}
          onClick={() => setIsFormOpen(true)}
        >
          New {caseLabel}
        </Button>
      </div>

      <div className={styles.toolbar}>
        <div className={styles.tabs}>
          <button
            className={`${styles.tab} ${statusFilter === 'OPEN' ? styles.tabActive : ''}`}
            onClick={() => setStatusFilter('OPEN')}
            data-testid="status-tab-open"
          >
            Open
          </button>
          <button
            className={`${styles.tab} ${statusFilter === 'CLOSED' ? styles.tabActive : ''}`}
            onClick={() => setStatusFilter('CLOSED')}
            data-testid="status-tab-closed"
          >
            Closed
          </button>
        </div>

        <div className={styles.toolbarRight}>
          <div className={styles.searchWrapper}>
            <Search size={16} className={styles.searchIcon} />
            <input
              type="text"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder="Search..."
              className={styles.searchInput}
              data-testid="search-filter"
            />
          </div>

          <div className={styles.columnSelector} ref={columnSelectorRef}>
            <button
              className={styles.columnSelectorButton}
              onClick={() => setIsColumnSelectorOpen(!isColumnSelectorOpen)}
              data-testid="column-selector-button"
              title="Toggle columns"
            >
              <Settings size={16} />
            </button>
            {isColumnSelectorOpen && (
              <div className={styles.columnSelectorPopover} data-testid="column-selector-popover">
                <div className={styles.columnSelectorTitle}>Columns</div>
                {allColumns.map((col) => (
                  <label key={col.key} className={styles.columnSelectorItem}>
                    <input
                      type="checkbox"
                      checked={isColumnVisible(col.key)}
                      onChange={() => toggleColumn(col.key)}
                      data-testid={`column-toggle-${col.key}`}
                    />
                    <span>{col.header}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.tableWrapper}>
        <Table columns={visibleColumns} data={paginatedCases} onRowClick={handleRowClick} resizable />
      </div>

      {totalPages > 1 && (
        <div className={styles.pagination} data-testid="pagination">
          <button
            className={styles.paginationButton}
            onClick={() => setCurrentPage((p) => Math.max(0, p - 1))}
            disabled={currentPage === 0}
            data-testid="pagination-prev"
          >
            <ChevronLeft size={16} />
          </button>
          <span className={styles.paginationInfo} data-testid="pagination-info">
            {currentPage + 1} / {totalPages}
          </span>
          <button
            className={styles.paginationButton}
            onClick={() => setCurrentPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={currentPage >= totalPages - 1}
            data-testid="pagination-next"
          >
            <ChevronRight size={16} />
          </button>
        </div>
      )}

      <CaseForm
        isOpen={isFormOpen}
        onClose={handleFormClose}
        caseItem={null}
      />
    </div>
  )
}
