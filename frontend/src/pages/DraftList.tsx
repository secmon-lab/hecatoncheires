import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { GET_DRAFTS } from '../graphql/drafts'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'

interface DraftRow {
  id: number
  title: string
  description: string | null
  status: 'DRAFT' | 'OPEN' | 'CLOSED'
  isPrivate: boolean
  reporterID: string
  createdAt: string
  updatedAt: string
}

function formatDate(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${yyyy}/${mm}/${dd} ${hh}:${mi}`
}

export default function DraftList() {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  const { data, loading } = useQuery(GET_DRAFTS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
    fetchPolicy: 'cache-and-network',
  })

  const drafts: DraftRow[] = data?.drafts ?? []

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('draftsPageTitle')}</h1>
          <div className="sub">{t('draftsPageSubtitle')}</div>
        </div>
      </div>

      <div className="card" style={{ padding: 0 }}>
        {loading && drafts.length === 0 ? (
          <div className="muted" style={{ padding: 24, textAlign: 'center' }}>{t('noDataAvailable')}</div>
        ) : drafts.length === 0 ? (
          <div className="muted" style={{ padding: 24, textAlign: 'center' }}>{t('draftsEmpty')}</div>
        ) : (
          <table className="h-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={{ textAlign: 'left', padding: '12px 16px' }}>{t('draftsColumnTitle')}</th>
                <th style={{ textAlign: 'left', padding: '12px 16px', width: 200 }}>{t('draftsColumnCreated')}</th>
              </tr>
            </thead>
            <tbody>
              {drafts.map((d) => (
                <tr
                  key={d.id}
                  onClick={() => navigate(`/ws/${currentWorkspace?.id}/drafts/${d.id}`)}
                  style={{ cursor: 'pointer', borderTop: '1px solid var(--border-default)' }}
                >
                  <td style={{ padding: '12px 16px' }}>
                    <div style={{ fontWeight: 500 }}>
                      {d.title || <span className="soft">{t('draftsUntitled')}</span>}
                    </div>
                    {d.description ? (
                      <div className="soft" style={{ marginTop: 4, fontSize: 12 }}>
                        {d.description.length > 120 ? `${d.description.slice(0, 120)}…` : d.description}
                      </div>
                    ) : null}
                  </td>
                  <td style={{ padding: '12px 16px', fontSize: 12 }}>{formatDate(d.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
