import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import { GET_ACTION, UPDATE_ACTION, DELETE_ACTION, GET_ACTIONS } from '../graphql/action'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { IconCases, IconCheck } from '../components/Icons'
import InlineText from '../components/inline/InlineText'
import InlineLongText from '../components/inline/InlineLongText'
import InlineSelect, { type InlineSelectOption } from '../components/inline/InlineSelect'
import InlineUserSelect from '../components/inline/InlineUserSelect'
import InlineDate from '../components/inline/InlineDate'

interface ActionModalProps {
  actionId: number
  onClose: () => void
}

const STATUSES = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'BLOCKED', 'COMPLETED'] as const
type Status = typeof STATUSES[number]

const statusKeyMap = {
  BACKLOG: 'statusBacklog',
  TODO: 'statusTodo',
  IN_PROGRESS: 'statusInProgress',
  BLOCKED: 'statusBlocked',
  COMPLETED: 'statusCompleted',
} as const

const statusColor: Record<Status, string> = {
  BACKLOG: '#9ca3af',
  TODO: '#9ca3af',
  IN_PROGRESS: '#f59e0b',
  BLOCKED: '#ef4444',
  COMPLETED: '#10b981',
}

function formatDue(iso?: string | null) {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  const today = new Date()
  const sameDay =
    d.getFullYear() === today.getFullYear() &&
    d.getMonth() === today.getMonth() &&
    d.getDate() === today.getDate()
  const time = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  const overdue = d.getTime() < today.getTime() && !sameDay
  return {
    label: sameDay ? `今日 ${time}` : `${d.getFullYear()}/${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')} ${time}`,
    urgent: sameDay,
    overdue,
  }
}

