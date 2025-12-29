import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2 } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import RiskForm from './RiskForm'
import RiskDeleteDialog from './RiskDeleteDialog'
import { GET_RISK, GET_RISK_CONFIGURATION, GET_SLACK_USERS } from '../graphql/risk'
import styles from './RiskDetail.module.css'

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

export default function RiskDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const { data: riskData, loading: riskLoading, error: riskError } = useQuery(GET_RISK, {
    variables: { id: parseInt(id || '0') },
    skip: !id,
  })

  const { data: configData } = useQuery(GET_RISK_CONFIGURATION)
  const { data: usersData } = useQuery(GET_SLACK_USERS)

  const risk: Risk | undefined = riskData?.risk

  const handleBack = () => {
    navigate('/risks')
  }

  const handleEdit = () => {
    setIsFormOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate('/risks')
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

  if (riskLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (riskError || !risk) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {riskError ? `Error: ${riskError.message}` : 'Risk not found'}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back to List
        </Button>
      </div>
    )
  }

  // Get configuration items
  const allCategories = configData?.riskConfiguration?.categories || []
  const categories = risk.categoryIDs
    .map(id => allCategories.find((c: any) => c.id === id))
    .filter(Boolean)

  const likelihood = configData?.riskConfiguration.likelihoodLevels.find(
    (l: any) => l.id === risk.likelihoodID
  )

  const impact = configData?.riskConfiguration.impactLevels.find(
    (i: any) => i.id === risk.impactID
  )

  const allTeams = configData?.riskConfiguration?.teams || []
  const teams = risk.responseTeamIDs
    .map(id => allTeams.find((t: any) => t.id === id))
    .filter(Boolean)

  const assignees = risk.assigneeIDs
    .map(id => usersData?.slackUsers.find((u: any) => u.id === id))
    .filter(Boolean)

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
          <h1 className={styles.title}>{risk.name}</h1>
          <p className={styles.description}>{risk.description}</p>
        </div>

        <div className={styles.sections}>
          {categories && categories.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Categories</h3>
              <div className={styles.chips}>
                {categories.map((cat: any) => {
                  const index = allCategories.findIndex((c: any) => c.id === cat.id)
                  return (
                    <Chip key={cat.id} variant="category" colorIndex={index}>
                      {cat.name}
                    </Chip>
                  )
                })}
              </div>
            </div>
          )}

          {risk.specificImpact && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Specific Impact</h3>
              <p className={styles.text}>{risk.specificImpact}</p>
            </div>
          )}

          <div className={styles.row}>
            {likelihood && (
              <div className={styles.section}>
                <h3 className={styles.sectionTitle}>Likelihood</h3>
                <div className={styles.levelCard}>
                  <div className={styles.levelName}>{likelihood.name}</div>
                  <div className={styles.levelScore}>Score: {likelihood.score}</div>
                  {likelihood.description && (
                    <div className={styles.levelDescription}>{likelihood.description}</div>
                  )}
                </div>
              </div>
            )}

            {impact && (
              <div className={styles.section}>
                <h3 className={styles.sectionTitle}>Impact</h3>
                <div className={styles.levelCard}>
                  <div className={styles.levelName}>{impact.name}</div>
                  <div className={styles.levelScore}>Score: {impact.score}</div>
                  {impact.description && (
                    <div className={styles.levelDescription}>{impact.description}</div>
                  )}
                </div>
              </div>
            )}
          </div>

          {teams && teams.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Response Teams</h3>
              <div className={styles.chips}>
                {teams.map((team: any) => {
                  const index = allTeams.findIndex((t: any) => t.id === team.id)
                  return (
                    <Chip key={team.id} variant="team" colorIndex={index}>
                      {team.name}
                    </Chip>
                  )
                })}
              </div>
            </div>
          )}

          {assignees && assignees.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Assignees</h3>
              <div className={styles.chips}>
                {assignees.map((user: any) => (
                  <Chip key={user.id} variant="user">
                    {user.realName || user.name}
                  </Chip>
                ))}
              </div>
            </div>
          )}

          {risk.detectionIndicators && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Detection Indicators / Triggers</h3>
              <p className={styles.text}>{risk.detectionIndicators}</p>
            </div>
          )}

          <div className={styles.metadata}>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Created:</span>
              <span className={styles.metadataValue}>
                {new Date(risk.createdAt).toLocaleString()}
              </span>
            </div>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Updated:</span>
              <span className={styles.metadataValue}>
                {new Date(risk.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>
      </div>

      <RiskForm isOpen={isFormOpen} onClose={() => setIsFormOpen(false)} risk={risk} />

      <RiskDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        riskName={risk.name}
      />
    </div>
  )
}
