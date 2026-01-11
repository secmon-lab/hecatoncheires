import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import ResponseForm from './ResponseForm'
import { GET_RESPONSES } from '../graphql/response'
import styles from './ResponseList.module.css'

interface Response {
  id: number
  title: string
  description: string
  responders: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  url?: string
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

export default function ResponseList() {
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [statusFilter, setStatusFilter] = useState<string | null>(null)

  const { data, loading, error } = useQuery(GET_RESPONSES)

  const handleFormClose = () => {
    setIsFormOpen(false)
  }

  const handleRowClick = (response: Response) => {
    navigate(`/responses/${response.id}`)
  }

  const renderStatus = (status: string) => {
    return (
      <Chip variant="status" colorIndex={STATUS_COLORS[status] || 0}>
        {STATUS_LABELS[status] || status}
      </Chip>
    )
  }

  const renderResponders = (responders: Response['responders']) => {
    if (!responders || responders.length === 0) return null

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        {responders.map((user) => (
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
    { header: 'Title', accessor: 'title' as keyof Response, width: '280px' },
    { header: 'Status', accessor: (response: Response) => renderStatus(response.status), width: '100px' },
    { header: 'Responders', accessor: (response: Response) => renderResponders(response.responders) },
    { header: 'Description', accessor: 'description' as keyof Response },
  ]

  if (loading) return <div className={styles.loading}>Loading...</div>
  if (error) return <div className={styles.error}>Error: {error.message}</div>

  const responses = data?.responses || []
  const filteredResponses = statusFilter
    ? responses.filter((r: Response) => r.status === statusFilter)
    : responses

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div>
          <h1>Responses</h1>
          <p>Manage risk response actions</p>
        </div>
        <Button icon={<Plus size={20} />} onClick={() => setIsFormOpen(true)}>
          New Response
        </Button>
      </div>

      <div className={styles.filters}>
        <button
          className={`${styles.filterButton} ${statusFilter === null ? styles.active : ''}`}
          onClick={() => setStatusFilter(null)}
        >
          ALL
        </button>
        {Object.entries(STATUS_LABELS).map(([status, label]) => (
          <button
            key={status}
            className={`${styles.filterButton} ${statusFilter === status ? styles.active : ''}`}
            onClick={() => setStatusFilter(status)}
          >
            {label.toUpperCase()}
          </button>
        ))}
      </div>

      <Table
        columns={columns}
        data={filteredResponses}
        onRowClick={handleRowClick}
      />

      {isFormOpen && <ResponseForm onClose={handleFormClose} />}
    </div>
  )
}
