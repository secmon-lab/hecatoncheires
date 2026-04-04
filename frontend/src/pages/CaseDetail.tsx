import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2, Plus, ExternalLink, BookOpen, Bot, ChevronLeft, ChevronRight, XCircle, RotateCcw, ClipboardList, Lock, Users, RefreshCw, Search } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Markdown from 'react-markdown'
import Button from '../components/Button'
import Chip from '../components/Chip'
import Modal from '../components/Modal'
import CaseForm from './CaseForm'
import CaseDeleteDialog from './CaseDeleteDialog'
import ActionForm from './ActionForm'
import ActionModal from './ActionModal'
import { GET_CASE, GET_CASES, CLOSE_CASE, REOPEN_CASE, GET_CASE_MEMBERS, SYNC_CASE_CHANNEL_USERS } from '../graphql/case'
import { GET_ASSIST_LOGS } from '../graphql/assistLog'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './CaseDetail.module.css'

interface Knowledge {
  id: string
  caseID: number
  sourceID: string
  sourceURLs: string[]
  title: string
  summary: string
  sourcedAt: string
  createdAt: string
  updatedAt: string
}

interface Case {
  id: number
  title: string
  description: string
  status: 'OPEN' | 'CLOSED'
  isPrivate: boolean
  accessDenied: boolean
  channelUserCount: number
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackChannelID: string
  slackChannelName: string
  slackChannelURL: string
  fields: Array<{ fieldId: string; value: any }>
  actions?: Array<{
    id: number
    title: string
    status: string
    assigneeIDs: string[]
    assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
    dueDate?: string
    createdAt: string
  }>
  knowledges?: Knowledge[]
  createdAt: string
  updatedAt: string
}

const STATUS_COLORS: Record<string, number> = {
  BACKLOG: 0,
  TODO: 1,
  IN_PROGRESS: 2,
  BLOCKED: 3,
  COMPLETED: 4,
}

