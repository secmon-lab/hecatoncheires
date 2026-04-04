import { useState, useEffect, useCallback, useRef } from 'react'
import { useQuery, useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import Select from 'react-select'
import Markdown from 'react-markdown'
import { Trash2, AlertTriangle, Check, Pencil } from 'lucide-react'
import Modal from '../components/Modal'
import Button from '../components/Button'
import Chip from '../components/Chip'
import { GET_ACTION, UPDATE_ACTION, DELETE_ACTION, GET_OPEN_CASE_ACTIONS } from '../graphql/action'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './ActionModal.module.css'

interface ActionModalProps {
  actionId: number | null
  isOpen: boolean
  onClose: () => void
}

interface AssigneeOption {
  value: string
  label: string
  name: string
  realName: string
  image?: string
}

const STATUS_COLORS: Record<string, number> = {
  BACKLOG: 0,
  TODO: 1,
  IN_PROGRESS: 2,
  BLOCKED: 3,
  COMPLETED: 4,
}

// STATUS_OPTIONS is defined inside the component to support i18n

function useFeedback() {
  const [visible, setVisible] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const show = useCallback(() => {
    setVisible(true)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setVisible(false), 2000)
  }, [])

  useEffect(() => {
    return () => { if (timerRef.current) clearTimeout(timerRef.current) }
  }, [])

  return { visible, show }
}

