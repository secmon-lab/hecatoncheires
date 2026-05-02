import { useState, useMemo } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import {
  GET_CASE,
  GET_CASE_MEMBERS,
  CLOSE_CASE,
  REOPEN_CASE,
  DELETE_CASE,
  UPDATE_CASE,
  SYNC_CASE_CHANNEL_USERS,
  GET_CASES,
} from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import InlineCustomField from '../components/inline/InlineCustomField'
import InlineText from '../components/inline/InlineText'
import InlineLongText from '../components/inline/InlineLongText'
import InlineUserSelect from '../components/inline/InlineUserSelect'
import FilterDropdown from '../components/FilterDropdown'
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
  IconDots,
  IconBell,
} from '../components/Icons'
import { Avatar, PrivateBadge, StatusBadge } from '../components/Primitives'
import CaseDeleteDialog from './CaseDeleteDialog'
import ActionForm from './ActionForm'
import ActionModal from './ActionModal'
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

function formatHHMM(iso?: string | null) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}

const STATUS_CLASS: Record<string, string> = {
  BACKLOG: 'backlog',
  TODO: 'todo',
  IN_PROGRESS: 'in-progress',
  BLOCKED: 'blocked',
  COMPLETED: 'done',
}
const STATUS_PIP: Record<string, string> = {
  BACKLOG: 'pip-bg',
  TODO: 'pip-todo',
  IN_PROGRESS: 'pip-prog',
  BLOCKED: 'pip-block',
  COMPLETED: 'pip-done',
}
const STATUS_LABEL_KEY = {
  BACKLOG: 'statusBacklog',
  TODO: 'statusTodo',
  IN_PROGRESS: 'statusInProgress',
  BLOCKED: 'statusBlocked',
  COMPLETED: 'statusCompleted',
} as const

