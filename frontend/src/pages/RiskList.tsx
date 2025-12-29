import { useState } from 'react'
import { useQuery, useMutation } from '@apollo/client'
import { Plus, Edit, Trash2 } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import RiskForm from './RiskForm'
import RiskDeleteDialog from './RiskDeleteDialog'
import { GET_RISKS, DELETE_RISK } from '../graphql/risk'
import styles from './RiskList.module.css'
import type { ReactElement } from 'react'

interface Risk {
  id: number
  name: string
  description: string
  createdAt: string
  updatedAt: string
}

export default function RiskList() {
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [selectedRisk, setSelectedRisk] = useState<Risk | null>(null)

  const { data, loading, error } = useQuery(GET_RISKS)
  const [deleteRisk] = useMutation(DELETE_RISK, {
    update(cache) {
      if (!selectedRisk) return
      const normalizedId = cache.identify({ id: selectedRisk.id, __typename: 'Risk' })
      cache.evict({ id: normalizedId })
      cache.gc()
    },
    onCompleted: () => {
      setIsDeleteDialogOpen(false)
      setSelectedRisk(null)
    },
  })

  const handleEdit = (risk: Risk) => {
    setSelectedRisk(risk)
    setIsFormOpen(true)
  }

  const handleDelete = (risk: Risk) => {
    setSelectedRisk(risk)
    setIsDeleteDialogOpen(true)
  }

  const handleFormClose = () => {
    setIsFormOpen(false)
    setSelectedRisk(null)
  }

  const handleDeleteConfirm = () => {
    if (selectedRisk) {
      deleteRisk({ variables: { id: selectedRisk.id } })
    }
  }

  const columns = [
    {
      header: 'ID',
      accessor: 'id' as keyof Risk,
      width: '80px',
    },
    {
      header: 'Name',
      accessor: 'name' as keyof Risk,
    },
    {
      header: 'Description',
      accessor: 'description' as keyof Risk,
    },
    {
      header: 'Created',
      accessor: ((risk: Risk) => new Date(risk.createdAt).toLocaleDateString()) as (row: Risk) => string,
      width: '120px',
    },
    {
      header: 'Actions',
      accessor: ((risk: Risk) => (
        <div className={styles.actions}>
          <button
            className={styles.actionButton}
            onClick={(e) => {
              e.stopPropagation()
              handleEdit(risk)
            }}
          >
            <Edit size={16} />
          </button>
          <button
            className={`${styles.actionButton} ${styles.danger}`}
            onClick={(e) => {
              e.stopPropagation()
              handleDelete(risk)
            }}
          >
            <Trash2 size={16} />
          </button>
        </div>
      )) as (row: Risk) => ReactElement,
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
          <h2 className={styles.title}>Risk Management</h2>
          <p className={styles.subtitle}>Manage and track security risks</p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={20} />}
          onClick={() => setIsFormOpen(true)}
        >
          New Risk
        </Button>
      </div>

      <div className={styles.tableWrapper}>
        <Table columns={columns} data={data?.risks || []} />
      </div>

      <RiskForm
        isOpen={isFormOpen}
        onClose={handleFormClose}
        risk={selectedRisk}
      />

      <RiskDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        riskName={selectedRisk?.name || ''}
      />
    </div>
  )
}
