import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useParams, useNavigate } from 'react-router-dom'
import Markdown from 'react-markdown'
import { ArrowLeft, Bot, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, CircleCheck, CircleMinus } from 'lucide-react'
import { GET_ASSIST_LOGS } from '../graphql/assistLog'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './AssistLogList.module.css'

interface AssistLog {
  id: string
  caseId: number
  summary: string
  actions: string
  reasoning: string
  nextSteps: string
  createdAt: string
}

interface AssistLogConnection {
  items: AssistLog[]
  totalCount: number
  hasMore: boolean
}

const PAGE_SIZES = [20, 50] as const

export default function AssistLogList() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState<number>(PAGE_SIZES[0])
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  const caseId = id ? parseInt(id, 10) : 0

  const { data, loading, error } = useQuery(GET_ASSIST_LOGS, {
    variables: {
      workspaceId: currentWorkspace!.id,
      caseId,
      limit: pageSize,
      offset: page * pageSize,
    },
    skip: !currentWorkspace || !caseId,
  })

  const handleBack = () => {
    navigate(`/ws/${currentWorkspace!.id}/cases/${caseId}`)
  }

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize)
    setPage(0)
  }

  const toggleExpand = (logId: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(logId)) {
        next.delete(logId)
      } else {
        next.add(logId)
      }
      return next
    })
  }

  if (loading) return <div className={styles.loading}>Loading...</div>
  if (error) return <div className={styles.error}>Error: {error.message}</div>

  const connection: AssistLogConnection = data?.assistLogs || { items: [], totalCount: 0, hasMore: false }
  const totalPages = Math.ceil(connection.totalCount / pageSize)

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button className={styles.backButton} onClick={handleBack}>
          <ArrowLeft size={16} />
          Back to Case
        </button>
        <div className={styles.headerContent}>
          <Bot size={28} className={styles.headerIcon} />
          <div>
            <h1>Assist Logs</h1>
            <p>AI assist execution history ({connection.totalCount} entries)</p>
          </div>
        </div>
      </div>

      {connection.items.length === 0 ? (
        <div className={styles.empty}>
          <Bot size={48} className={styles.emptyIcon} />
          <h2>No assist logs found</h2>
          <p>Assist logs will appear here once the AI assist agent has been run for this case.</p>
        </div>
      ) : (
        <>
          <div className={styles.pageSizeSelector}>
            <span>Show:</span>
            {PAGE_SIZES.map((size) => (
              <button
                key={size}
                className={`${styles.pageSizeButton} ${pageSize === size ? styles.pageSizeActive : ''}`}
                onClick={() => handlePageSizeChange(size)}
              >
                {size}
              </button>
            ))}
          </div>

          <table className={styles.logTable}>
            <thead>
              <tr>
                <th className={styles.colDate}>Date</th>
                <th className={styles.colSummary}>Summary</th>
                <th className={styles.colActions}>Actions</th>
                <th className={styles.colExpand}></th>
              </tr>
            </thead>
            <tbody>
              {connection.items.map((log) => {
                const isExpanded = expandedIds.has(log.id)
                const hasActions = !!log.actions
                return (
                  <>
                    <tr
                      key={log.id}
                      className={`${styles.logRow} ${isExpanded ? styles.logRowExpanded : ''}`}
                      onClick={() => toggleExpand(log.id)}
                    >
                      <td className={styles.colDate}>
                        {new Date(log.createdAt).toLocaleString()}
                      </td>
                      <td className={styles.colSummary}>{log.summary}</td>
                      <td className={styles.colActions}>
                        {hasActions ? (
                          <CircleCheck size={16} className={styles.iconHasActions} />
                        ) : (
                          <CircleMinus size={16} className={styles.iconNoActions} />
                        )}
                      </td>
                      <td className={styles.colExpand}>
                        {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                      </td>
                    </tr>
                    {isExpanded && (
                      <tr key={`${log.id}-detail`} className={styles.detailRow}>
                        <td colSpan={4}>
                          <div className={styles.logBody}>
                            {log.actions && (
                              <div className={styles.logSection}>
                                <h4 className={styles.logSectionTitle}>Actions</h4>
                                <div className={styles.logSectionContent}>
                                  <Markdown>{log.actions}</Markdown>
                                </div>
                              </div>
                            )}
                            <div className={styles.logSection}>
                              <h4 className={styles.logSectionTitle}>Reasoning</h4>
                              <div className={styles.logSectionContent}>
                                <Markdown>{log.reasoning}</Markdown>
                              </div>
                            </div>
                            {log.nextSteps && (
                              <div className={styles.logSection}>
                                <h4 className={styles.logSectionTitle}>Next Steps</h4>
                                <div className={styles.logSectionContent}>
                                  <Markdown>{log.nextSteps}</Markdown>
                                </div>
                              </div>
                            )}
                          </div>
                        </td>
                      </tr>
                    )}
                  </>
                )
              })}
            </tbody>
          </table>

          {totalPages > 1 && (
            <div className={styles.pagination}>
              <button
                className={styles.paginationButton}
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0}
              >
                <ChevronLeft size={16} />
                Previous
              </button>
              <span className={styles.paginationInfo}>
                Page {page + 1} of {totalPages}
              </span>
              <button
                className={styles.paginationButton}
                onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                disabled={!connection.hasMore}
              >
                Next
                <ChevronRight size={16} />
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