export default function ActionModal({ actionId, isOpen, onClose }: ActionModalProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const STATUS_OPTIONS = [
    { value: 'BACKLOG', label: t('statusBacklog') },
    { value: 'TODO', label: t('statusTodo') },
    { value: 'IN_PROGRESS', label: t('statusInProgress') },
    { value: 'BLOCKED', label: t('statusBlocked') },
    { value: 'COMPLETED', label: t('statusCompleted') },
  ]
  const [isDeleteConfirm, setIsDeleteConfirm] = useState(false)

  // Title inline edit
  const [isEditingTitle, setIsEditingTitle] = useState(false)
  const [editTitle, setEditTitle] = useState('')
  const [savingTitle, setSavingTitle] = useState(false)
  const titleFeedback = useFeedback()

  // Description
  const [isEditingDescription, setIsEditingDescription] = useState(false)
  const [editDescription, setEditDescription] = useState('')
  const [savingDescription, setSavingDescription] = useState(false)
  const descFeedback = useFeedback()

  // Sidebar auto-save feedback
  const statusFeedback = useFeedback()
  const assigneeFeedback = useFeedback()

  const { data: actionData, loading, error } = useQuery(GET_ACTION, {
    variables: { workspaceId: currentWorkspace!.id, id: actionId },
    skip: !actionId || !currentWorkspace,
  })

  const { data: usersData } = useQuery(GET_SLACK_USERS)

  const [updateAction] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_ACTION, variables: { workspaceId: currentWorkspace!.id, id: actionId } },
      { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace!.id } },
    ],
    onError: (err) => {
      console.error('Update error:', err)
    },
  })

  const [deleteAction, { loading: deleting }] = useMutation(DELETE_ACTION, {
    refetchQueries: [
      { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace!.id } },
    ],
    onCompleted: () => {
      setIsDeleteConfirm(false)
      onClose()
    },
    onError: (err) => {
      console.error('Delete error:', err)
    },
  })

  const action = actionData?.action

  // Sync local description from server data
  useEffect(() => {
    if (action) {
      setEditDescription(action.description || '')
    }
  }, [action])

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      setIsDeleteConfirm(false)
      setIsEditingTitle(false)
      setIsEditingDescription(false)
    }
  }, [isOpen])

  // --- Handlers ---

  const handleTitleEditStart = () => {
    if (!action) return
    setEditTitle(action.title)
    setIsEditingTitle(true)
  }

  const handleTitleSave = async () => {
    if (!action || !editTitle.trim()) return
    setSavingTitle(true)
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: action.id, title: editTitle.trim() },
      },
    })
    setSavingTitle(false)
    setIsEditingTitle(false)
    titleFeedback.show()
  }

  const handleTitleCancel = () => {
    setIsEditingTitle(false)
  }

  const handleDescriptionEditStart = () => {
    if (!action) return
    setEditDescription(action.description || '')
    setIsEditingDescription(true)
  }

  const handleDescriptionSave = async () => {
    if (!action) return
    setSavingDescription(true)
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: action.id, description: editDescription.trim() },
      },
    })
    setSavingDescription(false)
    setIsEditingDescription(false)
    descFeedback.show()
  }

  const handleDescriptionCancel = () => {
    setIsEditingDescription(false)
    setEditDescription(action?.description || '')
  }

  const handleStatusChange = async (newStatus: string) => {
    if (!action || newStatus === action.status) return
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: action.id, status: newStatus },
      },
    })
    statusFeedback.show()
  }

  const handleAssigneeChange = async (newAssigneeIDs: string[]) => {
    if (!action) return
    await updateAction({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: action.id, assigneeIDs: newAssigneeIDs },
      },
    })
    assigneeFeedback.show()
  }

  const handleDelete = async () => {
    if (!action) return
    await deleteAction({
      variables: { workspaceId: currentWorkspace!.id, id: action.id },
    })
  }

  const handleCaseClick = () => {
    if (action?.case) {
      onClose()
      navigate(`/ws/${currentWorkspace!.id}/cases/${action.case.id}`)
    }
  }

  const handleClose = () => {
    setIsDeleteConfirm(false)
    setIsEditingTitle(false)
    setIsEditingDescription(false)
    onClose()
  }

  if (!isOpen) return null

  // Delete confirmation
  if (isDeleteConfirm) {
    return (
      <Modal
        isOpen={true}
        onClose={() => setIsDeleteConfirm(false)}
        title={t('titleDeleteAction')}
        footer={
          <>
            <Button variant="outline" onClick={() => setIsDeleteConfirm(false)} disabled={deleting}>
              {t('btnCancel')}
            </Button>
            <Button variant="danger" onClick={handleDelete} disabled={deleting}>
              {deleting ? t('btnDeleting') : t('btnDelete')}
            </Button>
          </>
        }
      >
        <div className={styles.deleteContent}>
          <AlertTriangle size={48} className={styles.deleteIcon} />
          <p className={styles.deleteMessage} dangerouslySetInnerHTML={{ __html: t('msgDeleteActionConfirm', { title: action?.title || '' }) }} />
          <p className={styles.deleteWarning}>{t('warningDeleteActionPermanent')}</p>
        </div>
      </Modal>
    )
  }

  // Loading / Error
  if (loading) {
    return (
      <Modal isOpen={true} onClose={handleClose} title={t('titleAction')}>
        <div className={styles.loading}>{t('loading')}</div>
      </Modal>
    )
  }

  if (error || !action) {
    return (
      <Modal
        isOpen={true}
        onClose={handleClose}
        title={t('titleAction')}
        footer={
          <Button variant="outline" onClick={handleClose}>{t('btnClose')}</Button>
        }
      >
        <div className={styles.error}>
          {error ? `${t('errorPrefix')} ${error.message}` : t('errorActionNotFound')}
        </div>
      </Modal>
    )
  }

  // Assignee options for Select
  const assigneeOptions: AssigneeOption[] = (usersData?.slackUsers || []).map(
    (user: { id: string; name: string; realName: string; imageUrl?: string }) => ({
      value: user.id,
      label: user.realName || user.name,
      name: user.name,
      realName: user.realName,
      image: user.imageUrl,
    })
  )

  const selectedAssignees = assigneeOptions.filter((opt) =>
    (action.assigneeIDs || []).includes(opt.value)
  )

  // Unified view
  return (
    <Modal
      isOpen={true}
      onClose={handleClose}
      title={t('titleAction')}
    >
      <div className={styles.body}>
        {/* Case link */}
        {action.case && (
          <span className={styles.caseLink} onClick={handleCaseClick}>
            {t('caseLinkLabel', { id: String(action.caseID), title: action.case.title })}
          </span>
        )}

        {/* Title section */}
        <div className={styles.titleSection}>
          {isEditingTitle ? (
            <div className={styles.titleEditRow}>
              <input
                type="text"
                value={editTitle}
                onChange={(e) => setEditTitle(e.target.value)}
                className={styles.titleInput}
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleTitleSave()
                  if (e.key === 'Escape') handleTitleCancel()
                }}
                disabled={savingTitle}
              />
              <Button variant="primary" onClick={handleTitleSave} disabled={savingTitle || !editTitle.trim()}>
                {savingTitle ? t('btnSaving') : t('btnSave')}
              </Button>
              <Button variant="outline" onClick={handleTitleCancel} disabled={savingTitle}>
                {t('btnCancel')}
              </Button>
            </div>
          ) : (
            <div className={styles.titleDisplay} onClick={handleTitleEditStart}>
              <h2 className={styles.titleText}>{action.title}</h2>
              <Pencil size={14} className={styles.titleEditIcon} />
              {titleFeedback.visible && (
                <span className={styles.feedbackInline}>
                  <Check size={14} /> {t('feedbackUpdated')}
                </span>
              )}
            </div>
          )}
        </div>

        {/* Two-column layout */}
        <div className={styles.columns}>
          {/* Main: Description */}
          <div className={styles.mainColumn}>
            <label className={styles.fieldLabel}>{t('labelDescription')}</label>
            {isEditingDescription ? (
              <>
                <textarea
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                  className={styles.descriptionTextarea}
                  placeholder={t('placeholderAddDescription')}
                  rows={10}
                  autoFocus
                />
                <div className={styles.descriptionActions}>
                  <Button variant="outline" icon={<Trash2 size={16} />} onClick={() => setIsDeleteConfirm(true)}>
                    {t('btnDelete')}
                  </Button>
                  <div className={styles.descriptionActionsRight}>
                    {descFeedback.visible && (
                      <span className={styles.feedbackInline}>
                        <Check size={14} /> {t('feedbackSaved')}
                      </span>
                    )}
                    <Button variant="outline" onClick={handleDescriptionCancel} disabled={savingDescription}>
                      {t('btnCancel')}
                    </Button>
                    <Button
                      variant="primary"
                      onClick={handleDescriptionSave}
                      disabled={savingDescription}
                    >
                      {savingDescription ? t('btnSaving') : t('btnSave')}
                    </Button>
                  </div>
                </div>
              </>
            ) : (
              <>
                <div className={styles.descriptionDisplay}>
                  {action.description ? (
                    <div className={styles.descriptionMarkdown}>
                      <Markdown>{action.description}</Markdown>
                    </div>
                  ) : (
                    <p className={styles.descriptionPlaceholder}>{t('labelNoDescription')}</p>
                  )}
                </div>
                <div className={styles.descriptionActions}>
                  <Button variant="outline" icon={<Trash2 size={16} />} onClick={() => setIsDeleteConfirm(true)}>
                    {t('btnDelete')}
                  </Button>
                  <div className={styles.descriptionActionsRight}>
                    {descFeedback.visible && (
                      <span className={styles.feedbackInline}>
                        <Check size={14} /> {t('feedbackSaved')}
                      </span>
                    )}
                    <Button
                      variant="primary"
                      icon={<Pencil size={14} />}
                      onClick={handleDescriptionEditStart}
                    >
                      {t('btnEdit')}
                    </Button>
                  </div>
                </div>
              </>
            )}
          </div>

          {/* Sidebar: Status, Assignees, Meta */}
          <div className={styles.sidebar}>
            {/* Status */}
            <div className={styles.sidebarSection}>
              <label className={styles.fieldLabel}>{t('labelStatus')}</label>
              {/* Hidden native select for E2E testing */}
              <select
                value={action.status}
                onChange={(e) => handleStatusChange(e.target.value)}
                className={styles.hiddenSelect}
                data-testid="status-dropdown"
              >
                {STATUS_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              <Select
                value={STATUS_OPTIONS.find((opt) => opt.value === action.status)}
                onChange={(selected) => {
                  if (selected) handleStatusChange(selected.value)
                }}
                options={STATUS_OPTIONS}
                isSearchable={false}
                classNamePrefix="status-select"
                styles={{
                  control: (base) => ({ ...base, minHeight: '2rem', fontSize: '0.8125rem' }),
                  valueContainer: (base) => ({ ...base, padding: '0 0.5rem' }),
                  indicatorsContainer: (base) => ({ ...base, height: '2rem' }),
                }}
                formatOptionLabel={(option) => (
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                    <Chip variant="status" colorIndex={STATUS_COLORS[option.value] || 0}>
                      {option.label}
                    </Chip>
                  </div>
                )}
              />
              {statusFeedback.visible && (
                <span className={styles.feedback}>
                  <Check size={12} /> {t('feedbackUpdated')}
                </span>
              )}
            </div>

            {/* Assignees */}
            <div className={styles.sidebarSection}>
              <label className={styles.fieldLabel}>{t('labelAssignees')}</label>
              <Select<AssigneeOption, true>
                isMulti
                isClearable={false}
                value={selectedAssignees}
                onChange={(selected) => {
                  const ids = [...(selected || [])].map((s) => s.value)
                  handleAssigneeChange(ids)
                }}
                options={assigneeOptions}
                placeholder={t('placeholderAddAssignees')}
                classNamePrefix="assignee-select"
                styles={{
                  control: (base) => ({ ...base, minHeight: '2.25rem', fontSize: '0.8125rem', alignItems: 'center' }),
                  valueContainer: (base) => ({ ...base, padding: '0.25rem 0.5rem' }),
                  indicatorsContainer: (base) => ({ ...base, minHeight: '2.25rem' }),
                  multiValue: (base) => ({ ...base, margin: '2px 2px' }),
                }}
                filterOption={(option, inputValue) => {
                  const search = inputValue.toLowerCase()
                  const data = option.data
                  return (
                    data.label.toLowerCase().includes(search) ||
                    data.name.toLowerCase().includes(search) ||
                    data.realName.toLowerCase().includes(search)
                  )
                }}
                formatOptionLabel={(option) => (
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                    {option.image && (
                      <img
                        src={option.image}
                        alt={option.label}
                        style={{ width: '1.125rem', height: '1.125rem', borderRadius: '50%' }}
                      />
                    )}
                    <span>{option.label}</span>
                  </div>
                )}
              />
              {assigneeFeedback.visible && (
                <span className={styles.feedback}>
                  <Check size={12} /> {t('feedbackUpdated')}
                </span>
              )}
            </div>

            {/* Due Date */}
            <div className={styles.sidebarSection}>
              <label className={styles.fieldLabel}>{t('labelDueDate')}</label>
              <input
                type="date"
                value={action.dueDate ? new Date(action.dueDate).toISOString().split('T')[0] : ''}
                onChange={async (e) => {
                  const val = e.target.value
                  await updateAction({
                    variables: {
                      workspaceId: currentWorkspace!.id,
                      input: {
                        id: action.id,
                        ...(val ? { dueDate: new Date(val).toISOString() } : { clearDueDate: true }),
                      },
                    },
                  })
                }}
                className={styles.dueDateInput}
              />
            </div>

            {/* Metadata */}
            <div className={styles.sidebarMeta}>
              <div className={styles.metaItem}>
                <label className={styles.fieldLabel}>{t('labelCreated')}</label>
                <span className={styles.metaValue}>
                  {new Date(action.createdAt).toLocaleString()}
                </span>
              </div>
              <div className={styles.metaItem}>
                <label className={styles.fieldLabel}>{t('labelUpdated')}</label>
                <span className={styles.metaValue}>
                  {new Date(action.updatedAt).toLocaleString()}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  )
}
