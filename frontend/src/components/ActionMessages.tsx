import { useMemo } from 'react'
import { useQuery } from '@apollo/client'
import { GET_ACTION_MESSAGES } from '../graphql/action'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useTranslation } from '../i18n'
import Button from './Button'

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

interface ActionMessagesData {
  action: {
    id: number
    messages: {
      items: SlackMessage[]
      nextCursor: string
    }
  } | null
}

interface SlackUser {
  id: string
  name: string
  realName: string
  imageUrl: string
}

interface ActionMessagesProps {
  workspaceId: string
  actionId: number
  pageSize?: number
}

// Minimal Slack-emoji → unicode shortcuts for the change-notification
// messages this app posts. Anything we don't recognise is rendered as a
// neutral bullet so the raw `:foo:` shortcode doesn't leak into the UI.
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
  return text.replace(/:([a-z0-9_+-]+):/gi, (_m, name) => {
    const hit = EMOJI_MAP[name.toLowerCase()]
    return hit ?? '•'
  })
}

function renderText(text: string, userIdToName: Map<string, string>): string {
  let out = renderEmojiTokens(text)
  // <@U123> or <@U123|name>  → @resolved-name (fall back to ID)
  out = out.replace(/<@([A-Z0-9]+)(\|[^>]+)?>/g, (_m, uid: string) => {
    const display = userIdToName.get(uid) ?? uid
    return `@${display}`
  })
  // <https://... |label> → label, otherwise URL
  out = out.replace(/<([^|>]+)\|([^>]+)>/g, (_m, _url, label) => label)
  out = out.replace(/<([^|>]+)>/g, (_m, url) => url)
  // &gt; / &lt; / &amp; introduced by Slack's escaping
  out = out.replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&amp;/g, '&')
  return out
}

function formatTimestampShort(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const sameYear = d.getFullYear() === now.getFullYear()
  const sameDay =
    sameYear &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  if (sameDay) return `${hh}:${mi}`
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  if (sameYear) return `${mm}/${dd} ${hh}:${mi}`
  return `${d.getFullYear()}/${mm}/${dd}`
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

export default function ActionMessages({ workspaceId, actionId, pageSize = 20 }: ActionMessagesProps) {
  const { t } = useTranslation()
  const { data, loading, fetchMore } = useQuery<ActionMessagesData>(GET_ACTION_MESSAGES, {
    variables: { workspaceId, id: actionId, limit: pageSize, cursor: null },
    fetchPolicy: 'cache-and-network',
  })
  const usersQuery = useQuery<{ slackUsers: SlackUser[] }>(GET_SLACK_USERS, {
    fetchPolicy: 'cache-first',
  })

  const userIdToName = useMemo(() => {
    const map = new Map<string, string>()
    for (const u of usersQuery.data?.slackUsers ?? []) {
      map.set(u.id, u.realName || u.name || u.id)
    }
    return map
  }, [usersQuery.data])

  const userIdToImage = useMemo(() => {
    const map = new Map<string, string>()
    for (const u of usersQuery.data?.slackUsers ?? []) {
      if (u.imageUrl) map.set(u.id, u.imageUrl)
    }
    return map
  }, [usersQuery.data])

  const items = data?.action?.messages.items ?? []
  const nextCursor = data?.action?.messages.nextCursor ?? ''

  if (loading && items.length === 0) {
    return <div className="muted" style={{ fontSize: 12 }}>{t('loading')}</div>
  }

  if (items.length === 0) {
    return <div className="muted" style={{ fontSize: 12 }}>{t('emptyMessages')}</div>
  }

  return (
    <div data-testid="action-messages" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--spacing-sm)' }}>
      {items.map((m) => {
        const displayName = userIdToName.get(m.userID) ?? m.userName ?? m.userID
        const avatar = userIdToImage.get(m.userID)
        const body = renderText(m.text, userIdToName)
        return (
          <div
            key={m.id}
            style={{
              border: '1px solid var(--border-light)',
              borderRadius: 6,
              padding: 'var(--spacing-sm) var(--spacing-md-sm)',
              background: 'var(--bg-paper)',
            }}
          >
            <div className="row" style={{ gap: 8, fontSize: 12, color: 'var(--text-muted)', marginBottom: 4, alignItems: 'center' }}>
              {avatar && (
                <img
                  src={avatar}
                  alt=""
                  width={20}
                  height={20}
                  style={{ borderRadius: '50%' }}
                />
              )}
              <span style={{ fontWeight: 600, color: 'var(--text-body)' }}>{displayName}</span>
              <span title={formatTimestampFull(m.createdAt)}>{formatTimestampShort(m.createdAt)}</span>
            </div>
            <div style={{ fontSize: 13, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>{body}</div>
          </div>
        )
      })}
      {nextCursor && (
        <div style={{ alignSelf: 'flex-start' }}>
          <Button
            variant="ghost"
            onClick={() => {
              void fetchMore({
                variables: { workspaceId, id: actionId, limit: pageSize, cursor: nextCursor },
                updateQuery: (prev, { fetchMoreResult }) => {
                  if (!fetchMoreResult?.action) return prev
                  if (!prev.action) return fetchMoreResult
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
            data-testid="action-messages-load-more"
          >
            {t('messagesLoadMore')}
          </Button>
        </div>
      )}
    </div>
  )
}