export default function CaseDetail() {
  const { id, actionId } = useParams<{ id: string; actionId?: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isCloseDialogOpen, setIsCloseDialogOpen] = useState(false)
  const [isActionFormOpen, setIsActionFormOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const [selectedActionId, setSelectedActionId] = useState<number | null>(null)
  const [knowledgePage, setKnowledgePage] = useState(0)
  const knowledgePageSize = 5
  const [memberPage, setMemberPage] = useState(0)
  const [memberFilter, setMemberFilter] = useState('')
  const [memberFilterDebounced, setMemberFilterDebounced] = useState('')
  const memberPageSize = 20
  const menuRef = useRef<HTMLDivElement>(null)

  const STATUS_LABELS: Record<string, string> = {
    BACKLOG: t('statusBacklog'),
    TODO: t('statusTodo'),
    IN_PROGRESS: t('statusInProgress'),
    BLOCKED: t('statusBlocked'),
    COMPLETED: t('statusCompleted'),
  }

  // Debounce member filter
  useEffect(() => {
    const timer = setTimeout(() => {
      setMemberFilterDebounced(memberFilter)
      setMemberPage(0)
    }, 300)
    return () => clearTimeout(timer)
  }, [memberFilter])

  // Handle permalink: open action modal if actionId is in URL
  useEffect(() => {
    if (actionId) {
      setSelectedActionId(parseInt(actionId))
    }
  }, [actionId])

  const { data: caseData, loading: caseLoading, error: caseError, refetch } = useQuery(GET_CASE, {
    variables: { workspaceId: currentWorkspace!.id, id: parseInt(id || '0') },
    skip: !id || !currentWorkspace,
  })

  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })

  const caseId = id ? parseInt(id, 10) : 0
  const { data: assistLogData } = useQuery(GET_ASSIST_LOGS, {
    variables: {
      workspaceId: currentWorkspace!.id,
      caseId,
      limit: 3,
      offset: 0,
    },
    skip: !currentWorkspace || !caseId,
  })

  const { data: memberData, loading: memberLoading, refetch: refetchMembers } = useQuery(GET_CASE_MEMBERS, {
    variables: {
      workspaceId: currentWorkspace!.id,
      id: caseId,
      limit: memberPageSize,
      offset: memberPage * memberPageSize,
      filter: memberFilterDebounced,
    },
    skip: !currentWorkspace || !caseId,
  })

  const [syncMembers, { loading: syncing }] = useMutation(SYNC_CASE_CHANNEL_USERS, {
    variables: { workspaceId: currentWorkspace!.id, id: caseId },
    onCompleted: () => {
      refetch()
      refetchMembers()
    },
  })

  const [closeCase] = useMutation(CLOSE_CASE)
  const [reopenCase] = useMutation(REOPEN_CASE)

  const caseItem: Case | undefined = caseData?.case
  const fieldDefs = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'
  const assistLogTotalCount = assistLogData?.assistLogs?.totalCount || 0

  const handleBack = () => {
    navigate(`/ws/${currentWorkspace!.id}/cases`)
  }

  const handleEdit = () => {
    setIsFormOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate(`/ws/${currentWorkspace!.id}/cases`)
  }

  const handleCloseCase = async () => {
    if (!caseItem) return
    try {
      await closeCase({
        variables: { workspaceId: currentWorkspace!.id, id: caseItem.id },
        refetchQueries: [
          { query: GET_CASES, variables: { workspaceId: currentWorkspace!.id, status: 'OPEN' } },
          { query: GET_CASES, variables: { workspaceId: currentWorkspace!.id, status: 'CLOSED' } },
        ],
      })
      refetch()
    } catch (err) {
      console.error('Failed to close case:', err)
    }
  }

  const handleReopenCase = async () => {
    if (!caseItem) return
    try {
      await reopenCase({
        variables: { workspaceId: currentWorkspace!.id, id: caseItem.id },
        refetchQueries: [
          { query: GET_CASES, variables: { workspaceId: currentWorkspace!.id, status: 'OPEN' } },
          { query: GET_CASES, variables: { workspaceId: currentWorkspace!.id, status: 'CLOSED' } },
        ],
      })
      refetch()
    } catch (err) {
      console.error('Failed to reopen case:', err)
    }
  }

  const handleActionClick = (clickedActionId: number) => {
    setSelectedActionId(clickedActionId)
    navigate(`/ws/${currentWorkspace!.id}/cases/${id}/actions/${clickedActionId}`, { replace: true })
  }

  const handleActionModalClose = () => {
    setSelectedActionId(null)
    navigate(`/ws/${currentWorkspace!.id}/cases/${id}`, { replace: true })
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

  const parseMetadata = (meta: any): Record<string, any> | null => {
    if (!meta) return null
    if (typeof meta === 'object') return meta
    if (typeof meta === 'string') {
      try { return JSON.parse(meta) } catch { return null }
    }
    return null
  }

  const renderFieldValue = (fieldId: string, value: any) => {
    const fieldDef = fieldDefs.find((f: any) => f.id === fieldId)
    if (!fieldDef) return String(value)

    switch (fieldDef.type) {
      case 'TEXT':
      case 'NUMBER':
      case 'DATE':
        return String(value || '-')

      case 'URL':
        return value ? (
          <a href={value} target="_blank" rel="noopener noreferrer" className={styles.link}>
            {value}
          </a>
        ) : (
          '-'
        )

      case 'SELECT': {
        const option = fieldDef.options?.find((opt: any) => opt.id === value)
        if (!option) return value || '-'
        const meta = parseMetadata(option.metadata)
        const metaEntries = meta ? Object.entries(meta) : []
        return (
          <>
            <span>{option.name}</span>
            {metaEntries.length > 0 && (
              <div className={styles.fieldMetaList}>
                {metaEntries.map(([k, v]) => (
                  <span key={k} className={styles.fieldMeta}>{k}: {String(v)}</span>
                ))}
              </div>
            )}
          </>
        )
      }

      case 'MULTI_SELECT': {
        const selected = (value || [])
          .map((id: string) => fieldDef.options?.find((opt: any) => opt.id === id))
          .filter(Boolean)
        if (selected.length === 0) return '-'
        return (
          <div className={styles.multiSelectList}>
            {selected.map((opt: any) => {
              const meta = parseMetadata(opt.metadata)
              const metaEntries = meta ? Object.entries(meta) : []
              return (
                <div key={opt.id} className={styles.multiSelectItem}>
                  <span>{opt.name}</span>
                  {metaEntries.length > 0 && (
                    <div className={styles.fieldMetaList}>
                      {metaEntries.map(([k, v]) => (
                        <span key={k} className={styles.fieldMeta}>{k}: {String(v)}</span>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )
      }

      case 'USER':
      case 'MULTI_USER':
        return '-'

      default:
        return String(value || '-')
    }
  }

  if (caseLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>{t('loading')}</div>
      </div>
    )
  }

  if (caseError || !caseItem) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {caseError ? `${t('errorPrefix')} ${caseError.message}` : t('errorCaseNotFound', { caseLabel })}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          {t('btnBackToList')}
        </Button>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          {t('btnBack')}
        </Button>
        <div className={styles.actions}>
          {caseItem.status === 'OPEN' ? (
            <Button variant="outline" icon={<XCircle size={20} />} onClick={() => setIsCloseDialogOpen(true)} className={styles.closeButton} data-testid="close-case-button">
              {t('btnClose')}
            </Button>
          ) : (
            <Button variant="outline" icon={<RotateCcw size={20} />} onClick={handleReopenCase} className={styles.reopenButton}>
              {t('btnReopen')}
            </Button>
          )}
          <Button variant="outline" icon={<Edit size={20} />} onClick={handleEdit}>
            {t('btnEdit')}
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
                  <span>{t('btnDelete')}</span>
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <div className={styles.titleRow}>
            <div className={styles.titleLeft}>
              <h1 className={styles.title}>{caseItem.title}</h1>
              <Chip variant="status" colorIndex={caseItem.status === 'OPEN' ? 2 : 5}>
                {caseItem.status === 'OPEN' ? t('statusOpen') : t('statusClosed')}
              </Chip>
              {caseItem.isPrivate && (
                <span className={styles.privateBadge} data-testid="private-badge">
                  <Lock size={14} />
                  {t('badgePrivate')}
                </span>
              )}
            </div>
            {caseItem.slackChannelID && (
              <a
                href={caseItem.slackChannelURL || `https://slack.com/app_redirect?channel=${caseItem.slackChannelID}`}
                target="_blank"
                rel="noopener noreferrer"
                className={styles.slackChannelLink}
              >
                #{caseItem.slackChannelName || caseItem.slackChannelID}
                <ExternalLink size={14} />
              </a>
            )}
          </div>
          {caseItem.description && (
            <div className={styles.description}>
              <Markdown>{caseItem.description}</Markdown>
            </div>
          )}
          <div className={styles.metaRow}>
            <div className={styles.timestamps}>
              <span className={styles.timestampLabel}>{t('labelCreatedTimestamp')}</span>
              <span className={styles.timestampValue} data-testid="created-timestamp-value">{new Date(caseItem.createdAt).toLocaleString()}</span>
              <span className={styles.timestampDivider} />
              <span className={styles.timestampLabel}>{t('labelUpdatedTimestamp')}</span>
              <span className={styles.timestampValue} data-testid="updated-timestamp-value">{new Date(caseItem.updatedAt).toLocaleString()}</span>
            </div>
            <button
              className={styles.assistLogLink}
              onClick={() => navigate(`/ws/${currentWorkspace!.id}/cases/${caseItem.id}/assists`)}
            >
              <Bot size={14} />
              {t('linkAssistLogs')}{assistLogTotalCount > 0 && ` (${assistLogTotalCount})`}
            </button>
          </div>
        </div>

        <div className={styles.sections}>
          {/* Assignees section */}
          {caseItem.assignees && caseItem.assignees.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>{t('sectionAssignees')}</h3>
              <div className={styles.assigneesInline}>
                {caseItem.assignees.map((user: any) => (
                  <span key={user.id} className={styles.assigneeTag}>
                    {user.imageUrl && (
                      <img src={user.imageUrl} alt={user.realName} className={styles.avatarSmall} />
                    )}
                    {user.realName || user.name}
                  </span>
                ))}
              </div>
            </div>
          )}

          {/* Fields section (custom fields) */}
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>{t('sectionFields')}</h3>
            <div className={styles.fieldsGrid}>
              {caseItem.fields.map((fieldValue) => {
                const fieldDef = fieldDefs.find((f: any) => f.id === fieldValue.fieldId)
                if (!fieldDef) return null
                return (
                  <div key={fieldValue.fieldId} className={styles.fieldItem}>
                    <div className={styles.fieldLabel}>{fieldDef.name}</div>
                    <div className={styles.fieldValue}>
                      {renderFieldValue(fieldValue.fieldId, fieldValue.value)}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          {/* Related Actions section */}
          <div className={styles.section}>
            <div className={styles.sectionHeader}>
              <h3 className={styles.sectionTitle}>{t('sectionRelatedActions')}</h3>
              {caseItem.actions && caseItem.actions.length > 0 && (
                <Button
                  variant="outline"
                  size="sm"
                  icon={<Plus size={14} />}
                  onClick={() => setIsActionFormOpen(true)}
                >
                  {t('btnAddAction')}
                </Button>
              )}
            </div>
            {caseItem.actions && caseItem.actions.length > 0 ? (
              <table className={styles.actionTable}>
                <thead>
                  <tr>
                    <th>{t('headerTitle')}</th>
                    <th>{t('headerAssignees')}</th>
                    <th>{t('headerStatus')}</th>
                    <th>{t('headerDueDate')}</th>
                    <th>{t('headerCreated')}</th>
                  </tr>
                </thead>
                <tbody>
                  {caseItem.actions.map((action) => (
                    <tr
                      key={action.id}
                      className={styles.actionRow}
                      onClick={() => handleActionClick(action.id)}
                    >
                      <td className={styles.titleCell}>{action.title}</td>
                      <td className={styles.assigneeCell}>
                        {action.assignees && action.assignees.length > 0 ? (
                          <div className={styles.actionAssignees}>
                            {action.assignees.map((user) => (
                              <span key={user.id} className={styles.actionAssigneeTag}>
                                {user.imageUrl && (
                                  <img src={user.imageUrl} alt={user.realName} className={styles.avatarSmall} />
                                )}
                                <span>{user.realName || user.name}</span>
                              </span>
                            ))}
                          </div>
                        ) : (
                          <span className={styles.noAssignee}>-</span>
                        )}
                      </td>
                      <td className={styles.statusCell}>
                        <Chip variant="status" colorIndex={STATUS_COLORS[action.status] || 0}>
                          {STATUS_LABELS[action.status] || action.status}
                        </Chip>
                      </td>
                      <td className={styles.dateCell}>
                        {action.dueDate ? new Date(action.dueDate).toLocaleDateString() : '-'}
                      </td>
                      <td className={styles.dateCell}>
                        {new Date(action.createdAt).toLocaleDateString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className={styles.emptyState}>
                <ClipboardList size={32} className={styles.emptyStateIcon} />
                <p className={styles.emptyStateTitle}>{t('emptyActionsTitle')}</p>
                <p className={styles.emptyStateDescription}>{t('emptyActionsDesc')}</p>
                <Button
                  variant="outline"
                  size="sm"
                  icon={<Plus size={14} />}
                  onClick={() => setIsActionFormOpen(true)}
                >
                  {t('btnAddAction')}
                </Button>
              </div>
            )}
          </div>

          {caseItem.knowledges && caseItem.knowledges.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>
                  <BookOpen size={16} />
                  {t('sectionRelatedKnowledge', { count: caseItem.knowledges.length })}
                </h3>
              </div>
              <table className={styles.knowledgeTable}>
                <thead>
                  <tr>
                    <th>{t('headerTitle')}</th>
                    <th>{t('headerSummary')}</th>
                    <th>{t('headerDate')}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {caseItem.knowledges
                    .slice(knowledgePage * knowledgePageSize, (knowledgePage + 1) * knowledgePageSize)
                    .map((knowledge) => (
                      <tr
                        key={knowledge.id}
                        className={styles.knowledgeRow}
                        onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledges/${knowledge.id}`)}
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
                          {knowledge.sourceURLs?.length > 0 && (
                            <a
                              href={knowledge.sourceURLs[0]}
                              target="_blank"
                              rel="noopener noreferrer"
                              className={styles.knowledgeExternalLink}
                              onClick={(e) => e.stopPropagation()}
                            >
                              <ExternalLink size={16} />
                            </a>
                          )}
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
              {caseItem.knowledges.length > knowledgePageSize && (
                <div className={styles.pagination}>
                  <button
                    className={styles.paginationButton}
                    onClick={() => setKnowledgePage((p) => Math.max(0, p - 1))}
                    disabled={knowledgePage === 0}
                  >
                    <ChevronLeft size={16} />
                  </button>
                  <span className={styles.paginationInfo}>
                    {knowledgePage + 1} / {Math.ceil(caseItem.knowledges.length / knowledgePageSize)}
                  </span>
                  <button
                    className={styles.paginationButton}
                    onClick={() =>
                      setKnowledgePage((p) =>
                        Math.min(Math.ceil(caseItem.knowledges!.length / knowledgePageSize) - 1, p + 1)
                      )
                    }
                    disabled={knowledgePage >= Math.ceil(caseItem.knowledges.length / knowledgePageSize) - 1}
                  >
                    <ChevronRight size={16} />
                  </button>
                </div>
              )}
            </div>
          )}

          {/* Channel Members section */}
          {caseItem.channelUserCount > 0 && (
            <div className={styles.section} data-testid="channel-members-section">
              <div className={styles.sectionHeader}>
                <h3 className={styles.sectionTitle}>
                  <Users size={16} />
                  {t('sectionChannelMembers', { count: caseItem.channelUserCount })}
                </h3>
                <div className={styles.memberActions}>
                  <div className={styles.memberSearchWrapper}>
                    <Search size={14} className={styles.memberSearchIcon} />
                    <input
                      type="text"
                      value={memberFilter}
                      onChange={(e) => setMemberFilter(e.target.value)}
                      placeholder={t('placeholderFilterMembers')}
                      className={styles.memberSearchInput}
                      data-testid="member-search-filter"
                    />
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    icon={<RefreshCw size={14} className={syncing ? styles.spinning : ''} />}
                    onClick={() => syncMembers()}
                    disabled={syncing}
                    data-testid="sync-members-button"
                  >
                    {t('btnSync')}
                  </Button>
                </div>
              </div>
              {memberLoading ? (
                <div className={styles.memberLoading}>{t('loadingMembers')}</div>
              ) : (
                <>
                  <div className={styles.memberGrid}>
                    {(memberData?.case?.channelUsers?.items || []).map((user: { id: string; name: string; realName: string; imageUrl?: string }) => (
                      <div key={user.id} className={styles.memberItem}>
                        {user.imageUrl ? (
                          <img src={user.imageUrl} alt={user.realName} className={styles.memberAvatar} />
                        ) : (
                          <div className={styles.memberAvatarPlaceholder}>
                            {(user.realName || user.name).charAt(0).toUpperCase()}
                          </div>
                        )}
                        <div className={styles.memberInfo}>
                          <span className={styles.memberName}>{user.realName || user.name}</span>
                          <span className={styles.memberHandle}>@{user.name}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                  {(memberData?.case?.channelUsers?.totalCount || 0) > memberPageSize && (
                    <div className={styles.pagination}>
                      <button
                        className={styles.paginationButton}
                        onClick={() => setMemberPage((p) => Math.max(0, p - 1))}
                        disabled={memberPage === 0}
                      >
                        <ChevronLeft size={16} />
                      </button>
                      <span className={styles.paginationInfo}>
                        {memberPage + 1} / {Math.ceil((memberData?.case?.channelUsers?.totalCount || 0) / memberPageSize)}
                      </span>
                      <button
                        className={styles.paginationButton}
                        onClick={() =>
                          setMemberPage((p) =>
                            Math.min(Math.ceil((memberData?.case?.channelUsers?.totalCount || 0) / memberPageSize) - 1, p + 1)
                          )
                        }
                        disabled={!memberData?.case?.channelUsers?.hasMore}
                      >
                        <ChevronRight size={16} />
                      </button>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

        </div>
      </div>

      <CaseForm isOpen={isFormOpen} onClose={() => setIsFormOpen(false)} caseItem={caseItem} />

      <CaseDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        caseTitle={caseItem.title}
      />

      <Modal
        isOpen={isCloseDialogOpen}
        onClose={() => setIsCloseDialogOpen(false)}
        title={t('titleCloseCase', { caseLabel })}
        footer={
          <>
            <Button variant="outline" onClick={() => setIsCloseDialogOpen(false)}>
              {t('btnCancel')}
            </Button>
            <Button
              variant="danger"
              data-testid="confirm-close-button"
              onClick={async () => {
                await handleCloseCase()
                setIsCloseDialogOpen(false)
              }}
            >
              {t('btnClose')}
            </Button>
          </>
        }
      >
        <p
          style={{ margin: 0, color: 'var(--text-body)' }}
          dangerouslySetInnerHTML={{ __html: t('msgCloseCaseConfirm', { title: caseItem.title }) }}
        />
      </Modal>

      {isActionFormOpen && (
        <ActionForm
          isOpen={isActionFormOpen}
          initialCaseID={caseItem.id}
          onClose={() => {
            setIsActionFormOpen(false)
            refetch()
          }}
        />
      )}

      <ActionModal
        actionId={selectedActionId}
        isOpen={selectedActionId !== null}
        onClose={handleActionModalClose}
      />
    </div>
  )
}
