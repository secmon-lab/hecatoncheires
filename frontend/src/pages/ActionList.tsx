import { useState, useEffect, useMemo } from 'react'
import { useQuery, useMutation } from '@apollo/client'
import { useNavigate, useParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import Button from '../components/Button'
import KanbanBoard from '../components/KanbanBoard'
import ActionFilterBar from '../components/ActionFilterBar'
import ActionModal from './ActionModal'
import ActionForm from './ActionForm'
import { GET_OPEN_CASE_ACTIONS, UPDATE_ACTION } from '../graphql/action'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './ActionList.module.css'

interface Action {
  id: number
  caseID: number
  case?: { id: number; title: string }
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackMessageTS: string
  status: string
  createdAt: string
  updatedAt: string
}

export default function ActionList() {
  const navigate = useNavigate()
  const { actionId } = useParams<{ actionId?: string }>()
  const { currentWorkspace } = useWorkspace()
  const [isCreateFormOpen, setIsCreateFormOpen] = useState(false)
  const [selectedActionId, setSelectedActionId] = useState<number | null>(null)

  // Filter state
  const [searchText, setSearchText] = useState('')
  const [selectedAssigneeIDs, setSelectedAssigneeIDs] = useState<string[]>([])

  // Optimistic status overrides to avoid snap-back on drag
  const [statusOverrides, setStatusOverrides] = useState<Record<number, string>>({})

  const { data, loading, error } = useQuery(GET_OPEN_CASE_ACTIONS, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })

  const [updateAction] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace!.id } },
    ],
  })

  // Handle permalink: open modal if actionId is in URL
  useEffect(() => {
    if (actionId) {
      setSelectedActionId(parseInt(actionId))
    }
  }, [actionId])

  const filteredActions = useMemo(() => {
    const actions: Action[] = data?.openCaseActions || []
    return actions
      .map((action) =>
        statusOverrides[action.id] ? { ...action, status: statusOverrides[action.id] } : action
      )
      .filter((action) => {
        // Text search
        if (searchText) {
          const search = searchText.toLowerCase()
          const matchesTitle = action.title.toLowerCase().includes(search)
          const matchesDescription = (action.description || '').toLowerCase().includes(search)
          const matchesCaseName = (action.case?.title || '').toLowerCase().includes(search)
          if (!matchesTitle && !matchesDescription && !matchesCaseName) {
            return false
          }
        }

        // Assignee filter
        if (selectedAssigneeIDs.length > 0) {
          const hasMatchingAssignee = action.assigneeIDs.some((id) =>
            selectedAssigneeIDs.includes(id)
          )
          if (!hasMatchingAssignee) {
            return false
          }
        }

        return true
      })
  }, [data, searchText, selectedAssigneeIDs, statusOverrides])

  const handleStatusChange = async (actionId: number, newStatus: string) => {
    // Immediately reflect the change locally
    setStatusOverrides((prev) => ({ ...prev, [actionId]: newStatus }))
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: actionId, status: newStatus },
      },
    })
    // Clear override after refetch completes
    setStatusOverrides((prev) => {
      const next = { ...prev }
      delete next[actionId]
      return next
    })
  }

  const handleCardClick = (action: Action) => {
    setSelectedActionId(action.id)
    navigate(`/ws/${currentWorkspace!.id}/actions/${action.id}`, { replace: true })
  }

  const handleModalClose = () => {
    setSelectedActionId(null)
    navigate(`/ws/${currentWorkspace!.id}/actions`, { replace: true })
  }

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
          <h2 className={styles.title}>{currentWorkspace?.name} Actions</h2>
          <p className={styles.subtitle}>Manage and track actions</p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={20} />}
          onClick={() => setIsCreateFormOpen(true)}
        >
          New Action
        </Button>
      </div>

      <ActionFilterBar
        searchText={searchText}
        onSearchTextChange={setSearchText}
        selectedAssigneeIDs={selectedAssigneeIDs}
        onAssigneeChange={setSelectedAssigneeIDs}
      />

      <KanbanBoard
        actions={filteredActions}
        onCardClick={handleCardClick}
        onStatusChange={handleStatusChange}
      />

      <ActionModal
        actionId={selectedActionId}
        isOpen={selectedActionId !== null}
        onClose={handleModalClose}
      />

      <ActionForm
        isOpen={isCreateFormOpen}
        onClose={() => setIsCreateFormOpen(false)}
        action={null}
      />
    </div>
  )
}
