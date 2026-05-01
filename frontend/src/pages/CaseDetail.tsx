import { useState, useMemo } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import {
  GET_CASE,
  GET_CASE_MEMBERS,
  CLOSE_CASE,
  REOPEN_CASE,
  DELETE_CASE,
  SYNC_CASE_CHANNEL_USERS,
  GET_CASES,
} from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import Modal from '../components/Modal'
import {
  IconChevLeft,
  IconLock,
  IconCheck,
  IconRefresh,
  IconSearch,
  IconPlus,
  IconCalendar,
  IconSlack,
  IconExt,
} from '../components/Icons'
import { Avatar, Badge, PrivateBadge, StatusBadge } from '../components/Primitives'
import CaseForm from './CaseForm'
import CaseDeleteDialog from './CaseDeleteDialog'
import styles from './CaseDetail.module.css'

interface User {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

function formatTimestamp(iso?: string | null) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

export default function CaseDetail() {
  const { id } = useParams<{ id: string }>()
  const caseId = Number(id)
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const [editing, setEditing] = useState(false)
  const [confirmClose, setConfirmClose] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [memberFilter, setMemberFilter] = useState('')

  const { data, loading, error } = useQuery(GET_CASE, {
    variables: { workspaceId: currentWorkspace?.id, id: caseId },
    skip: !currentWorkspace || Number.isNaN(caseId),
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const c = data?.case
  const isPrivate = !!c?.isPrivate
  const slackChannelID: string = c?.slackChannelID || ''
  const slackChannelName: string = c?.slackChannelName || ''
  const slackChannelURL: string | null = c?.slackChannelURL || null
  const channelUserCount: number = c?.channelUserCount || 0

  const { data: membersData } = useQuery(GET_CASE_MEMBERS, {
    variables: {
      workspaceId: currentWorkspace?.id,
      id: caseId,
      limit: 200,
      offset: 0,
      filter: memberFilter || undefined,
    },
    skip: !currentWorkspace || Number.isNaN(caseId) || !isPrivate || !slackChannelID,
  })

  const refetchOptions = useMemo(
    () => [
      { query: GET_CASE, variables: { workspaceId: currentWorkspace?.id, id: caseId } },
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' } },
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'CLOSED' } },
    ],
    [currentWorkspace?.id, caseId],
  )

  const [closeCase, { loading: closing }] = useMutation(CLOSE_CASE, { refetchQueries: refetchOptions })
  const [reopenCase, { loading: reopening }] = useMutation(REOPEN_CASE, { refetchQueries: refetchOptions })
  const [deleteCase, { loading: deleting }] = useMutation(DELETE_CASE, {
    refetchQueries: [
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' } },
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'CLOSED' } },
    ],
  })
  const [syncMembers, { loading: syncing }] = useMutation(SYNC_CASE_CHANNEL_USERS, {
    refetchQueries: [{
      query: GET_CASE_MEMBERS,
      variables: { workspaceId: currentWorkspace?.id, id: caseId, limit: 200, offset: 0, filter: memberFilter || undefined },
    }],
  })

  if (loading) {
    return <div className="h-main-inner muted">{t('loading')}</div>
  }
  if (error || !c) {
    return (
      <div className="h-main-inner">
        <div className="card" style={{ padding: 24, color: 'var(--danger)' }}>
          {t('errorCaseNotFound', { caseLabel: configData?.fieldConfiguration?.labels?.case || 'Case' })}
        </div>
      </div>
    )
  }

  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'
  const fields = configData?.fieldConfiguration?.fields || []

  const handleClose = async () => {
    await closeCase({ variables: { workspaceId: currentWorkspace!.id, id: caseId } })
    setConfirmClose(false)
  }
  const handleReopen = async () => {
    await reopenCase({ variables: { workspaceId: currentWorkspace!.id, id: caseId } })
  }
  const handleDelete = async () => {
    await deleteCase({ variables: { workspaceId: currentWorkspace!.id, id: caseId } })
    setConfirmDelete(false)
    navigate(`/ws/${currentWorkspace!.id}/cases`)
  }
  const handleSync = async () => {
    await syncMembers({ variables: { workspaceId: currentWorkspace!.id, id: caseId } })
  }

  const renderFieldValue = (fieldId: string) => {
    const fv = c.fields?.find((f: any) => f.fieldId === fieldId)
    if (!fv) return <span className="soft">{t('emptyValue')}</span>
    const def = fields.find((f: any) => f.id === fieldId)
    if (!def) return <span>{String(fv.value)}</span>
    switch (def.type) {
      case 'TEXT':
      case 'NUMBER':
      case 'DATE':
        return <span>{String(fv.value ?? '—')}</span>
      case 'URL':
        return (
          <a href={String(fv.value)} target="_blank" rel="noreferrer noopener" style={{ color: 'var(--accent)' }}>
            {String(fv.value)}<IconExt size={10} style={{ verticalAlign: -1, marginLeft: 4 }} />
          </a>
        )
      case 'SELECT': {
        const opt = def.options?.find((o: any) => o.id === fv.value)
        return <Badge>{opt ? opt.name : String(fv.value)}</Badge>
      }
      case 'MULTI_SELECT': {
        const names = (fv.value || []).map((vid: string) => def.options?.find((o: any) => o.id === vid)?.name).filter(Boolean)
        return (
          <div className="row" style={{ gap: 4, flexWrap: 'wrap' }}>
            {names.length === 0 ? <span className="soft">—</span> : names.map((n: string) => <span key={n} className="chip" style={{ height: 20, fontSize: 11 }}>{n}</span>)}
          </div>
        )
      }
      default:
        return <span>{String(fv.value ?? '—')}</span>
    }
  }

  const members: User[] = membersData?.case?.channelUsers?.items || []
  const memberTotal: number = membersData?.case?.channelUserCount ?? channelUserCount

  return (
    <div className="h-main-inner" style={{ maxWidth: 1100 }}>
      <div className="row" style={{ marginBottom: 12 }}>
        <Button
          variant="ghost"
          size="sm"
          icon={<IconChevLeft size={13} />}
          onClick={() => navigate(`/ws/${currentWorkspace!.id}/cases`)}
        >
          {t('btnBack')}
        </Button>
        <span className="spacer" />
        {c.status === 'OPEN' ? (
          <Button
            size="sm"
            icon={<IconCheck size={13} />}
            onClick={() => setConfirmClose(true)}
            disabled={closing}
            data-testid="close-case-button"
          >
            {t('btnClose')}
          </Button>
        ) : (
          <Button
            size="sm"
            icon={<IconRefresh size={13} />}
            onClick={handleReopen}
            disabled={reopening}
          >
            {t('btnReopen')}
          </Button>
        )}
        <Button size="sm" onClick={() => setEditing(true)}>{t('btnEdit')}</Button>
        <Button size="sm" variant="danger" onClick={() => setConfirmDelete(true)}>
          {t('btnDelete')}
        </Button>
      </div>

      <div className="card" style={{ padding: 24 }}>
        <div className="row" style={{ alignItems: 'flex-start', gap: 12, marginBottom: 8 }}>
          <span className="mono soft" style={{ fontSize: 13, marginTop: 4 }}>#{c.id}</span>
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 600, letterSpacing: '-0.02em', flex: 1 }}>
            {c.title}
          </h1>
          <div className="row" style={{ gap: 6 }}>
            <StatusBadge status={c.status} labelOpen={t('statusOpen')} labelClosed={t('statusClosed')} />
            {isPrivate && <span data-testid="private-badge"><PrivateBadge label={t('badgePrivate')} /></span>}
          </div>
        </div>

        <div className="row soft" style={{ fontSize: 12, gap: 16, marginBottom: 16, flexWrap: 'wrap' }}>
          <span><IconCalendar size={11} style={{ marginRight: 4, verticalAlign: '-2px' }} />
            {t('labelCreatedTimestamp')} <span className="mono" data-testid="created-timestamp-value">{formatTimestamp(c.createdAt)}</span>
          </span>
          <span>
            {t('labelUpdatedTimestamp')} <span className="mono" data-testid="updated-timestamp-value">{formatTimestamp(c.updatedAt)}</span>
          </span>
          {slackChannelID && (
            <a
              className="slack-link"
              href={slackChannelURL || `slack://channel?id=${slackChannelID}`}
              target="_blank"
              rel="noreferrer noopener"
            >
              <IconSlack size={11} />#{slackChannelName || slackChannelID}
              <IconExt size={10} />
            </a>
          )}
        </div>

        {isPrivate && (
          <div className="private-banner" style={{ marginBottom: 20 }}>
            <IconLock size={13} sw={2} />
            <span>
              This case is <b>Private</b>. Only members of <b>#{slackChannelName || slackChannelID}</b> can view or edit.
            </span>
          </div>
        )}

        <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 24 }}>
          <div className="col" style={{ gap: 24 }}>
            <section>
              <div className="field-label">{t('labelDescription')}</div>
              <p style={{ fontSize: 13.5, lineHeight: 1.65, whiteSpace: 'pre-wrap', margin: 0 }}>
                {c.description || t('labelNoDescription')}
              </p>
            </section>

            <section>
              <div className="row" style={{ marginBottom: 10 }}>
                <div className="field-label" style={{ marginBottom: 0 }}>{t('sectionRelatedActions')}</div>
                <span className="spacer" />
                <Button size="sm" icon={<IconPlus size={12} />}>{t('btnAddAction')}</Button>
              </div>
              {(!c.actions || c.actions.length === 0) ? (
                <div className="card" style={{ padding: 24, textAlign: 'center' }}>
                  <h3 style={{ fontSize: 14, margin: 0 }}>{t('emptyActionsTitle')}</h3>
                  <p className="muted" style={{ fontSize: 12, marginTop: 6 }}>{t('emptyActionsDesc')}</p>
                  <div style={{ marginTop: 12 }}>
                    <Button size="sm" icon={<IconPlus size={12} />}>{t('btnAddAction')}</Button>
                  </div>
                </div>
              ) : (
                <div className="col" style={{ gap: 6 }}>
                  {c.actions.map((a: any) => (
                    <Link
                      key={a.id}
                      to={`/ws/${currentWorkspace!.id}/cases/${c.id}/actions/${a.id}`}
                      className="row"
                      style={{
                        padding: 10, border: '1px solid var(--line)', borderRadius: 6, gap: 10,
                        background: 'var(--bg-elev)', textDecoration: 'none', color: 'inherit',
                      }}
                    >
                      <span
                        className={'pip ' + ({
                          BACKLOG: 'pip-bg', TODO: 'pip-todo', IN_PROGRESS: 'pip-prog',
                          BLOCKED: 'pip-block', COMPLETED: 'pip-done',
                        }[a.status as string] || 'pip-bg')}
                        style={{ width: 8, height: 8, borderRadius: '50%' }}
                      />
                      <span style={{ fontSize: 13, fontWeight: 500, flex: 1, textDecoration: a.status === 'COMPLETED' ? 'line-through' : 'none', color: a.status === 'COMPLETED' ? 'var(--fg-soft)' : undefined }}>
                        {a.title}
                      </span>
                      {a.assignees?.[0] && <Avatar size="sm" name={a.assignees[0].name} realName={a.assignees[0].realName} imageUrl={a.assignees[0].imageUrl} />}
                    </Link>
                  ))}
                </div>
              )}
            </section>

            {c.knowledges && c.knowledges.length > 0 && (
              <section>
                <div className="field-label">{t('sectionRelatedKnowledge', { count: c.knowledges.length })}</div>
                <div className="col" style={{ gap: 6 }}>
                  {c.knowledges.map((k: any) => (
                    <Link
                      key={k.id}
                      to={`/ws/${currentWorkspace!.id}/knowledges/${k.id}`}
                      style={{ display: 'block', padding: 10, border: '1px solid var(--line)', borderRadius: 6, background: 'var(--bg-elev)', textDecoration: 'none', color: 'inherit' }}
                    >
                      <div style={{ fontSize: 13, fontWeight: 500 }}>{k.title}</div>
                      {k.summary && <div className="soft" style={{ fontSize: 11.5, marginTop: 2 }}>{k.summary}</div>}
                    </Link>
                  ))}
                </div>
              </section>
            )}
          </div>

          <aside className="col" style={{ gap: 18 }}>
            <section>
              <div className="field-label">{t('sectionAssignees')}</div>
              {(!c.assignees || c.assignees.length === 0) ? (
                <span className="soft" style={{ fontSize: 12 }}>{t('emptyValue')}</span>
              ) : (
                <div className="col" style={{ gap: 6 }}>
                  {c.assignees.map((u: User) => (
                    <div key={u.id} className="row" style={{ gap: 8 }}>
                      <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />
                      <span style={{ fontSize: 13 }}>{u.realName}</span>
                    </div>
                  ))}
                </div>
              )}
            </section>

            {c.reporter && (
              <section>
                <div className="field-label">{t('labelReporter')}</div>
                <div className="row" style={{ gap: 8 }}>
                  <Avatar size="sm" name={c.reporter.name} realName={c.reporter.realName} imageUrl={c.reporter.imageUrl} />
                  <span style={{ fontSize: 13 }}>{c.reporter.realName}</span>
                </div>
              </section>
            )}

            {fields.length > 0 && (
              <section>
                <h3 style={{ margin: 0, fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--fg-soft)', marginBottom: 6 }}>
                  {t('sectionFields')}
                </h3>
                <div className="col" style={{ gap: 8 }}>
                  {fields.map((f: any) => (
                    <div key={f.id} style={{ padding: '8px 10px', border: '1px solid var(--line)', borderRadius: 6 }}>
                      <div style={{ fontSize: 10.5, color: 'var(--fg-soft)', textTransform: 'uppercase', letterSpacing: '0.06em', fontWeight: 600 }}>{f.name}</div>
                      <div style={{ fontSize: 13, marginTop: 2 }}>{renderFieldValue(f.id)}</div>
                    </div>
                  ))}
                </div>
              </section>
            )}

            {isPrivate && slackChannelID && (
              <section data-testid="channel-members-section">
                <div className="row" style={{ marginBottom: 8 }}>
                  <h3 style={{ margin: 0, fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--fg-soft)' }}>
                    {t('sectionChannelMembers', { count: memberTotal })}
                  </h3>
                  <span className="spacer" />
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={handleSync}
                    disabled={syncing}
                    data-testid="sync-members-button"
                    icon={<IconRefresh size={12} />}
                  >
                    {t('btnSync')}
                  </Button>
                </div>
                <div className="h-search" style={{ width: '100%', marginLeft: 0, marginBottom: 8 }}>
                  <IconSearch size={13} />
                  <input
                    value={memberFilter}
                    onChange={(e) => setMemberFilter(e.target.value)}
                    placeholder={t('placeholderFilterMembers')}
                    data-testid="member-search-filter"
                    style={{ flex: 1, border: 'none', background: 'transparent', outline: 'none', fontFamily: 'inherit', fontSize: 12.5, color: 'var(--fg)' }}
                  />
                </div>
                <div className="col" style={{ gap: 0 }}>
                  {members.map((m) => (
                    <div key={m.id} className={styles.memberItem}>
                      <Avatar size="sm" name={m.name} realName={m.realName} imageUrl={m.imageUrl} />
                      <span className={`name truncate ${styles.name}`}>{m.realName}</span>
                    </div>
                  ))}
                </div>
              </section>
            )}
          </aside>
        </div>
      </div>

      {editing && <CaseForm caseItem={{
        id: c.id, title: c.title, description: c.description,
        isPrivate: c.isPrivate, assigneeIDs: c.assigneeIDs || [],
        fields: c.fields || [],
      }} onClose={() => setEditing(false)} />}

      {confirmClose && (
        <Modal
          open
          onClose={() => setConfirmClose(false)}
          title={t('titleCloseCase', { caseLabel })}
          width={460}
          footer={
            <>
              <Button variant="ghost" onClick={() => setConfirmClose(false)}>{t('btnCancel')}</Button>
              <Button variant="primary" onClick={handleClose} disabled={closing} data-testid="confirm-close-button">
                {t('btnClose')}
              </Button>
            </>
          }
        >
          <div
            style={{ fontSize: 13, lineHeight: 1.6 }}
            dangerouslySetInnerHTML={{ __html: t('msgCloseCaseConfirm', { title: escapeHtml(c.title) }) }}
          />
        </Modal>
      )}

      {confirmDelete && (
        <CaseDeleteDialog
          caseTitle={c.title}
          caseLabel={caseLabel}
          deleting={deleting}
          onCancel={() => setConfirmDelete(false)}
          onConfirm={handleDelete}
        />
      )}
    </div>
  )
}

function escapeHtml(s: string) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}
