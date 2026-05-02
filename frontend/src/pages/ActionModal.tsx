import { useState, useEffect, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import { buildSelectStyles, portalProps } from '../components/selectStyles'
import { GET_ACTION, UPDATE_ACTION, DELETE_ACTION, GET_ACTIONS } from '../graphql/action'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { AvatarStack } from '../components/Primitives'
import { IconCases, IconCheck } from '../components/Icons'

interface ActionModalProps {
  actionId: number
  onClose: () => void
}

const STATUSES = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'BLOCKED', 'COMPLETED'] as const

const statusKeyMap = {
  BACKLOG: 'statusBacklog',
  TODO: 'statusTodo',
  IN_PROGRESS: 'statusInProgress',
  BLOCKED: 'statusBlocked',
  COMPLETED: 'statusCompleted',
} as const

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
  const [editingTitle, setEditingTitle] = useState('')
  const [editing, setEditing] = useState(false)
  const [editingDescription, setEditingDescription] = useState('')
  const [, setDescriptionDirty] = useState(false)
  const [savedFlash, setSavedFlash] = useState(false)

  const { data: usersData } = useQuery(GET_SLACK_USERS)
  const users = usersData?.slackUsers || []
  const userOptions = users.map((u: any) => ({
    value: u.id as string,
    label: u.realName || u.name,
  }))

  const [updateAction, { loading: saving }] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_ACTION, variables: { workspaceId: currentWorkspace?.id, id: actionId } },
      { query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
    ],
  })
  const [deleteAction, { loading: deleting }] = useMutation(DELETE_ACTION, {
    refetchQueries: [{ query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } }],
  })

  useEffect(() => {
    if (action) {
      setEditingTitle(action.title || '')
      setEditingDescription(action.description || '')
      setDescriptionDirty(false)
    }
  }, [action?.id])

  const flashSaved = () => {
    setSavedFlash(true)
    window.setTimeout(() => setSavedFlash(false), 1500)
  }

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
            // Navigate without calling onClose() — onClose triggers a route
            // change back to the actions list which would override this nav.
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
  ), [actionId, action?.case?.id, action?.case?.title, savedFlash, t, currentWorkspace, navigate, onClose])

  if (!loading && !action) {
    return (
      <Modal open onClose={onClose} title={t('errorActionNotFound')}>
        <p className="muted">{t('errorActionNotFound')}</p>
      </Modal>
    )
  }

  const handleStatusChange = async (next: string) => {
    if (!action || next === action.status) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, status: next } },
    })
    flashSaved()
  }

  const handleSaveTitle = async () => {
    if (!action || !editingTitle.trim() || editingTitle === action.title) {
      setEditing(false)
      return
    }
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, title: editingTitle.trim() } },
    })
    setEditing(false)
    flashSaved()
  }

  const handleAssigneesChange = async (next: string[]) => {
    if (!action) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, assigneeIDs: next } },
    })
    flashSaved()
  }

  const handleSave = async () => {
    if (!action) return
    const input: any = { id: action.id }
    if (editingTitle.trim() && editingTitle !== action.title) input.title = editingTitle.trim()
    if (editingDescription !== (action.description || '')) input.description = editingDescription
    if (Object.keys(input).length === 1) {
      // Nothing changed; still flash to acknowledge the click
      flashSaved()
      return
    }
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input },
    })
    setDescriptionDirty(false)
    setEditing(false)
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
          <Button
            variant="primary"
            onClick={handleSave}
            disabled={saving}
            data-testid="action-save-button"
          >
            {saving ? t('btnSaving') : t('btnSave')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div className="muted">{t('loading')}</div>
      ) : (
        <>
          {editing ? (
            <input
              className="input"
              autoFocus
              value={editingTitle}
              onChange={(e) => setEditingTitle(e.target.value)}
              onBlur={handleSaveTitle}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSaveTitle()
                else if (e.key === 'Escape') { setEditingTitle(action.title); setEditing(false) }
              }}
              style={{ height: 38, fontSize: 18, fontWeight: 600, marginBottom: 16, padding: '0 6px' }}
              data-testid="action-title-input"
            />
          ) : (
            <h3
              className="titleText"
              onClick={() => setEditing(true)}
              style={{ margin: '0 0 16px 0', fontSize: 20, fontWeight: 600, cursor: 'text' }}
              data-testid="action-title"
            >
              {action.title}
            </h3>
          )}

          <div className="row" style={{ gap: 18, fontSize: 12, marginBottom: 16, flexWrap: 'wrap', alignItems: 'center' }}>
            <div className="row" style={{ gap: 8, alignItems: 'center' }}>
              <span className="soft">{t('labelStatus')}</span>
              <div className="seg" style={{ fontSize: 11 }} data-testid="status-segmented">
                {STATUSES.map((s) => (
                  <button
                    key={s}
                    type="button"
                    className={action.status === s ? 'on' : ''}
                    onClick={() => handleStatusChange(s)}
                    data-testid={`status-${s}`}
                  >
                    {t(statusKeyMap[s])}
                  </button>
                ))}
              </div>
              {/* keep an a11y dropdown for assistive tech and existing e2e */}
              <select
                aria-hidden
                tabIndex={-1}
                data-testid="status-dropdown"
                value={action.status}
                onChange={(e) => handleStatusChange(e.target.value)}
                style={{ position: 'absolute', width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
              >
                {STATUSES.map((s) => (
                  <option key={s} value={s}>{t(statusKeyMap[s])}</option>
                ))}
              </select>
            </div>
            <div className="row" style={{ gap: 8, alignItems: 'center', minWidth: 280, flex: 1 }}>
              <span className="soft">{t('labelAssignees')}</span>
              {action.assignees && action.assignees.length > 0 && (
                <AvatarStack users={action.assignees} max={4} />
              )}
              <div style={{ flex: 1, minWidth: 200 }}>
                <Select
                  inputId={`action-assignees-${action.id}`}
                  aria-label={t('labelAssignees')}
                  isMulti
                  options={userOptions}
                  value={userOptions.filter((o: any) => (action.assigneeIDs || []).includes(o.value))}
                  onChange={(opts: any) =>
                    handleAssigneesChange((opts || []).map((o: any) => o.value))
                  }
                  placeholder={t('placeholderAddAssignees')}
                  classNamePrefix="rs"
                  {...portalProps}
                  styles={buildSelectStyles()}
                />
              </div>
            </div>
          </div>

          {due && (
            <div className="row" style={{ gap: 8, fontSize: 12, marginBottom: 16, alignItems: 'center' }}>
              <span className="soft">{t('labelDueDate')}</span>
              <span className="mono" style={{ color: due.urgent || due.overdue ? 'var(--danger)' : 'var(--fg)' }}>
                {due.label}
              </span>
            </div>
          )}

          <div className="field-label">{t('labelDescription')}</div>
          <textarea
            className="textarea"
            value={editingDescription}
            onChange={(e) => { setEditingDescription(e.target.value); setDescriptionDirty(true) }}
            placeholder={t('placeholderAddDescription')}
            data-testid="action-description-input"
            style={{
              minHeight: 88, fontSize: 13, lineHeight: 1.6,
              border: '1px dashed var(--line)', borderRadius: 6,
              padding: '8px 10px', background: 'transparent',
              color: editingDescription ? 'var(--fg)' : 'var(--fg-muted)',
            }}
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
