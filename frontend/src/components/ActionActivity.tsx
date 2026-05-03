import { useMemo, useState, type CSSProperties } from 'react'
import { useQuery } from '@apollo/client'
import { GET_ACTION_MESSAGES, GET_ACTION_EVENTS } from '../graphql/action'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useTranslation, type MsgKey } from '../i18n'
import { useActionStatuses } from '../hooks/useActionStatuses'
import { actionStatusColorStyle } from '../utils/actionStatusStyle'
import Button from './Button'

type EventKind =
  | 'CREATED'
  | 'TITLE_CHANGED'
  | 'STATUS_CHANGED'
  | 'ASSIGNEE_CHANGED'

interface SlackUser {
  id: string
  name: string
  realName: string
  imageUrl: string
}

interface SlackMessage {
  id: string
  channelID: string
  threadTS?: string | null
  teamID: string
  userID: string
  userName: string
  text: string
  createdAt: string
}

interface ActionEvent {
  id: string
  actionID: number
  kind: EventKind
  actorID: string
  actor?: SlackUser | null
  oldValue: string
  newValue: string
  createdAt: string
}

interface MessagesData {
  action: {
    id: number
    messages: { items: SlackMessage[]; nextCursor: string }
  } | null
}

interface EventsData {
  action: {
    id: number
    events: { items: ActionEvent[]; nextCursor: string }
  } | null
}

interface ActionActivityProps {
  workspaceId: string
  actionId: number
  pageSize?: number
  slackMessageTS?: string | null
  slackChannelID?: string | null
  slackChannelURL?: string | null
}

// buildSlackPermalink composes a permalink to the action's thread root.
// Slack accepts archive URLs of the form {channelURL}/p{ts-without-dot}, and
// also tolerates the canonical https://slack.com/archives/... fallback when
// the workspace subdomain isn't known.
function buildSlackPermalink(channelURL: string | null | undefined, channelID: string | null | undefined, ts: string | null | undefined): string | null {
  if (!ts) return null
  const tsCompact = ts.replace('.', '')
  if (channelURL) {
    const trimmed = channelURL.replace(/\/+$/, '')
    return `${trimmed}/p${tsCompact}`
  }
  if (channelID) {
    return `https://slack.com/archives/${channelID}/p${tsCompact}`
  }
  return null
}

type Tab = 'all' | 'comments' | 'history'

const EMOJI_MAP: Record<string, string> = {
  pencil2: '✏️',
  arrows_counterclockwise: '🔁',
  bust_in_silhouette: '👤',
  link: '🔗',
  white_check_mark: '✅',
  warning: '⚠️',
  speech_balloon: '💬',
}

function renderEmojiTokens(text: string): string {
  return text.replace(/:([a-z0-9_+-]+):/gi, (m, name) => EMOJI_MAP[name.toLowerCase()] ?? m)
}

function renderText(text: string, userIdToName: Map<string, string>): string {
  let out = renderEmojiTokens(text)
  out = out.replace(/<@([A-Z0-9]+)(\|[^>]+)?>/g, (_m, uid: string) => `@${userIdToName.get(uid) ?? uid}`)
  out = out.replace(/<([^|>]+)\|([^>]+)>/g, (_m, _url, label) => label)
  out = out.replace(/<([^|>]+)>/g, (_m, url) => url)
  out = out.replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&amp;/g, '&')
  return out
}

function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

function formatTimestamp(iso: string, todayLabel: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  if (isSameDay(d, now)) return `${todayLabel} ${hh}:${mi}`
  const sameYear = d.getFullYear() === now.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  if (sameYear) return `${mm}/${dd} ${hh}:${mi}`
  return `${d.getFullYear()}/${mm}/${dd}`
}

function formatTimestampShort(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  if (isSameDay(d, now)) return `${hh}:${mi}`
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${mm}/${dd} ${hh}:${mi}`
}

function formatTimestampFull(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}`
}


const EVENT_KEY_MAP: Record<EventKind, MsgKey> = {
  CREATED: 'activityEventCreated',
  TITLE_CHANGED: 'activityEventTitleChanged',
  STATUS_CHANGED: 'activityEventStatusChanged',
  ASSIGNEE_CHANGED: 'activityEventAssigneeChanged',
}

const EVENT_ICON: Record<EventKind, string> = {
  CREATED: '＋',
  TITLE_CHANGED: '✎',
  STATUS_CHANGED: '◉',
  ASSIGNEE_CHANGED: '👤',
}

