import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2, Plus, ExternalLink, BookOpen, ChevronLeft, ChevronRight } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import CaseForm from './CaseForm'
import CaseDeleteDialog from './CaseDeleteDialog'
import ActionForm from './ActionForm'
import { GET_CASE } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import styles from './CaseDetail.module.css'

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
}

interface Case {
  id: number
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackChannelID: string
  slackChannelName: string
  fields: Array<{ fieldId: string; value: any }>
  actions?: Array<{
    id: number
    title: string
    status: string
    createdAt: string
  }>
  knowledges?: Knowledge[]
  createdAt: string
  updatedAt: string
}

const STATUS_LABELS: Record<string, string> = {
  BACKLOG: 'Backlog',
  TODO: 'To Do',
  IN_PROGRESS: 'In Progress',
  BLOCKED: 'Blocked',
  COMPLETED: 'Completed',
  ABANDONED: 'Abandoned',
}

const STATUS_COLORS: Record<string, number> = {
  BACKLOG: 0,
  TODO: 1,
  IN_PROGRESS: 2,
  BLOCKED: 3,
  COMPLETED: 4,
  ABANDONED: 5,
}

export default function CaseDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isActionFormOpen, setIsActionFormOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const [knowledgePage, setKnowledgePage] = useState(0)
  const knowledgePageSize = 5
  const menuRef = useRef<HTMLDivElement>(null)

  const { data: caseData, loading: caseLoading, error: caseError, refetch } = useQuery(GET_CASE, {
    variables: { id: parseInt(id || '0') },
    skip: !id,
  })

  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION)

  const caseItem: Case | undefined = caseData?.case
  const fieldDefs = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  const handleBack = () => {
    navigate('/cases')
  }

  const handleEdit = () => {
    setIsFormOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate('/cases')
  }

  const handleActionClick = (actionId: number) => {
    navigate(`/actions/${actionId}`)
  }

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsMenuOpen(false)
      }
    }

    if (isMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isMenuOpen])

  const renderFieldValue = (fieldId: string, value: any) => {
    const fieldDef = fieldDefs.find((f: any) => f.id === fieldId)
    if (!fieldDef) return String(value)

    switch (fieldDef.type) {
      case 'TEXT':
      case 'NUMBER':
      case 'DATE':
        return String(value || '-')

      case 'URL':
        return value ? (
          <a href={value} target="_blank" rel="noopener noreferrer" className={styles.link}>
            {value}
          </a>
        ) : (
          '-'
        )

      case 'SELECT':
        const option = fieldDef.options?.find((opt: any) => opt.id === value)
        return option ? option.name : value || '-'

      case 'MULTI_SELECT':
        const selectedOptions = (value || [])
          .map((id: string) => fieldDef.options?.find((opt: any) => opt.id === id)?.name)
          .filter(Boolean)
        return selectedOptions.length > 0 ? selectedOptions.join(', ') : '-'

      case 'USER':
      case 'MULTI_USER':
        return '-'

      default:
        return String(value || '-')
    }
  }

  if (caseLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (caseError || !caseItem) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {caseError ? `Error: ${caseError.message}` : `${caseLabel} not found`}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back to List
        </Button>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back
        </Button>
        <div className={styles.actions}>
          <Button variant="outline" icon={<Edit size={20} />} onClick={handleEdit}>
            Edit
          </Button>
          <div style={{ position: 'relative' }} ref={menuRef}>
            <Button
              variant="outline"
              icon={<MoreVertical size={20} />}
              onClick={() => setIsMenuOpen(!isMenuOpen)}
            />
            {isMenuOpen && (
              <div className={styles.menu}>
                <button
                  className={styles.menuItem}
                  onClick={() => {
                    setIsMenuOpen(false)
                    handleDelete()
                  }}
                >
                  <Trash2 size={16} />
                  <span>Delete</span>
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <h1 className={styles.title}>{caseItem.title}</h1>
          <p className={styles.description}>{caseItem.description}</p>
        </div>

        <div className={styles.sections}>
          {caseItem.assignees && caseItem.assignees.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Assignees</h3>
              <div className={styles.assignees}>
                {caseItem.assignees.map((user: any) => (
                  <div key={user.id} className={styles.assignee}>
                    {user.imageUrl && (
                      <img src={user.imageUrl} alt={user.realName} className={styles.avatar} />
                    )}
                    <span>{user.realName || user.name}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {caseItem.fields && caseItem.fields.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Custom Fields</h3>
              <div className={styles.customFields}>
                {caseItem.fields.map((fieldValue) => {
                  const fieldDef = fieldDefs.find((f: any) => f.id === fieldValue.fieldId)
                  if (!fieldDef) return null
                  return (
                    <div key={fieldValue.fieldId} className={styles.customField}>
                      <div className={styles.customFieldLabel}>{fieldDef.name}:</div>
                      <div className={styles.customFieldValue}>
                        {renderFieldValue(fieldValue.fieldId, fieldValue.value)}
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {caseItem.actions && caseItem.actions.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>Related Actions</h3>
                <Button
                  variant="outline"
                  icon={<Plus size={16} />}
                  onClick={() => setIsActionFormOpen(true)}
                >
                  Add Action
                </Button>
              </div>
              <table className={styles.actionTable}>
                <thead>
                  <tr>
                    <th>Title</th>
                    <th>Status</th>
                    <th>Created</th>
                  </tr>
                </thead>
                <tbody>
                  {caseItem.actions.map((action) => (
                    <tr
                      key={action.id}
                      className={styles.actionRow}
                      onClick={() => handleActionClick(action.id)}
                    >
                      <td className={styles.titleCell}>{action.title}</td>
                      <td className={styles.statusCell}>
                        <Chip variant="status" colorIndex={STATUS_COLORS[action.status] || 0}>
                          {STATUS_LABELS[action.status] || action.status}
                        </Chip>
                      </td>
                      <td className={styles.dateCell}>
                        {new Date(action.createdAt).toLocaleDateString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {(!caseItem.actions || caseItem.actions.length === 0) && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>Related Actions</h3>
                <Button
                  variant="outline"
                  icon={<Plus size={16} />}
                  onClick={() => setIsActionFormOpen(true)}
                >
                  Add Action
                </Button>
              </div>
              <p className={styles.text}>No actions yet.</p>
            </div>
          )}

          {caseItem.knowledges && caseItem.knowledges.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>
                  <BookOpen size={20} style={{ marginRight: '0.5rem', verticalAlign: 'middle' }} />
                  Related Knowledge ({caseItem.knowledges.length})
                </h3>
              </div>
              <table className={styles.knowledgeTable}>
                <thead>
                  <tr>
                    <th>Title</th>
                    <th>Summary</th>
                    <th>Date</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {caseItem.knowledges
                    .slice(knowledgePage * knowledgePageSize, (knowledgePage + 1) * knowledgePageSize)
                    .map((knowledge) => (
                      <tr
                        key={knowledge.id}
                        className={styles.knowledgeRow}
                        onClick={() => navigate(`/knowledges/${knowledge.id}`)}
                      >
                        <td className={styles.knowledgeTitleCell}>{knowledge.title}</td>
                        <td className={styles.knowledgeSummaryCell}>
                          {knowledge.summary.length > 50
                            ? knowledge.summary.substring(0, 50) + '...'
                            : knowledge.summary}
                        </td>
                        <td className={styles.knowledgeDateCell}>
                          {new Date(knowledge.sourcedAt).toLocaleDateString()}
                        </td>
                        <td className={styles.knowledgeLinkCell}>
                          <a
                            href={knowledge.sourceURL}
                            target="_blank"
                            rel="noopener noreferrer"
                            className={styles.knowledgeExternalLink}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <ExternalLink size={16} />
                          </a>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
              {caseItem.knowledges.length > knowledgePageSize && (
                <div className={styles.pagination}>
                  <button
                    className={styles.paginationButton}
                    onClick={() => setKnowledgePage((p) => Math.max(0, p - 1))}
                    disabled={knowledgePage === 0}
                  >
                    <ChevronLeft size={16} />
                  </button>
                  <span className={styles.paginationInfo}>
                    {knowledgePage + 1} / {Math.ceil(caseItem.knowledges.length / knowledgePageSize)}
                  </span>
                  <button
                    className={styles.paginationButton}
                    onClick={() =>
                      setKnowledgePage((p) =>
                        Math.min(Math.ceil(caseItem.knowledges!.length / knowledgePageSize) - 1, p + 1)
                      )
                    }
                    disabled={knowledgePage >= Math.ceil(caseItem.knowledges.length / knowledgePageSize) - 1}
                  >
                    <ChevronRight size={16} />
                  </button>
                </div>
              )}
            </div>
          )}

          <div className={styles.metadata}>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Created:</span>
              <span className={styles.metadataValue}>
                {new Date(caseItem.createdAt).toLocaleString()}
              </span>
            </div>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Updated:</span>
              <span className={styles.metadataValue}>
                {new Date(caseItem.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>
      </div>

      <CaseForm isOpen={isFormOpen} onClose={() => setIsFormOpen(false)} caseItem={caseItem} />

      <CaseDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        caseTitle={caseItem.title}
      />

      {isActionFormOpen && (
        <ActionForm
          isOpen={isActionFormOpen}
          initialCaseID={caseItem.id}
          onClose={() => {
            setIsActionFormOpen(false)
            refetch()
          }}
        />
      )}
    </div>
  )
}
