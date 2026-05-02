import { useQuery } from '@apollo/client'
import { GET_ACTION_MESSAGES } from '../graphql/action'
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

interface ActionMessagesProps {
  workspaceId: string
  actionId: number
  pageSize?: number
}

function formatTimestamp(iso: string): string {
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
      {items.map((m) => (
        <div
          key={m.id}
          style={{
            border: '1px solid var(--border-light)',
            borderRadius: 6,
            padding: 'var(--spacing-sm) var(--spacing-md-sm)',
            background: 'var(--bg-paper)',
          }}
        >
          <div className="row" style={{ gap: 8, fontSize: 12, color: 'var(--text-muted)', marginBottom: 4 }}>
            <span style={{ fontWeight: 600, color: 'var(--text-body)' }}>{m.userName || m.userID}</span>
            <span>{formatTimestamp(m.createdAt)}</span>
          </div>
          <div style={{ fontSize: 13, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>{m.text}</div>
        </div>
      ))}
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
