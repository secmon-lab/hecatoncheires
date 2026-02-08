import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { BookOpen, ChevronLeft, ChevronRight, ExternalLink } from 'lucide-react'
import { GET_KNOWLEDGES } from '../graphql/knowledge'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './KnowledgeList.module.css'

interface Knowledge {
  id: string
  caseID: number
  sourceID: string
  sourceURL: string
  title: string
  summary: string
  sourcedAt: string
  createdAt: string
  updatedAt: string
  case?: {
    id: number
    title: string
  }
}

interface KnowledgeConnection {
  items: Knowledge[]
  totalCount: number
  hasMore: boolean
}

const PAGE_SIZE = 20

export default function KnowledgeList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [page, setPage] = useState(0)

  const { data, loading, error } = useQuery(GET_KNOWLEDGES, {
    variables: { workspaceId: currentWorkspace!.id, limit: PAGE_SIZE, offset: page * PAGE_SIZE },
    skip: !currentWorkspace,
  })

  const handleRowClick = (knowledge: Knowledge) => {
    navigate(`/ws/${currentWorkspace!.id}/knowledges/${knowledge.id}`)
  }

  const handleCaseClick = (e: React.MouseEvent, caseId: number) => {
    e.stopPropagation()
    navigate(`/ws/${currentWorkspace!.id}/cases/${caseId}`)
  }

  if (loading) return <div className={styles.loading}>Loading...</div>
  if (error) return <div className={styles.error}>Error: {error.message}</div>

  const connection: KnowledgeConnection = data?.knowledges || { items: [], totalCount: 0, hasMore: false }
  const totalPages = Math.ceil(connection.totalCount / PAGE_SIZE)

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerContent}>
          <BookOpen size={28} className={styles.headerIcon} />
          <div>
            <h1>Knowledge Base</h1>
            <p>AI-extracted knowledge from configured sources ({connection.totalCount} items)</p>
          </div>
        </div>
      </div>

      {connection.items.length === 0 ? (
        <div className={styles.empty}>
          <BookOpen size={48} className={styles.emptyIcon} />
          <h2>No knowledge found</h2>
          <p>Knowledge will appear here once extracted from your configured sources.</p>
        </div>
      ) : (
        <>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Title</th>
                <th>Related Case</th>
                <th>Summary</th>
                <th>Date</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {connection.items.map((knowledge) => (
                <tr
                  key={knowledge.id}
                  className={styles.row}
                  onClick={() => handleRowClick(knowledge)}
                >
                  <td className={styles.titleCell}>{knowledge.title}</td>
                  <td className={styles.riskCell}>
                    {knowledge.case ? (
                      <button
                        className={styles.riskLink}
                        onClick={(e) => handleCaseClick(e, knowledge.case!.id)}
                      >
                        {knowledge.case.title}
                      </button>
                    ) : (
                      <span className={styles.noRisk}>-</span>
                    )}
                  </td>
                  <td className={styles.summaryCell}>
                    {knowledge.summary.length > 120
                      ? knowledge.summary.substring(0, 120) + '...'
                      : knowledge.summary}
                  </td>
                  <td className={styles.dateCell}>
                    {new Date(knowledge.sourcedAt).toLocaleDateString()}
                  </td>
                  <td className={styles.linkCell}>
                    <a
                      href={knowledge.sourceURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.externalLink}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <ExternalLink size={16} />
                    </a>
                  </td>
                </tr>
              ))}
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