export default function CaseDetail() {
  const { id, actionId: actionIdParam } = useParams<{ id: string; actionId?: string }>()
  const caseId = Number(id)
  const openActionId = actionIdParam ? Number(actionIdParam) : null
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const [addingAction, setAddingAction] = useState(false)
  const [actionStatusFilters, setActionStatusFilters] = useState<string[]>([])
  const [actionAssigneeFilters, setActionAssigneeFilters] = useState<string[]>([])
  const [confirmClose, setConfirmClose] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
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
  const [updateCase, { loading: updating }] = useMutation(UPDATE_CASE, { refetchQueries: refetchOptions })
  const [deleteCase, { loading: deleting }] = useMutation(DELETE_CASE, {
    refetchQueries: [
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' } },
      { query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'CLOSED' } },
    ],
  })
  const { data: slackUsersData } = useQuery(GET_SLACK_USERS)
  const slackUsers = slackUsersData?.slackUsers || []

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
  const handleAssigneesChange = async (ids: string[]) => {
    await updateCase({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: caseId, assigneeIDs: ids },
      },
    })
  }
  const handleTitleChange = async (next: string) => {
    if (next === c.title) return
    await updateCase({
      variables: { workspaceId: currentWorkspace!.id, input: { id: caseId, title: next } },
    })
  }
  const handleDescriptionChange = async (next: string) => {
    if (next === (c.description || '')) return
    await updateCase({
      variables: { workspaceId: currentWorkspace!.id, input: { id: caseId, description: next } },
    })
  }
  const handleFieldChange = async (fieldId: string, value: any) => {
    // Send only the changed field. The backend merges this with existing
    // values, so we don't risk re-validating stale entries (e.g. option IDs
    // from a prior config) that would cause the whole update to be rejected.
    let v = value
    // Promote a "YYYY-MM-DD" date to RFC3339 — the backend's date validator
    // requires a full timestamp string.
    if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v)) {
      v = `${v}T00:00:00Z`
    }
    // Empty single-value: nothing to persist (the backend has no explicit
    // "clear field" path; preserve current value).
    const isEmptySingle = v == null || v === ''
    if (isEmptySingle) return
    await updateCase({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: { id: caseId, fields: [{ fieldId, value: v }] },
      },
    })
  }

  const members: User[] = membersData?.case?.channelUsers?.items || []
  const memberTotal: number = membersData?.case?.channelUserCount ?? channelUserCount

  const totalActions = c.actions?.length || 0
  const doneActions = (c.actions || []).filter((a: any) => a.status === 'COMPLETED').length
  const progressPct = totalActions > 0 ? Math.round((doneActions / totalActions) * 100) : 0

  return (
    <div className="h-main-inner" style={{ maxWidth: 1180 }}>
      {/* top action row */}
      <div className="row" style={{ marginBottom: 18 }}>
        <Button
          variant="ghost"
          size="sm"
          icon={<IconChevLeft size={13} />}
          onClick={() => navigate(`/ws/${currentWorkspace!.id}/cases`)}
        >
          {t('btnBack')}
        </Button>
        <span className="spacer" />
        <Button
          size="sm"
          variant="ghost"
          icon={<IconBell size={13} />}
          data-testid="watch-case-button"
          aria-label={t('btnWatch')}
        >
          {t('btnWatch')}
        </Button>
        {c.status === 'OPEN' ? (
          <Button
            size="sm"
            variant="danger"
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
        <div style={{ position: 'relative' }}>
          <Button
            size="sm"
            variant="ghost"
            icon={<IconDots size={14} />}
            onClick={() => setMenuOpen((v) => !v)}
            aria-label={t('btnMore')}
            data-testid="case-menu-button"
          />
          {menuOpen && (
            <>
              <div
                onClick={() => setMenuOpen(false)}
                style={{ position: 'fixed', inset: 0, zIndex: 100 }}
              />
              <div
                role="menu"
                data-testid="case-menu-popover"
                className={styles.kebabMenu}
              >
                <button
                  type="button"
                  onClick={() => { setMenuOpen(false); setConfirmDelete(true) }}
                  data-testid="case-delete-menu-item"
                  className={`${styles.kebabItem} ${styles.kebabDanger}`}
                >
                  {t('btnDelete')}
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {/* title row */}
      <div className="h-detail-h">
        <span className="h-detail-id">#{c.id}</span>
        <h1 style={{ flex: 1, margin: 0 }}>
          <InlineText
            value={c.title}
            onSave={handleTitleChange}
            ariaLabel={t('labelTitle')}
            variant="title"
            placeholder={t('placeholderAddTitle')}
            disabled={updating}
            testId="case-title"
          />
        </h1>
        <div className="h-detail-badges">
          {isPrivate && <span data-testid="private-badge"><PrivateBadge label={t('badgePrivate')} /></span>}
        </div>
      </div>

      {/* sub-meta */}
      <div className="h-detail-meta">
        <span>
          <IconCalendar size={11} style={{ marginRight: 4, verticalAlign: '-2px' }} />
          {t('labelCreatedTimestamp')} <span className="mono" data-testid="created-timestamp-value">{formatTimestamp(c.createdAt)}</span>
        </span>
        <span>
          {t('labelUpdatedTimestamp')} <span className="mono" data-testid="updated-timestamp-value">{formatTimestamp(c.updatedAt)}</span>
        </span>
        {c.reporter && (
          <span>
            {t('labelBy')} <span className="name">{c.reporter.realName || c.reporter.name}</span>
          </span>
        )}
        <span className="spacer" />
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

      {/* private banner */}
      {isPrivate && (
        <div className="h-banner warn" data-testid="case-private-banner">
          <IconLock size={13} sw={2} />
          <span className="h-banner-text">
            {t('msgPrivateBanner', {
              channel: slackChannelName || slackChannelID,
              count: String(channelUserCount || 0),
            })}
          </span>
        </div>
      )}

      {/* main grid */}
      <div className="h-detail-grid">
        {/* left column */}
        <div>
          <section className="h-section h-detail-body">
            <div className="h-section-h">
              <span className="h-section-title">{t('labelDescription')}</span>
            </div>
            <InlineLongText
              value={c.description || ''}
              onSave={handleDescriptionChange}
              ariaLabel={t('labelDescription')}
              placeholder={t('placeholderAddDescription')}
              disabled={updating}
              testId="case-description"
            />
          </section>

          <section className="h-section">
            <div className="h-section-h">
              <span className="h-section-title">{t('sectionRelatedActions')}</span>
              {totalActions > 0 && (
                <>
                  <span className="h-section-progress" data-testid="related-actions-progress">
                    {t('labelProgress', { done: String(doneActions), total: String(totalActions) })}
                  </span>
                  <div className="h-section-bar">
                    <span data-testid="related-actions-progress-bar" style={{ width: `${progressPct}%` }} />
                  </div>
                </>
              )}
              <span className="spacer" />
              <Button
                size="sm"
                icon={<IconPlus size={12} />}
                onClick={() => setAddingAction(true)}
                data-testid="add-action-button"
              >
                {t('btnAddAction')}
              </Button>
            </div>

            {totalActions > 0 && (() => {
              const STATUS_ORDER: Array<{ value: string; labelKey: 'statusBacklog' | 'statusTodo' | 'statusInProgress' | 'statusBlocked' | 'statusCompleted' }> = [
                { value: 'BACKLOG', labelKey: 'statusBacklog' },
                { value: 'TODO', labelKey: 'statusTodo' },
                { value: 'IN_PROGRESS', labelKey: 'statusInProgress' },
                { value: 'BLOCKED', labelKey: 'statusBlocked' },
                { value: 'COMPLETED', labelKey: 'statusCompleted' },
              ]
              const presentStatuses = new Set<string>(c.actions.map((a: any) => a.status))
              const statusOpts = STATUS_ORDER
                .filter((s) => presentStatuses.has(s.value))
                .map((s) => ({ value: s.value, label: t(s.labelKey) }))

              const assigneeMap = new Map<string, { value: string; label: string }>()
              let anyUnassigned = false
              c.actions.forEach((a: any) => {
                if (!a.assignees || a.assignees.length === 0) {
                  anyUnassigned = true
                  return
                }
                a.assignees.forEach((u: any) => {
                  if (!assigneeMap.has(u.id)) {
                    assigneeMap.set(u.id, { value: u.id, label: u.realName || u.name })
                  }
                })
              })
              const assigneeOpts: Array<{ value: string; label: string }> = []
              if (anyUnassigned) assigneeOpts.push({ value: 'NONE', label: t('filterUnassigned') })
              assigneeOpts.push(...Array.from(assigneeMap.values()))

              const filtered = (c.actions as any[]).filter((a) => {
                if (actionStatusFilters.length > 0 && !actionStatusFilters.includes(a.status)) return false
                if (actionAssigneeFilters.length > 0) {
                  const wantUnassigned = actionAssigneeFilters.includes('NONE')
                  const isUnassigned = !a.assignees || a.assignees.length === 0
                  const userIds = actionAssigneeFilters.filter((id) => id !== 'NONE')
                  const matchesUser = userIds.length > 0
                    && (a.assignees || []).some((u: any) => userIds.includes(u.id))
                  if (!((wantUnassigned && isUnassigned) || matchesUser)) return false
                }
                return true
              })

              return (
                <>
                  <div className="h-filter-bar">
                    <FilterDropdown
                      testId="related-action-status-filter"
                      label="Status"
                      allLabel={t('filterAllShort')}
                      options={statusOpts}
                      value={actionStatusFilters}
                      onChange={setActionStatusFilters}
                    />
                    <FilterDropdown
                      testId="related-action-assignee-filter"
                      label="Assignee"
                      allLabel={t('filterAllAssigneesShort')}
                      options={assigneeOpts}
                      value={actionAssigneeFilters}
                      onChange={setActionAssigneeFilters}
                    />
                    <span className="h-filter-count">{t('labelCount', { count: filtered.length })}</span>
                  </div>

                  {filtered.length === 0 ? (
                    <div className="card" style={{ padding: 16, textAlign: 'center' }} data-testid="related-actions-empty-after-filter">
                      <p className="muted" style={{ fontSize: 12, margin: 0 }}>{t('emptyActionsFilter')}</p>
                    </div>
                  ) : (
                    <div className="h-action-list">
                      {filtered.map((a: any) => {
                        const status = String(a.status)
                        const cls = STATUS_CLASS[status] || 'backlog'
                        const pip = STATUS_PIP[status] || 'pip-bg'
                        const labelKey = STATUS_LABEL_KEY[status as keyof typeof STATUS_LABEL_KEY]
                        return (
                          <Link
                            key={a.id}
                            to={`/ws/${currentWorkspace!.id}/cases/${c.id}/actions/${a.id}`}
                            className="h-action-row"
                            data-testid="case-related-action"
                            data-status={status}
                          >
                            <span className={`h-action-pip ${pip}`} />
                            <span className="h-action-title">{a.title}</span>
                            {labelKey && (
                              <span
                                className={`h-action-status ${cls}`}
                                data-testid="case-related-action-status"
                              >
                                {t(labelKey)}
                              </span>
                            )}
                            {status === 'COMPLETED' && a.updatedAt && (
                              <span className="h-action-meta" data-testid="case-related-action-completed-at">
                                {t('labelCompleted')}<span className="mono">{formatHHMM(a.updatedAt)}</span>
                              </span>
                            )}
                            {a.assignees?.[0] && (
                              <Avatar
                                size="sm"
                                name={a.assignees[0].name}
                                realName={a.assignees[0].realName}
                                imageUrl={a.assignees[0].imageUrl}
                              />
                            )}
                          </Link>
                        )
                      })}
                    </div>
                  )}
                </>
              )
            })()}

            {totalActions === 0 && (
              <div className="card" style={{ padding: 24, textAlign: 'center' }}>
                <h3 style={{ fontSize: 14, margin: 0 }}>{t('emptyActionsTitle')}</h3>
                <p className="muted" style={{ fontSize: 12, marginTop: 6 }}>{t('emptyActionsDesc')}</p>
                <div style={{ marginTop: 12 }}>
                  <Button
                    size="sm"
                    icon={<IconPlus size={12} />}
                    onClick={() => setAddingAction(true)}
                    data-testid="empty-add-action-button"
                  >
                    {t('btnAddAction')}
                  </Button>
                </div>
              </div>
            )}
          </section>

          {c.knowledges && c.knowledges.length > 0 && (
            <section className="h-section">
              <div className="h-section-h">
                <span className="h-section-title">{t('sectionRelatedKnowledge', { count: c.knowledges.length })}</span>
              </div>
              <div className="col" style={{ gap: 6 }}>
                {c.knowledges.map((k: any) => (
                  <Link
                    key={k.id}
                    to={`/ws/${currentWorkspace!.id}/knowledges/${k.id}`}
                    className="h-action-row"
                  >
                    <span className="h-action-title">
                      {k.title}
                      {k.summary && <span className="soft" style={{ fontSize: 11.5, marginLeft: 8, fontWeight: 400 }}>{k.summary}</span>}
                    </span>
                  </Link>
                ))}
              </div>
            </section>
          )}
        </div>

        {/* right column / sidebar */}
        <aside className="h-aside">
          <section className="h-aside-section">
            <div className="h-aside-h">
              <span className="h-aside-title">{t('labelStatus')}</span>
            </div>
            <div data-testid="aside-status-display" className="h-status-strong">
              <StatusBadge
                status={c.status}
                labelOpen={t('statusOpen')}
                labelClosed={t('statusClosed')}
              />
            </div>
          </section>

          <section className="h-aside-section" data-testid="case-assignees-inline">
            <div className="h-aside-h">
              <span className="h-aside-title">{t('sectionAssignees')}</span>
            </div>
            <InlineUserSelect
              isMulti
              users={slackUsers}
              value={c.assigneeIDs || []}
              onSave={(ids) => handleAssigneesChange(ids as string[])}
              ariaLabel={t('sectionAssignees')}
              placeholder={t('placeholderAddAssignees')}
              disabled={updating}
              testId="case-assignees"
            />
          </section>

          {c.reporter && (
            <section className="h-aside-section">
              <div className="h-aside-h">
                <span className="h-aside-title">{t('labelReporter')}</span>
              </div>
              <div className="row" style={{ gap: 8, alignItems: 'center' }}>
                <Avatar size="sm" name={c.reporter.name} realName={c.reporter.realName} imageUrl={c.reporter.imageUrl} />
                <span style={{ fontSize: 13 }}>{c.reporter.realName || c.reporter.name}</span>
              </div>
            </section>
          )}

          {fields.length > 0 && (
            <section className="h-aside-section" data-testid="case-fields-inline">
              <div className="h-aside-h">
                <span className="h-aside-title">{t('sectionFields')}</span>
              </div>
              <div className="kv-list">
                {fields.map((f: any) => {
                  const fv = c.fields?.find((x: any) => x.fieldId === f.id)
                  return (
                    <div key={f.id} className="kv-row" data-testid={`case-field-${f.id}`}>
                      <span className="kv-label">{f.name}</span>
                      <span className="kv-value">
                        <InlineCustomField
                          field={f}
                          value={fv?.value}
                          users={slackUsers}
                          disabled={updating}
                          onSave={(v) => handleFieldChange(f.id, v)}
                        />
                      </span>
                    </div>
                  )
                })}
              </div>
            </section>
          )}

          {isPrivate && slackChannelID && (
            <section className="h-aside-section" data-testid="channel-members-section">
              <div className="h-aside-h">
                <span className="h-aside-title">
                  {t('sectionChannelMembers', { count: memberTotal })}
                </span>
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

      {addingAction && (
        <ActionForm
          action={null}
          defaultCaseID={c.id}
          onClose={() => setAddingAction(false)}
        />
      )}

      {openActionId !== null && (
        <ActionModal
          actionId={openActionId}
          onClose={() => navigate(`/ws/${currentWorkspace!.id}/cases/${c.id}`)}
        />
      )}

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
