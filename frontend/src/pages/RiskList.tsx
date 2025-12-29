import { useState } from 'react'
import { useQuery, useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus, Edit, Trash2 } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import RiskForm from './RiskForm'
import RiskDeleteDialog from './RiskDeleteDialog'
import { GET_RISKS, DELETE_RISK, GET_RISK_CONFIGURATION, GET_SLACK_USERS } from '../graphql/risk'
import styles from './RiskList.module.css'
import type { ReactElement } from 'react'

interface Risk {
  id: number
  name: string
  description: string
  categoryIDs: string[]
  specificImpact: string
  likelihoodID: string
  impactID: string
  responseTeamIDs: string[]
  assigneeIDs: string[]
  detectionIndicators: string
  createdAt: string
  updatedAt: string
}

export default function RiskList() {
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [selectedRisk, setSelectedRisk] = useState<Risk | null>(null)

  const { data, loading, error } = useQuery(GET_RISKS)
  const { data: configData } = useQuery(GET_RISK_CONFIGURATION)
  const { data: usersData } = useQuery(GET_SLACK_USERS)
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

  const handleRowClick = (risk: Risk) => {
    navigate(`/risks/${risk.id}`)
  }

  const renderCategories = (categoryIDs: string[]) => {
    if (!configData?.riskConfiguration?.categories) return null
    const allCategories = configData.riskConfiguration.categories
    const categories = categoryIDs
      .map(id => allCategories.find((cat: { id: string; name: string }) => cat.id === id))
      .filter(Boolean)

    return (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {categories.map((cat: { id: string; name: string }) => {
          const index = allCategories.findIndex((c: { id: string }) => c.id === cat.id)
          return (
            <Chip key={cat.id} variant="category" colorIndex={index}>
              {cat.name}
            </Chip>
          )
        })}
      </div>
    )
  }

  const renderTeams = (teamIDs: string[]) => {
    if (!configData?.riskConfiguration?.teams) return null
    const allTeams = configData.riskConfiguration.teams
    const teams = teamIDs
      .map(id => allTeams.find((team: { id: string; name: string }) => team.id === id))
      .filter(Boolean)

    return (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {teams.map((team: { id: string; name: string }) => {
          const index = allTeams.findIndex((t: { id: string }) => t.id === team.id)
          return (
            <Chip key={team.id} variant="team" colorIndex={index}>
              {team.name}
            </Chip>
          )
        })}
      </div>
    )
  }

  const renderAssignees = (assigneeIDs: string[]) => {
    if (!usersData?.slackUsers) return null
    const users = assigneeIDs
      .map(id => usersData.slackUsers.find((user: { id: string; realName: string; imageUrl?: string }) => user.id === id))
      .filter(Boolean)

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        {users.map((user: { id: string; realName: string; imageUrl?: string }) => (
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
      accessor: 'id' as keyof Risk,
      width: '80px',
    },
    {
      header: 'Name',
      accessor: 'name' as keyof Risk,
    },
    {
      header: 'Category',
      accessor: ((risk: Risk) => renderCategories(risk.categoryIDs)) as (row: Risk) => ReactElement,
    },
    {
      header: 'Response Team',
      accessor: ((risk: Risk) => renderTeams(risk.responseTeamIDs)) as (row: Risk) => ReactElement,
    },
    {
      header: 'Assignee',
      accessor: ((risk: Risk) => renderAssignees(risk.assigneeIDs)) as (row: Risk) => ReactElement,
    },
    {
      header: 'Created',
      accessor: ((risk: Risk) => new Date(risk.createdAt).toLocaleDateString()) as (row: Risk) => string,
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
          <p className={styles.subtitle}>Manage and track risks</p>
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
        <Table columns={columns} data={data?.risks || []} onRowClick={handleRowClick} />
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