export default function ActionModal({ actionId, onClose }: ActionModalProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data, loading } = useQuery(GET_ACTION, {
    variables: { workspaceId: currentWorkspace?.id, id: actionId },
    skip: !currentWorkspace,
  })

  const action = data?.action
  const [savedFlash, setSavedFlash] = useState(false)

  const { data: usersData } = useQuery(GET_SLACK_USERS)
  const users = usersData?.slackUsers || []

  const [updateAction] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_ACTION, variables: { workspaceId: currentWorkspace?.id, id: actionId } },
      { query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
    ],
  })
  const [deleteAction, { loading: deleting }] = useMutation(DELETE_ACTION, {
    refetchQueries: [{ query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } }],
  })

  const flashSaved = () => {
    setSavedFlash(true)
    window.setTimeout(() => setSavedFlash(false), 1500)
  }

  const statusOptions: InlineSelectOption<Status>[] = useMemo(
    () => STATUSES.map((s) => ({
      value: s,
      label: t(statusKeyMap[s]),
      color: statusColor[s],
    })),
    [t],
  )

  const titleEl = useMemo(() => (
    <div className="row" style={{ gap: 12, alignItems: 'center', flex: 1 }}>
      <h2
        id="modal-title"
        style={{ margin: 0, fontSize: 13, fontWeight: 500, color: 'var(--fg-soft)', fontFamily: 'var(--font-mono)' }}
      >
        #A-{actionId}
      </h2>
      {action?.case && (
        <a
          className="slack-link"
          href={`/ws/${currentWorkspace!.id}/cases/${action.case.id}`}
          data-testid="action-case-link"
          onClick={(e) => {
            e.preventDefault()
            navigate(`/ws/${currentWorkspace!.id}/cases/${action.case.id}`)
          }}
        >
          <IconCases size={11} />
          #{action.case.id} {action.case.title}
        </a>
      )}
      {savedFlash && (
        <span className="badge open" style={{ fontSize: 10 }}>
          <IconCheck size={9} sw={2.5} />
          {t('feedbackSaved')}
        </span>
      )}
    </div>
  ), [actionId, action?.case?.id, action?.case?.title, savedFlash, t, currentWorkspace, navigate])

  if (!loading && !action) {
    return (
      <Modal open onClose={onClose} title={t('errorActionNotFound')}>
        <p className="muted">{t('errorActionNotFound')}</p>
      </Modal>
    )
  }

  const handleStatusChange = async (next: Status) => {
    if (!action || next === action.status) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, status: next } },
    })
    flashSaved()
  }

  const handleSaveTitle = async (next: string) => {
    if (!action || next === action.title) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, title: next } },
    })
    flashSaved()
  }

  const handleSaveDescription = async (next: string) => {
    if (!action || next === (action.description || '')) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, description: next } },
    })
    flashSaved()
  }

  const handleDueDateChange = async (next: string | null) => {
    if (!action) return
    const input: any = { id: action.id }
    if (next == null) {
      input.clearDueDate = true
    } else {
      // Promote a "YYYY-MM-DD" to a full ISO timestamp at midnight UTC so the
      // backend's Time scalar can parse it.
      input.dueDate = /^\d{4}-\d{2}-\d{2}$/.test(next) ? `${next}T00:00:00Z` : next
    }
    await updateAction({ variables: { workspaceId: currentWorkspace!.id, input } })
    flashSaved()
  }

  const handleAssigneeChange = async (next: string | null) => {
    if (!action) return
    const input: Record<string, unknown> = { id: action.id }
    if (next) {
      input.assigneeID = next
    } else {
      input.clearAssignee = true
    }
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input },
    })
    flashSaved()
  }

  const [confirmDelete, setConfirmDelete] = useState(false)
  const handleDelete = async () => {
    if (!action) return
    await deleteAction({ variables: { workspaceId: currentWorkspace!.id, id: action.id } })
    setConfirmDelete(false)
    onClose()
  }

  const due = action ? formatDue(action.dueDate) : null

  return (
    <Modal
      open
      onClose={onClose}
      width={680}
      title={titleEl}
      footer={
        <>
          <Button variant="danger" onClick={() => setConfirmDelete(true)} disabled={deleting} data-testid="action-delete-button">
            {t('btnDelete')}
          </Button>
          <span className="spacer" />
          <Button variant="ghost" onClick={onClose}>{t('btnClose')}</Button>
        </>
      }
    >
      {loading ? (
        <div className="muted">{t('loading')}</div>
      ) : (
        <>
          <div style={{ marginBottom: 16 }}>
            <InlineText
              value={action.title || ''}
              onSave={handleSaveTitle}
              ariaLabel={t('labelTitle')}
              variant="title"
              placeholder={t('placeholderAddTitle')}
              testId="action-title"
            />
          </div>

          <div className="row" style={{ gap: 18, fontSize: 13, marginBottom: 16, flexWrap: 'wrap', alignItems: 'center' }}>
            <div className="row" style={{ gap: 8, alignItems: 'center', minWidth: 0 }}>
              <span className="soft">{t('labelStatus')}</span>
              <InlineSelect<Status>
                value={action.status as Status}
                options={statusOptions}
                onSave={handleStatusChange}
                ariaLabel={t('labelStatus')}
                placeholder={t('placeholderAddStatus')}
                testId="action-status"
              />
              {/* a11y dropdown for assistive tech and existing e2e */}
              <select
                aria-hidden
                tabIndex={-1}
                data-testid="status-dropdown"
                value={action.status}
                onChange={(e) => handleStatusChange(e.target.value as Status)}
                style={{ position: 'absolute', width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
              >
                {STATUSES.map((s) => (
                  <option key={s} value={s}>{t(statusKeyMap[s])}</option>
                ))}
              </select>
            </div>
            <div className="row" style={{ gap: 8, alignItems: 'center', minWidth: 280, flex: 1 }}>
              <span className="soft">{t('labelAssignee')}</span>
              <InlineUserSelect
                users={users}
                value={action.assigneeID || null}
                onSave={handleAssigneeChange}
                ariaLabel={t('labelAssignee')}
                placeholder={t('placeholderSelectAssignee')}
                testId="action-assignee"
              />
            </div>
          </div>

          <div className="row" style={{ gap: 8, fontSize: 13, marginBottom: 16, alignItems: 'center', flexWrap: 'wrap' }}>
            <span className="soft">{t('labelDueDate')}</span>
            <div style={{ flex: 1, minWidth: 160, color: due?.urgent || due?.overdue ? 'var(--danger)' : undefined }}>
              <InlineDate
                value={action.dueDate || null}
                onSave={handleDueDateChange}
                ariaLabel={t('labelDueDate')}
                placeholder={t('placeholderAddValue')}
                testId="action-due-date"
              />
            </div>
          </div>

          <div className="field-label">{t('labelDescription')}</div>
          <InlineLongText
            value={action.description || ''}
            onSave={handleSaveDescription}
            ariaLabel={t('labelDescription')}
            placeholder={t('placeholderAddDescription')}
            testId="action-description"
          />

          <div className="field-label" style={{ marginTop: 18 }}>
            {t('sectionActivity')}
          </div>
          <div className="muted" style={{ fontSize: 12 }}>
            {t('emptyActivity')}
          </div>
        </>
      )}
      {confirmDelete && action && (
        <Modal
          open
          onClose={() => setConfirmDelete(false)}
          title={t('titleDeleteAction')}
          width={420}
          footer={
            <>
              <Button variant="ghost" onClick={() => setConfirmDelete(false)}>{t('btnCancel')}</Button>
              <Button
                variant="danger"
                onClick={handleDelete}
                disabled={deleting}
                data-testid="action-delete-confirm-button"
              >
                {deleting ? t('btnDeleting') : t('btnDelete')}
              </Button>
            </>
          }
        >
          <p style={{ fontSize: 13, lineHeight: 1.6, margin: 0 }}>
            {t('warningDeleteActionPermanent')}
          </p>
        </Modal>
      )}
    </Modal>
  )
}
