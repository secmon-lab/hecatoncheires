import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2 } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import ActionForm from './ActionForm'
import ActionDeleteDialog from './ActionDeleteDialog'
import { GET_ACTION, UPDATE_ACTION } from '../graphql/action'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './ActionDetail.module.css'

interface Action {
  id: number
  caseID: number
  case?: {
    id: number
    title: string
  }
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackMessageTS: string
  status: string
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

export default function ActionDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const { data: actionData, loading: actionLoading, error: actionError, refetch } = useQuery(GET_ACTION, {
    variables: { workspaceId: currentWorkspace!.id, id: parseInt(id || '0') },
    skip: !id || !currentWorkspace,
  })

  const [updateAction, { loading: statusUpdating }] = useMutation(UPDATE_ACTION, {
    onCompleted: () => {
      refetch()
    },
    onError: (error) => {
      console.error('Failed to update status:', error)
    },
  })

  const action: Action | undefined = actionData?.action

  const handleStatusChange = async (newStatus: string) => {
    if (!action || newStatus === action.status) return
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          id: action.id,
          status: newStatus,
        },
      },
    })
  }

  const handleBack = () => {
    navigate(`/ws/${currentWorkspace!.id}/actions`)
  }

  const handleEdit = () => {
    setIsFormOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate(`/ws/${currentWorkspace!.id}/actions`)
  }

  const handleCaseClick = () => {
    if (action?.case) {
      navigate(`/ws/${currentWorkspace!.id}/cases/${action.case.id}`)
    }
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

  if (actionLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (actionError || !action) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {actionError ? `Error: ${actionError.message}` : 'Action not found'}
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
          <h1 className={styles.title}>{action.title}</h1>
          <p className={styles.description}>{action.description}</p>
        </div>

        <div className={styles.sections}>
          {action.case && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Related Case</h3>
              <div className={styles.caseLink} onClick={handleCaseClick}>
                <span className={styles.caseLinkText}>
                  {action.case.title} (ID: {action.case.id})
                </span>
              </div>
            </div>
          )}

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Status</h3>
            <div className={styles.statusDropdownWrapper}>
              <select
                value={action.status}
                onChange={(e) => handleStatusChange(e.target.value)}
                disabled={statusUpdating}
                className={styles.statusDropdown}
                data-testid="status-dropdown"
              >
                {Object.entries(STATUS_LABELS).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
              <Chip variant="status" colorIndex={STATUS_COLORS[action.status] || 0}>
                {statusUpdating ? 'Updating...' : (STATUS_LABELS[action.status] || action.status)}
              </Chip>
            </div>
          </div>

          {action.assignees && action.assignees.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Assignees</h3>
              <div className={styles.assignees}>
                {action.assignees.map((user: any) => (
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

          {action.slackMessageTS && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Slack Message</h3>
              <p className={styles.text}>Message TS: {action.slackMessageTS}</p>
            </div>
          )}

          <div className={styles.metadata}>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Created:</span>
              <span className={styles.metadataValue}>
                {new Date(action.createdAt).toLocaleString()}
              </span>
            </div>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Updated:</span>
              <span className={styles.metadataValue}>
                {new Date(action.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>
      </div>

      <ActionForm isOpen={isFormOpen} onClose={() => setIsFormOpen(false)} action={action} />

      <ActionDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        actionTitle={action.title}
        actionId={action.id}
      />
    </div>
  )
}
