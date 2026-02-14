import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import CaseForm from './CaseForm'
import { GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './CaseList.module.css'
import type { ReactElement } from 'react'

type CaseStatus = 'OPEN' | 'CLOSED'

interface Case {
  id: number
  title: string
  description: string
  status: CaseStatus
  assigneeIDs: string[]
  assignees: Array<{ id: string; realName: string; imageUrl?: string }>
  slackChannelID: string
  slackChannelName: string
  createdAt: string
  updatedAt: string
  fields: Array<{ fieldId: string; value: any }>
}

export default function CaseList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [statusFilter, setStatusFilter] = useState<CaseStatus>('OPEN')

  const { data, loading, error } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace!.id, status: statusFilter },
    skip: !currentWorkspace,
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })

  const handleFormClose = () => {
    setIsFormOpen(false)
  }

  const handleRowClick = (caseItem: Case) => {
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

      case 'SELECT':
        const option = fieldDef.options?.find((opt: any) => opt.id === fieldValue.value)
        return option ? option.name : fieldValue.value

      case 'MULTI_SELECT':
        const selectedOptions = (fieldValue.value || [])
          .map((id: string) => fieldDef.options?.find((opt: any) => opt.id === id)?.name)
          .filter(Boolean)
        return selectedOptions.length > 0 ? selectedOptions.join(', ') : '-'

      case 'USER':
      case 'MULTI_USER':
        return '-'

      default:
        return String(fieldValue.value || '-')
    }
  }

  const columns = [
    {
      header: 'ID',
      accessor: 'id' as keyof Case,
      width: '48px',
    },
    {
      header: 'Title',
      accessor: 'title' as keyof Case,
      width: '200px',
    },
    {
      header: 'Assignees',
      accessor: ((caseItem: Case) => renderAssignees(caseItem.assignees)) as (row: Case) => ReactElement | null,
      width: '200px',
    },
    {
      header: 'Created',
      accessor: ((caseItem: Case) => new Date(caseItem.createdAt).toLocaleDateString()) as (row: Case) => string,
      width: '120px',
    },
  ]

  // Add custom field columns
  if (configData?.fieldConfiguration?.fields) {
    configData.fieldConfiguration.fields.forEach((field: any) => {
      columns.push({
        header: field.name,
        accessor: ((caseItem: Case) => renderFieldValue(caseItem, field.id)) as (row: Case) => string,
        width: '150px',
      })
    })
  }

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

      <div className={styles.tabs}>
        <button
          className={`${styles.tab} ${statusFilter === 'OPEN' ? styles.tabActive : ''}`}
          onClick={() => setStatusFilter('OPEN')}
        >
          Open
        </button>
        <button
          className={`${styles.tab} ${statusFilter === 'CLOSED' ? styles.tabActive : ''}`}
          onClick={() => setStatusFilter('CLOSED')}
        >
          Closed
        </button>
      </div>

      <div className={styles.tableWrapper}>
        <Table columns={columns} data={data?.cases || []} onRowClick={handleRowClick} />
      </div>

      <CaseForm
        isOpen={isFormOpen}
        onClose={handleFormClose}
        caseItem={null}
      />
    </div>
  )
}
