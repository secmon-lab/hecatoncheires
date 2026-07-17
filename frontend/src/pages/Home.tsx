import { useMemo, type ReactNode } from 'react'
import { useQuery } from '@apollo/client'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation, type MsgKey } from '../i18n'
import { useWorkspace } from '../contexts/workspace-context'
import { UserMenu } from '../components/UserMenu'
import WorkspaceChooser from '../components/WorkspaceChooser'
import Button from '../components/Button'
import {
  GET_HOME_MESSAGE,
  GET_MY_OPEN_CASES,
  GET_MY_DUE_ACTIONS,
} from '../graphql/dashboard'
import { toRFC3339WithOffset, formatHomeDate } from '../utils/time'
import { displayName } from '../utils/user'
import { workspaceVisual } from '../utils/workspace'
import { activateOnEnterOrSpace } from '../utils/keyboard'
import styles from './Home.module.css'

type TFunc = (key: MsgKey, params?: Record<string, string | number>) => string

interface HomeUser {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

interface HomeCase {
  id: number
  title: string
  status: string
  assigneeIDs: string[]
  assignees: HomeUser[]
  updatedAt: string
}

interface MyOpenCase {
  workspaceId: string
  workspaceName: string
  stalled: boolean
  case: HomeCase
}

interface HomeAction {
  id: number
  title: string
  status: string
  dueDate?: string | null
}

interface MyDueAction {
  workspaceId: string
  workspaceName: string
  caseId: number
  caseTitle: string
  action: HomeAction
}

// ─── Ported helpers (see the approved design's renderVals() logic) ───

// localDayStart strips the time-of-day so day-difference math compares
// calendar days, not 24h windows (e.g. "yesterday at 23:59" must count as
// 1 day ago even if less than 24 raw hours have elapsed).
function localDayStart(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

// daysDiff > 0 means `iso` is in the past (overdue by that many days),
// 0 means today, and < 0 means `iso` is that many days in the future.
function daysDiff(iso: string, now: Date): number {
  const target = new Date(iso)
  return Math.round((localDayStart(now) - localDayStart(target)) / 86_400_000)
}

// rank buckets a due-date diff for sorting: overdue first, then today, then
// future, then no-due-date last.
function rank(dd: number | null): number {
  if (dd === null) return 3
  if (dd > 0) return 0
  if (dd === 0) return 1
  return 2
}

function sortDueActions(rows: MyDueAction[], now: Date): MyDueAction[] {
  return [...rows].sort((x, y) => {
    const dx = x.action.dueDate ? daysDiff(x.action.dueDate, now) : null
    const dy = y.action.dueDate ? daysDiff(y.action.dueDate, now) : null
    const byRank = rank(dx) - rank(dy)
    if (byRank !== 0) return byRank
    // Within the same rank, descending dd: for overdue (dd>0) the larger
    // (older) due date sorts first; for future (dd<0) the smaller
    // magnitude (nearer date) sorts first — one formula covers both.
    return (dy ?? -1) - (dx ?? -1)
  })
}

function mdy(d: Date): string {
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${mm}/${dd}`
}

function avatarInitial(u: HomeUser): string {
  const name = displayName(u)
  const first = [...name.trim()][0]
  return first ? first.toUpperCase() : '?'
}

// ─── Small shared pieces ───

function WsBadge({ id, name }: { id: string; name: string }) {
  const visual = workspaceVisual({ id, name })
  if (visual.kind === 'emoji') {
    return <span className={styles.wsBadgeEmoji} aria-hidden="true">{visual.emoji}</span>
  }
  return <span className={styles.wsBadgeColor} style={{ background: visual.background }} aria-hidden="true" />
}

function SkeletonRows({ cols }: { cols: number }) {
  return (
    <div aria-hidden="true" data-testid="home-skeleton">
      {[0, 1, 2].map((i) => (
        <div key={i} className={styles.skeletonRow}>
          {Array.from({ length: cols }).map((_, j) => (
            <span key={j} className={styles.skeletonBlock} />
          ))}
        </div>
      ))}
    </div>
  )
}

function CaseRow({ row, now, t }: { row: MyOpenCase; now: Date; t: TFunc }) {
  const navigate = useNavigate()
  const caseHref = `/ws/${row.workspaceId}/cases/${row.case.id}`
  const wsHref = `/ws/${row.workspaceId}/cases`
  const assignees = row.case.assignees ?? []
  const avatars = assignees.slice(0, 3)
  const extra = Math.max(0, assignees.length - 3)
  const dd = daysDiff(row.case.updatedAt, now)
  const activate = () => navigate(caseHref)

  return (
    <div
      className={`${styles.caseRow} ${row.stalled ? styles.caseRowStalled : ''}`}
      role="link"
      tabIndex={0}
      onClick={activate}
      onKeyDown={activateOnEnterOrSpace(activate)}
      data-testid="home-case-row"
    >
      <Link to={caseHref} className={styles.caseTitleLink} tabIndex={-1} onClick={(e) => e.stopPropagation()}>
        {row.case.title}
      </Link>
      <span className={styles.statusCell}>
        <span className={styles.statusDot} aria-hidden="true" />
        {t('statusOpen')}
      </span>
      <span className={styles.avatarStack}>
        {avatars.map((u) => (
          <span key={u.id} className={styles.avatarChip} title={displayName(u)}>
            {avatarInitial(u)}
          </span>
        ))}
        {extra > 0 && <span className={styles.avatarExtra}>{t('extra', { n: extra })}</span>}
      </span>
      <span className={styles.updatedCell}>{dd === 0 ? t('today') : t('updated', { n: dd })}</span>
      <Link to={wsHref} className={styles.wsBadgeLink} onClick={(e) => e.stopPropagation()}>
        <WsBadge id={row.workspaceId} name={row.workspaceName} />
        <span className={styles.wsBadgeName}>{row.workspaceName}</span>
      </Link>
      {row.stalled && (
        <span className={styles.stalledPill}>
          <span className={styles.stalledDot} aria-hidden="true" />
          {t('stalled')}
        </span>
      )}
    </div>
  )
}

function ActionRow({ row, now, t }: { row: MyDueAction; now: Date; t: TFunc }) {
  const navigate = useNavigate()
  const actionHref = `/ws/${row.workspaceId}/cases/${row.caseId}/actions/${row.action.id}`
  const caseHref = `/ws/${row.workspaceId}/cases/${row.caseId}`
  const wsHref = `/ws/${row.workspaceId}/cases`
  const dd = row.action.dueDate ? daysDiff(row.action.dueDate, now) : null
  const overdue = dd !== null && dd > 0
  const activate = () => navigate(actionHref)

  let dueNode: ReactNode
  if (dd === null) {
    dueNode = <span className={styles.dueNone}>{t('noDue')}</span>
  } else if (dd > 0) {
    dueNode = <span className={styles.duePillOverdue}>{t('overdue', { n: dd })}</span>
  } else if (dd === 0) {
    dueNode = <span className={styles.dueToday}>{t('today')}</span>
  } else {
    dueNode = <span className={styles.dueFuture}>{mdy(new Date(row.action.dueDate as string))}</span>
  }

  return (
    <div
      className={`${styles.actionRow} ${overdue ? styles.actionRowOverdue : ''}`}
      role="link"
      tabIndex={0}
      onClick={activate}
      onKeyDown={activateOnEnterOrSpace(activate)}
      data-testid="home-action-row"
    >
      <span className={styles.dueCell}>{dueNode}</span>
      <span className={styles.actionTitle}>{row.action.title}</span>
      <Link to={caseHref} className={styles.caseLink} onClick={(e) => e.stopPropagation()}>
        {row.caseTitle}
      </Link>
      <Link to={wsHref} className={styles.wsBadgeLink} onClick={(e) => e.stopPropagation()}>
        <WsBadge id={row.workspaceId} name={row.workspaceName} />
        <span className={styles.wsBadgeName}>{row.workspaceName}</span>
      </Link>
    </div>
  )
}

export default function Home() {
  const { t, lang } = useTranslation()
  const { workspaces, isLoading: workspacesLoading, favoriteWorkspaceIds, toggleFavorite } = useWorkspace()

  // Captured once per mount: the header date and the "now" used for every
  // day-diff calculation should stay internally consistent for the whole
  // render, not drift mid-session.
  const now = useMemo(() => new Date(), [])
  const clientTime = useMemo(() => toRFC3339WithOffset(now), [now])
  const dateLabel = useMemo(() => formatHomeDate(now, lang), [now, lang])

  const {
    data: msgData,
    loading: msgLoading,
    error: msgError,
  } = useQuery<{ homeMessage: { message: string } }>(GET_HOME_MESSAGE, {
    variables: { clientTime, lang },
    fetchPolicy: 'cache-first',
  })

  const {
    data: casesData,
    loading: casesLoading,
    error: casesError,
    refetch: refetchCases,
  } = useQuery<{ myOpenCases: MyOpenCase[] }>(GET_MY_OPEN_CASES)
  const {
    data: actionsData,
    loading: actionsLoading,
    error: actionsError,
    refetch: refetchActions,
  } = useQuery<{ myDueActions: MyDueAction[] }>(GET_MY_DUE_ACTIONS)

  const openCases = casesData?.myOpenCases ?? []
  const dueActions = useMemo(
    () => sortDueActions(actionsData?.myDueActions ?? [], now),
    [actionsData, now],
  )

  // The greeting is an independent query: a failure or empty message here
  // must never hide the sections below it.
  const greetingText =
    !msgError && msgData?.homeMessage.message ? msgData.homeMessage.message : t('homeMessageFallback')

  return (
    <div className={styles.page} data-testid="home-page">
      <div className={styles.container}>
        <header className={styles.header}>
          <div>
            <h1 className={styles.greeting}>
              {msgLoading ? (
                <span
                  className={styles.greetingSkeleton}
                  role="status"
                  aria-label={t('homeMessageLoadingAria')}
                />
              ) : (
                greetingText
              )}
            </h1>
            <div className={styles.dateLabel}>{dateLabel}</div>
          </div>
          <div className={styles.headerActions}>
            <UserMenu />
          </div>
        </header>

        {/* Section B: My Open Cases */}
        <section className={styles.card} data-testid="home-open-cases">
          <div className={styles.cardHeader}>
            <h2 className={styles.cardTitle}>{t('sectionB')}</h2>
            <span className={styles.cardCount}>{t('count', { n: openCases.length })}</span>
          </div>
          {casesLoading ? (
            <SkeletonRows cols={6} />
          ) : casesError ? (
            <div className={styles.errorState} data-testid="home-cases-error">
              <div className={styles.errorBody}>{t('homeSectionLoadError')}</div>
              <Button size="sm" onClick={() => { void refetchCases() }}>
                {t('homeRetry')}
              </Button>
            </div>
          ) : openCases.length === 0 ? (
            <div className={styles.emptyState}>
              <div className={styles.emptyTitle}>{t('emptyBTitle')}</div>
              <div className={styles.emptyBody}>{t('emptyBBody')}</div>
            </div>
          ) : (
            <div className={styles.tableScroll}>
              <div className={styles.caseHeaderRow}>
                <span>{t('colCaseTitle')}</span>
                <span>{t('colStatus')}</span>
                <span>{t('colAssignee')}</span>
                <span>{t('colUpdated')}</span>
                <span>{t('colWs')}</span>
                <span />
              </div>
              {openCases.map((row) => (
                <CaseRow key={`${row.workspaceId}-${row.case.id}`} row={row} now={now} t={t} />
              ))}
            </div>
          )}
        </section>

        {/* Section A: My Due Actions */}
        <section className={styles.card} data-testid="home-due-actions">
          <div className={styles.cardHeader}>
            <h2 className={styles.cardTitle}>{t('sectionA')}</h2>
            <span className={styles.cardSub}>{t('sectionASub')}</span>
            <span className={styles.cardCount}>{t('count', { n: dueActions.length })}</span>
          </div>
          {actionsLoading ? (
            <SkeletonRows cols={4} />
          ) : actionsError ? (
            <div className={styles.errorState} data-testid="home-actions-error">
              <div className={styles.errorBody}>{t('homeSectionLoadError')}</div>
              <Button size="sm" onClick={() => { void refetchActions() }}>
                {t('homeRetry')}
              </Button>
            </div>
          ) : dueActions.length === 0 ? (
            <div className={styles.emptyState}>
              <div className={styles.emptyTitle}>{t('emptyATitle')}</div>
              <div className={styles.emptyBody}>{t('emptyABody')}</div>
            </div>
          ) : (
            <div className={styles.tableScroll}>
              <div className={styles.actionHeaderRow}>
                <span className={styles.dueCell}>{t('colDue')}</span>
                <span>{t('colAction')}</span>
                <span>{t('colCase')}</span>
                <span>{t('colWs')}</span>
              </div>
              {dueActions.map((row) => (
                <ActionRow key={`${row.workspaceId}-${row.action.id}`} row={row} now={now} t={t} />
              ))}
            </div>
          )}
        </section>

        {/* Section C: Choose a Workspace */}
        <section data-testid="home-workspaces">
          <div className={styles.sectionCHeader}>
            <h2 className={styles.cardTitle}>{t('sectionC')}</h2>
          </div>
          <WorkspaceChooser
            workspaces={workspaces}
            isLoading={workspacesLoading}
            favoriteWorkspaceIds={favoriteWorkspaceIds}
            onToggleFavorite={toggleFavorite}
          />
        </section>
      </div>
    </div>
  )
}
