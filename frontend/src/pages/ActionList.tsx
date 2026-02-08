import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import ActionForm from './ActionForm'
import { GET_ACTIONS } from '../graphql/action'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './ActionList.module.css'
import type { ReactElement } from 'react'

interface Action {
  id: number
  caseID: number
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; realName: string; imageUrl?: string }>
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

export default function ActionList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [isFormOpen, setIsFormOpen] = useState(false)

  const { data, loading, error } = useQuery(GET_ACTIONS, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })
  const handleFormClose = () => {
    setIsFormOpen(false)
  }

  const handleRowClick = (action: Action) => {
    navigate(`/ws/${currentWorkspace!.id}/actions/${action.id}`)
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

  const columns = [
    {
      header: 'ID',
      accessor: 'id' as keyof Action,
      width: '48px',
    },
    {
      header: 'Title',
      accessor: 'title' as keyof Action,
      width: '200px',
    },
    {
      header: 'Case ID',
      accessor: 'caseID' as keyof Action,
      width: '80px',
    },
    {
      header: 'Status',
      accessor: ((action: Action) => (
        <Chip variant="status" colorIndex={STATUS_COLORS[action.status] || 0}>
          {STATUS_LABELS[action.status] || action.status}
        </Chip>
      )) as (row: Action) => ReactElement,
      width: '150px',
    },
    {
      header: 'Assignees',
      accessor: ((action: Action) => renderAssignees(action.assignees)) as (row: Action) => ReactElement | null,
      width: '200px',
    },
    {
      header: 'Created',
      accessor: ((action: Action) => new Date(action.createdAt).toLocaleDateString()) as (row: Action) => string,
      width: '120px',
    },
  ]

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
          onClick={() => setIsFormOpen(true)}
        >
          New Action
        </Button>
      </div>

      <div className={styles.tableWrapper}>
        <Table columns={columns} data={data?.actions || []} onRowClick={handleRowClick} />
      </div>

      <ActionForm
        isOpen={isFormOpen}
        onClose={handleFormClose}
        action={null}
      />
    </div>
  )
}
