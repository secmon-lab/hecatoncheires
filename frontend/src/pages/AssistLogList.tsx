import { useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { GET_ASSIST_LOGS } from '../graphql/assistLog'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'

interface AssistLog {
  id: string
  caseId: number
  summary?: string | null
  actions?: string | null
  reasoning?: string | null
  nextSteps?: string | null
  createdAt: string
}

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

export default function AssistLogList() {
  const { id } = useParams<{ id: string }>()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const caseId = id ? parseInt(id, 10) : 0
  const { data } = useQuery(GET_ASSIST_LOGS, {
    variables: { workspaceId: currentWorkspace?.id, caseId, limit: 100, offset: 0 },
    skip: !currentWorkspace || !caseId,
  })

  const items: AssistLog[] = data?.assistLogs?.items || []

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>Assist Logs</h1>
          <div className="sub">#{caseId}</div>
        </div>
      </div>
      <div className="card" style={{ padding: 16 }}>
        {items.length === 0 ? (
          <div className="muted" style={{ padding: 24, textAlign: 'center' }}>{t('noDataAvailable')}</div>
        ) : (
          <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
            {items.map((log) => (
              <li key={log.id} style={{ borderBottom: '1px solid var(--line)', padding: '12px 0' }}>
                <div className="mono soft" style={{ fontSize: 11 }}>{formatDate(log.createdAt)}</div>
                {log.summary && <div style={{ marginTop: 4, fontSize: 13 }}>{log.summary}</div>}
                {log.reasoning && <div className="soft" style={{ marginTop: 2, fontSize: 12 }}>{log.reasoning}</div>}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
