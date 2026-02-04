import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2, Plus, ExternalLink, BookOpen, ChevronLeft, ChevronRight } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import RiskForm from './RiskForm'
import RiskDeleteDialog from './RiskDeleteDialog'
import ResponseForm from './ResponseForm'
import { GET_RISK, GET_RISK_CONFIGURATION, GET_SLACK_USERS } from '../graphql/risk'
import styles from './RiskDetail.module.css'

interface Knowledge {
  id: string
  riskID: number
  sourceID: string
  sourceURL: string
  title: string
  summary: string
  sourcedAt: string
  createdAt: string
  updatedAt: string
}

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
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  detectionIndicators: string
  responses?: Array<{
    id: number
    title: string
    status: string
    responders: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
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

export default function RiskDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isResponseFormOpen, setIsResponseFormOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const [knowledgePage, setKnowledgePage] = useState(0)
  const knowledgePageSize = 5
  const menuRef = useRef<HTMLDivElement>(null)

  const { data: riskData, loading: riskLoading, error: riskError, refetch } = useQuery(GET_RISK, {
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

  const handleResponseClick = (responseId: number) => {
    navigate(`/responses/${responseId}`)
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
  const categoryMap = new Map(allCategories.map((c: any, index: number) => [c.id, { ...c, index }]))
  const categories = risk.categoryIDs
    .map(id => categoryMap.get(id))
    .filter(Boolean)

  const likelihood = configData?.riskConfiguration?.likelihoodLevels.find(
    (l: any) => l.id === risk.likelihoodID
  )

  const impact = configData?.riskConfiguration?.impactLevels.find(
    (i: any) => i.id === risk.impactID
  )

  const allTeams = configData?.riskConfiguration?.teams || []
  const teamMap = new Map(allTeams.map((t: any, index: number) => [t.id, { ...t, index }]))
  const teams = risk.responseTeamIDs
    .map(id => teamMap.get(id))
    .filter(Boolean)

  const userMap = new Map((usersData?.slackUsers || []).map((u: any) => [u.id, u]))
  const assignees = risk.assigneeIDs
    .map(id => userMap.get(id))
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
                {categories.map((cat: any) => (
                  <Chip key={cat.id} variant="category" colorIndex={cat.index}>
                    {cat.name}
                  </Chip>
                ))}
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
                {teams.map((team: any) => (
                  <Chip key={team.id} variant="team" colorIndex={team.index}>
                    {team.name}
                  </Chip>
                ))}
              </div>
            </div>
          )}

          {assignees && assignees.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Assignees</h3>
              <div className={styles.assignees}>
                {assignees.map((user: any) => (
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

          {risk.detectionIndicators && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Detection Indicators / Triggers</h3>
              <p className={styles.text}>{risk.detectionIndicators}</p>
            </div>
          )}

          {risk.responses && risk.responses.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>Related Responses</h3>
                <Button
                  variant="outline"
                  icon={<Plus size={16} />}
                  onClick={() => setIsResponseFormOpen(true)}
                >
                  Add Response
                </Button>
              </div>
              <table className={styles.responseTable}>
                <thead>
                  <tr>
                    <th>Title</th>
                    <th>Responders</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {risk.responses.map((response) => (
                    <tr
                      key={response.id}
                      className={styles.responseRow}
                      onClick={() => handleResponseClick(response.id)}
                    >
                      <td className={styles.titleCell}>{response.title}</td>
                      <td className={styles.respondersCell}>
                        <div className={styles.responders}>
                          {response.responders.map((user) => (
                            <div key={user.id} className={styles.responder}>
                              {user.imageUrl && (
                                <img src={user.imageUrl} alt={user.realName} className={styles.responderAvatar} />
                              )}
                              <span>{user.realName || user.name}</span>
                            </div>
                          ))}
                        </div>
                      </td>
                      <td className={styles.statusCell}>
                        <Chip variant="status" colorIndex={STATUS_COLORS[response.status] || 0}>
                          {STATUS_LABELS[response.status] || response.status}
                        </Chip>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {(!risk.responses || risk.responses.length === 0) && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>Related Responses</h3>
                <Button
                  variant="outline"
                  icon={<Plus size={16} />}
                  onClick={() => setIsResponseFormOpen(true)}
                >
                  Add Response
                </Button>
              </div>
              <p className={styles.text}>No responses yet.</p>
            </div>
          )}

          {risk.knowledges && risk.knowledges.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>
                  <BookOpen size={20} style={{ marginRight: '0.5rem', verticalAlign: 'middle' }} />
                  Related Knowledge ({risk.knowledges.length})
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
                  {risk.knowledges
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
              {risk.knowledges.length > knowledgePageSize && (
                <div className={styles.pagination}>
                  <button
                    className={styles.paginationButton}
                    onClick={() => setKnowledgePage((p) => Math.max(0, p - 1))}
                    disabled={knowledgePage === 0}
                  >
                    <ChevronLeft size={16} />
                  </button>
                  <span className={styles.paginationInfo}>
                    {knowledgePage + 1} / {Math.ceil(risk.knowledges.length / knowledgePageSize)}
                  </span>
                  <button
                    className={styles.paginationButton}
                    onClick={() =>
                      setKnowledgePage((p) =>
                        Math.min(Math.ceil(risk.knowledges!.length / knowledgePageSize) - 1, p + 1)
                      )
                    }
                    disabled={knowledgePage >= Math.ceil(risk.knowledges.length / knowledgePageSize) - 1}
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

      {isResponseFormOpen && (
        <ResponseForm
          initialRiskID={risk.id}
          onClose={() => {
            setIsResponseFormOpen(false)
            refetch()
          }}
        />
      )}
    </div>
  )
}
