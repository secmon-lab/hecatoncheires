import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import RiskForm from './RiskForm'
import { GET_RISKS, GET_RISK_CONFIGURATION, GET_SLACK_USERS } from '../graphql/risk'
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

  const { data, loading, error } = useQuery(GET_RISKS)
  const { data: configData } = useQuery(GET_RISK_CONFIGURATION)
  const { data: usersData } = useQuery(GET_SLACK_USERS)

  const handleFormClose = () => {
    setIsFormOpen(false)
  }

  const handleRowClick = (risk: Risk) => {
    navigate(`/risks/${risk.id}`)
  }

  const renderCategories = (categoryIDs: string[]) => {
    if (!configData?.riskConfiguration?.categories) return null
    const allCategories = configData.riskConfiguration.categories
    const categoryMap = new Map(allCategories.map((cat: { id: string; name: string }, index: number) => [cat.id, { ...cat, index }]))
    const categories = categoryIDs
      .map(id => categoryMap.get(id))
      .filter((cat): cat is { id: string; name: string; index: number } => cat !== undefined)

    return (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {categories.map((cat) => (
          <Chip key={cat.id} variant="category" colorIndex={cat.index}>
            {cat.name}
          </Chip>
        ))}
      </div>
    )
  }

  const renderTeams = (teamIDs: string[]) => {
    if (!configData?.riskConfiguration?.teams) return null
    const allTeams = configData.riskConfiguration.teams
    const teamMap = new Map(allTeams.map((team: { id: string; name: string }, index: number) => [team.id, { ...team, index }]))
    const teams = teamIDs
      .map(id => teamMap.get(id))
      .filter((team): team is { id: string; name: string; index: number } => team !== undefined)

    return (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {teams.map((team) => (
          <Chip key={team.id} variant="team" colorIndex={team.index}>
            {team.name}
          </Chip>
        ))}
      </div>
    )
  }

  const renderAssignees = (assigneeIDs: string[]) => {
    if (!usersData?.slackUsers) return null
    const userMap = new Map(usersData.slackUsers.map((user: { id: string; realName: string; imageUrl?: string }) => [user.id, user]))
    const users = assigneeIDs
      .map(id => userMap.get(id))
      .filter((user): user is { id: string; realName: string; imageUrl?: string } => user !== undefined)

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        {users.map((user) => (
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
        risk={null}
      />
    </div>
  )
}
