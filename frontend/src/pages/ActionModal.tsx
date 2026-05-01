import { useState, useMemo, useEffect } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import { GET_ACTION, UPDATE_ACTION, DELETE_ACTION, GET_ACTIONS } from '../graphql/action'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { Avatar } from '../components/Primitives'
import { IconExt } from '../components/Icons'

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

export default function ActionModal({ actionId, onClose }: ActionModalProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const { data, loading } = useQuery(GET_ACTION, {
    variables: { workspaceId: currentWorkspace?.id, id: actionId },
    skip: !currentWorkspace,
  })

  const action = data?.action
  const [editingTitle, setEditingTitle] = useState('')
  const [editing, setEditing] = useState(false)

  const [updateAction] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_ACTION, variables: { workspaceId: currentWorkspace?.id, id: actionId } },
      { query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
    ],
  })
  const [deleteAction, { loading: deleting }] = useMutation(DELETE_ACTION, {
    refetchQueries: [{ query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } }],
  })

  useEffect(() => { if (action) setEditingTitle(action.title) }, [action?.id])

  const titleEl = useMemo(() => (
    <h2 id="modal-title" style={{ margin: 0, fontSize: 13, fontWeight: 500, color: 'var(--fg-soft)', fontFamily: 'var(--font-mono)' }}>
      #{actionId}
    </h2>
  ), [actionId])

  if (!loading && !action) {
    return (
      <Modal open onClose={onClose} title={t('errorActionNotFound')}>
        <p className="muted">{t('errorActionNotFound')}</p>
      </Modal>
    )
  }

  const handleStatusChange = async (next: string) => {
    if (!action) return
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, status: next } },
    })
  }

  const handleSaveTitle = async () => {
    if (!action || !editingTitle.trim()) { setEditing(false); return }
    await updateAction({
      variables: { workspaceId: currentWorkspace!.id, input: { id: action.id, title: editingTitle.trim() } },
    })
    setEditing(false)
  }

  const handleDelete = async () => {
    if (!action) return
    if (!window.confirm(t('warningDeleteActionPermanent'))) return
    await deleteAction({ variables: { workspaceId: currentWorkspace!.id, id: action.id } })
    onClose()
  }

  return (
    <Modal
      open
      onClose={onClose}
      width={620}
      title={titleEl}
      footer={
        <>
          <Button variant="danger" onClick={handleDelete} disabled={deleting}>{t('btnDelete')}</Button>
          <span className="spacer" />
          <Button variant="ghost" onClick={onClose}>{t('btnClose')}</Button>
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
              style={{ height: 38, fontSize: 16, fontWeight: 600, marginBottom: 12 }}
            />
          ) : (
            <h3
              className="titleText"
              onClick={() => setEditing(true)}
              style={{ margin: '0 0 12px 0', fontSize: 18, fontWeight: 600, cursor: 'text' }}
            >
              {action.title}
            </h3>
          )}

          <div className="row" style={{ gap: 14, fontSize: 12, marginBottom: 16, flexWrap: 'wrap' }}>
            <div className="row" style={{ gap: 6 }}>
              <span className="soft">{t('labelStatus')}</span>
              <select
                data-testid="status-dropdown"
                className="select"
                value={action.status}
                onChange={(e) => handleStatusChange(e.target.value)}
                style={{ width: 'auto', height: 26 }}
              >
                {STATUSES.map((s) => (
                  <option key={s} value={s}>{t(statusKeyMap[s])}</option>
                ))}
              </select>
            </div>
            {action.assignees?.[0] && (
              <div className="row" style={{ gap: 6 }}>
                <span className="soft">{t('labelAssignees')}</span>
                <Avatar size="sm" name={action.assignees[0].name} realName={action.assignees[0].realName} imageUrl={action.assignees[0].imageUrl} />
                <span>{action.assignees[0].realName}</span>
              </div>
            )}
            {action.case && (
              <a
                className="slack-link"
                href={`/ws/${currentWorkspace!.id}/cases/${action.case.id}`}
              >
                #{action.case.id} {action.case.title}
                <IconExt size={10} />
              </a>
            )}
          </div>
          <div className="field-label">{t('labelDescription')}</div>
          <p style={{ fontSize: 13, lineHeight: 1.6, margin: 0, color: action.description ? undefined : 'var(--fg-muted)' }}>
            {action.description || t('labelNoDescription')}
          </p>
        </>
      )}
    </Modal>
  )
}