interface UserIndex {
  byName: Map<string, string>
  byImage: Map<string, string>
  byInitial: Map<string, string>
}

function initialOf(name: string): string {
  const trimmed = name.trim()
  if (!trimmed) return '?'
  return trimmed.charAt(0).toUpperCase()
}

const styles: Record<string, CSSProperties> = {
  root: { display: 'flex', flexDirection: 'column', gap: 12 },
  header: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 },
  title: { display: 'flex', alignItems: 'baseline', gap: 6, fontSize: 13, color: 'var(--text-muted)', fontWeight: 500 },
  count: { color: 'var(--text-muted)', fontWeight: 500 },
  tabs: { display: 'inline-flex', background: 'var(--bg-subtle, #F3F4F6)', borderRadius: 6, padding: 2, gap: 2 },
  tab: { appearance: 'none', background: 'transparent', border: 'none', cursor: 'pointer', padding: '4px 10px', fontSize: 12, color: 'var(--text-muted)', borderRadius: 4, fontWeight: 500, lineHeight: 1.4 },
  tabActive: { background: 'var(--bg-paper, #FFFFFF)', color: 'var(--text-heading)', fontWeight: 600, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' },
  feed: { display: 'flex', flexDirection: 'column', position: 'relative', gap: 6 },
  rail: { position: 'absolute', left: 15, top: 12, bottom: 12, width: 1, background: 'var(--border-light, #E5E7EB)' },
  row: { display: 'flex', alignItems: 'flex-start', gap: 12, position: 'relative', padding: '4px 0' },
  avatar: { flex: '0 0 auto', width: 32, height: 32, borderRadius: '50%', background: 'var(--bg-muted, #E5E7EB)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 12, fontWeight: 600, overflow: 'hidden', position: 'relative', zIndex: 1 },
  avatarImg: { width: '100%', height: '100%', objectFit: 'cover' },
  eventIcon: { flex: '0 0 auto', width: 32, height: 32, borderRadius: '50%', background: 'var(--bg-paper, #fff)', border: '1px solid var(--border-light, #E5E7EB)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 13, position: 'relative', zIndex: 1 },
  body: { flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 6, paddingTop: 4 },
  messageHead: { display: 'flex', alignItems: 'baseline', gap: 8, fontSize: 13 },
  messageName: { color: 'var(--text-heading)', fontWeight: 600 },
  timestamp: { color: 'var(--text-muted)', fontSize: 12 },
  messageCard: { border: '1px solid var(--border-light, #E5E7EB)', borderRadius: 6, padding: '8px 12px', background: 'var(--bg-paper, #fff)', fontSize: 13, lineHeight: 1.6, whiteSpace: 'pre-wrap', color: 'var(--text-body)' },
  eventLine: { display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', fontSize: 13, color: 'var(--text-muted)', minHeight: 32, paddingTop: 6, flex: 1, minWidth: 0 },
  eventActor: { color: 'var(--text-heading)', fontWeight: 600 },
  eventVerb: { color: 'var(--text-body)' },
  eventTime: { marginLeft: 'auto', color: 'var(--text-muted)', fontSize: 12, whiteSpace: 'nowrap' },
  statusPill: { display: 'inline-flex', alignItems: 'center', gap: 4, padding: '1px 6px', borderRadius: 4, fontSize: 11, fontWeight: 600, background: 'var(--bg-subtle, #F3F4F6)', color: 'var(--text-body)', border: '1px solid var(--border-light, #E5E7EB)', textTransform: 'uppercase', letterSpacing: '0.02em', fontFamily: 'var(--font-mono, ui-monospace, monospace)' },
  statusDot: { width: 6, height: 6, borderRadius: '50%', flex: '0 0 auto' },
  userPill: { display: 'inline-flex', alignItems: 'center', gap: 4, padding: '1px 6px 1px 2px', borderRadius: 999, fontSize: 12, background: 'var(--bg-subtle, #F3F4F6)', border: '1px solid var(--border-light, #E5E7EB)', color: 'var(--text-body)' },
  userPillImg: { width: 16, height: 16, borderRadius: '50%' },
  userPillFallback: { width: 16, height: 16, borderRadius: '50%', background: 'var(--bg-muted, #E5E7EB)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: 'var(--text-muted)' },
  titleChip: { display: 'inline-flex', alignItems: 'center', padding: '1px 6px', borderRadius: 4, fontSize: 12, background: 'var(--bg-subtle, #F3F4F6)', border: '1px solid var(--border-light, #E5E7EB)', color: 'var(--text-muted)', maxWidth: '16rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  titleChipNew: { background: 'var(--bg-paper, #fff)', color: 'var(--text-body)', borderColor: 'var(--border-medium, #D1D5DB)' },
  arrow: { color: 'var(--text-muted)', fontSize: 12 },
  empty: { color: 'var(--text-muted)', fontSize: 12, padding: '8px 0' },
  loadMoreBar: { display: 'flex', gap: 8, alignSelf: 'flex-start' },
  slackLinkRow: { display: 'flex', justifyContent: 'flex-end', paddingTop: 4 },
  slackLink: { display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--accent)', textDecoration: 'none', padding: '4px 10px', borderRadius: 6, border: '1px solid color-mix(in oklch, var(--accent) 25%, var(--line))', background: 'color-mix(in oklch, var(--accent) 6%, transparent)' },
  inline: { display: 'inline-flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' },
}

export default function ActionActivity({ workspaceId, actionId, pageSize = 20, slackMessageTS, slackChannelID, slackChannelURL }: ActionActivityProps) {
  const slackPermalink = buildSlackPermalink(slackChannelURL, slackChannelID, slackMessageTS)
  const { t } = useTranslation()
  const actionStatuses = useActionStatuses(workspaceId)
  const statusLabel = (id: string) => actionStatuses.label(id)
  const statusColor = (id: string) => {
    const def = actionStatuses.get(id)
    return (actionStatusColorStyle(def?.color).background as string) ?? 'var(--text-muted)'
  }
  const [tab, setTab] = useState<Tab>('all')

  const messagesQuery = useQuery<MessagesData>(GET_ACTION_MESSAGES, {
    variables: { workspaceId, id: actionId, limit: pageSize, cursor: null },
    fetchPolicy: 'cache-and-network',
  })
  const eventsQuery = useQuery<EventsData>(GET_ACTION_EVENTS, {
    variables: { workspaceId, id: actionId, limit: pageSize, cursor: null },
    fetchPolicy: 'cache-and-network',
  })
  const usersQuery = useQuery<{ slackUsers: SlackUser[] }>(GET_SLACK_USERS, {
    fetchPolicy: 'cache-first',
  })

  const userIndex: UserIndex = useMemo(() => {
    const byName = new Map<string, string>()
    const byImage = new Map<string, string>()
    const byInitial = new Map<string, string>()
    for (const u of usersQuery.data?.slackUsers ?? []) {
      const display = u.realName || u.name || u.id
      byName.set(u.id, display)
      byInitial.set(u.id, initialOf(display))
      if (u.imageUrl) byImage.set(u.id, u.imageUrl)
    }
    return { byName, byImage, byInitial }
  }, [usersQuery.data])

  const messages = messagesQuery.data?.action?.messages.items ?? []
  const events = eventsQuery.data?.action?.events.items ?? []
  const messagesCursor = messagesQuery.data?.action?.messages.nextCursor ?? ''
  const eventsCursor = eventsQuery.data?.action?.events.nextCursor ?? ''

  const messageCount = messages.length
  const eventCount = events.length
  const totalCount = messageCount + eventCount

  const visible = useMemo(() => {
    type Item =
      | { kind: 'message'; createdAt: string; data: SlackMessage }
      | { kind: 'event'; createdAt: string; data: ActionEvent }
    const items: Item[] = []
    if (tab !== 'history') {
      for (const m of messages) items.push({ kind: 'message', createdAt: m.createdAt, data: m })
    }
    if (tab !== 'comments') {
      for (const e of events) items.push({ kind: 'event', createdAt: e.createdAt, data: e })
    }
    // Newest first.
    items.sort((a, b) => (a.createdAt < b.createdAt ? 1 : a.createdAt > b.createdAt ? -1 : 0))
    return items
  }, [messages, events, tab])

  const loadingInitial = (messagesQuery.loading && messages.length === 0) || (eventsQuery.loading && events.length === 0)

  return (
    <div style={styles.root} data-testid="action-activity">
      <div style={styles.header}>
        <div style={styles.title}>
          <span>{t('sectionActivity')}</span>
          <span style={styles.count}>{t('activityCount', { count: totalCount })}</span>
        </div>
        <div style={styles.tabs} role="tablist" aria-label="activity tabs">
          <TabButton label={t('activityTabAll')} active={tab === 'all'} onClick={() => setTab('all')} testId="activity-tab-all" />
          <TabButton label={t('activityTabComments')} active={tab === 'comments'} onClick={() => setTab('comments')} testId="activity-tab-comments" />
          <TabButton label={t('activityTabHistory')} active={tab === 'history'} onClick={() => setTab('history')} testId="activity-tab-history" />
        </div>
      </div>

      {loadingInitial && visible.length === 0 ? (
        <div style={styles.empty}>{t('loading')}</div>
      ) : visible.length === 0 ? (
        <div style={styles.empty}>
          {tab === 'comments' ? t('activityEmptyComments') : tab === 'history' ? t('activityEmptyHistory') : t('activityEmptyAll')}
        </div>
      ) : (
        <div style={styles.feed}>
          <div style={styles.rail} aria-hidden />
          {visible.map((it) => it.kind === 'message' ? (
            <MessageRow key={`m-${it.data.id}`} message={it.data} userIndex={userIndex} t={t} />
          ) : (
            <EventRow key={`e-${it.data.id}`} event={it.data} userIndex={userIndex} t={t} statusLabel={statusLabel} statusColor={statusColor} />
          ))}
        </div>
      )}

      {(tab !== 'history' && messagesCursor) || (tab !== 'comments' && eventsCursor) ? (
        <div style={styles.loadMoreBar}>
          {tab !== 'history' && messagesCursor && (
            <Button
              variant="ghost"
              onClick={() => {
                void messagesQuery.fetchMore({
                  variables: { workspaceId, id: actionId, limit: pageSize, cursor: messagesCursor },
                  updateQuery: (prev, { fetchMoreResult }) => {
                    if (!fetchMoreResult?.action || !prev.action) return prev
                    return {
                      action: {
                        ...prev.action,
                        messages: {
                          items: [...prev.action.messages.items, ...fetchMoreResult.action.messages.items],
                          nextCursor: fetchMoreResult.action.messages.nextCursor,
                        },
                      },
                    }
                  },
                })
              }}
              data-testid="activity-load-more-messages"
            >
              {t('messagesLoadMore')}
            </Button>
          )}
          {tab !== 'comments' && eventsCursor && (
            <Button
              variant="ghost"
              onClick={() => {
                void eventsQuery.fetchMore({
                  variables: { workspaceId, id: actionId, limit: pageSize, cursor: eventsCursor },
                  updateQuery: (prev, { fetchMoreResult }) => {
                    if (!fetchMoreResult?.action || !prev.action) return prev
                    return {
                      action: {
                        ...prev.action,
                        events: {
                          items: [...prev.action.events.items, ...fetchMoreResult.action.events.items],
                          nextCursor: fetchMoreResult.action.events.nextCursor,
                        },
                      },
                    }
                  },
                })
              }}
              data-testid="activity-load-older-events"
            >
              {t('activityLoadOlder')}
            </Button>
          )}
        </div>
      ) : null}

      {slackPermalink && (
        <div style={styles.slackLinkRow}>
          <a
            href={slackPermalink}
            target="_blank"
            rel="noreferrer noopener"
            style={styles.slackLink}
            data-testid="activity-slack-link"
          >
            <span aria-hidden>💬</span>
            <span>{t('activityOpenInSlack')}</span>
          </a>
        </div>
      )}
    </div>
  )
}

function TabButton({ label, active, onClick, testId }: { label: string; active: boolean; onClick: () => void; testId: string }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      data-testid={testId}
      style={{ ...styles.tab, ...(active ? styles.tabActive : {}) }}
    >
      {label}
    </button>
  )
}

function Avatar({ name, image }: { name: string; image: string | undefined }) {
  return (
    <div style={styles.avatar} aria-hidden>
      {image ? <img src={image} alt="" style={styles.avatarImg} /> : <span>{name}</span>}
    </div>
  )
}

function MessageRow({ message, userIndex, t }: { message: SlackMessage; userIndex: UserIndex; t: (k: MsgKey, p?: Record<string, string | number>) => string }) {
  const resolved = userIndex.byName.get(message.userID)
  const displayName = resolved ?? (message.userName && message.userName !== '' ? message.userName : 'App')
  const avatar = userIndex.byImage.get(message.userID)
  const initial = userIndex.byInitial.get(message.userID) ?? initialOf(displayName)
  const body = renderText(message.text, userIndex.byName)
  return (
    <div style={styles.row}>
      <Avatar name={initial} image={avatar} />
      <div style={styles.body}>
        <div style={styles.messageHead}>
          <span style={styles.messageName}>{displayName}</span>
          <span style={styles.timestamp} title={formatTimestampFull(message.createdAt)}>
            {formatTimestamp(message.createdAt, t('activityToday'))}
          </span>
        </div>
        <div style={styles.messageCard}>{body}</div>
      </div>
    </div>
  )
}

function EventRow({ event, userIndex, t, statusLabel, statusColor }: {
  event: ActionEvent
  userIndex: UserIndex
  t: (k: MsgKey, p?: Record<string, string | number>) => string
  statusLabel: (id: string) => string
  statusColor: (id: string) => string
}) {
  const actorName = event.actorID
    ? (event.actor?.realName || event.actor?.name || userIndex.byName.get(event.actorID) || event.actorID)
    : t('activityActorSystem')

  return (
    <div style={styles.row} data-testid={`activity-event-${event.kind.toLowerCase()}`}>
      <div style={styles.eventIcon} aria-hidden>{EVENT_ICON[event.kind]}</div>
      <div style={styles.eventLine}>
        <span style={styles.eventActor}>{actorName}</span>
        <span style={styles.eventVerb}>{t(EVENT_KEY_MAP[event.kind])}</span>
        <EventDelta event={event} userIndex={userIndex} t={t} statusLabel={statusLabel} statusColor={statusColor} />
        <span style={styles.eventTime} title={formatTimestampFull(event.createdAt)}>
          {event.kind === 'CREATED'
            ? formatTimestamp(event.createdAt, t('activityToday'))
            : formatTimestampShort(event.createdAt)}
        </span>
      </div>
    </div>
  )
}

function EventDelta({ event, userIndex, t, statusLabel, statusColor }: {
  event: ActionEvent
  userIndex: UserIndex
  t: (k: MsgKey, p?: Record<string, string | number>) => string
  statusLabel: (id: string) => string
  statusColor: (id: string) => string
}) {
  switch (event.kind) {
    case 'CREATED':
      return null
    case 'TITLE_CHANGED':
      return (
        <span style={styles.inline}>
          {event.oldValue && <span style={styles.titleChip} title={event.oldValue}>{event.oldValue}</span>}
          <span style={styles.arrow} aria-hidden>{t('activityArrowTo')}</span>
          <span style={{ ...styles.titleChip, ...styles.titleChipNew }} title={event.newValue}>{event.newValue}</span>
        </span>
      )
    case 'STATUS_CHANGED':
      return (
        <span style={styles.inline}>
          {event.oldValue && <StatusPill status={event.oldValue} statusLabel={statusLabel} statusColor={statusColor} />}
          <span style={styles.arrow} aria-hidden>{t('activityArrowTo')}</span>
          {event.newValue && <StatusPill status={event.newValue} statusLabel={statusLabel} statusColor={statusColor} />}
        </span>
      )
    case 'ASSIGNEE_CHANGED':
      return (
        <span style={styles.inline}>
          {event.oldValue
            ? <UserPill userID={event.oldValue} userIndex={userIndex} />
            : <span style={styles.titleChip}>{t('activityCleared')}</span>}
          <span style={styles.arrow} aria-hidden>{t('activityArrowTo')}</span>
          {event.newValue
            ? <UserPill userID={event.newValue} userIndex={userIndex} />
            : <span style={styles.titleChip}>{t('activityCleared')}</span>}
        </span>
      )
  }
}

function StatusPill({ status, statusLabel, statusColor }: { status: string; statusLabel: (id: string) => string; statusColor: (id: string) => string }) {
  return (
    <span style={styles.statusPill}>
      <span style={{ ...styles.statusDot, background: statusColor(status) }} />
      {statusLabel(status)}
    </span>
  )
}

function UserPill({ userID, userIndex }: { userID: string; userIndex: UserIndex }) {
  const name = userIndex.byName.get(userID) ?? userID
  const image = userIndex.byImage.get(userID)
  const initial = userIndex.byInitial.get(userID) ?? initialOf(name)
  return (
    <span style={styles.userPill}>
      {image ? <img src={image} alt="" style={styles.userPillImg} /> : <span style={styles.userPillFallback}>{initial}</span>}
      <span>{name}</span>
    </span>
  )
}
