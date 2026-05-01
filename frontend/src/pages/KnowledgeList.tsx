import { useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { GET_KNOWLEDGES } from '../graphql/knowledge'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { IconSearch } from '../components/Icons'

interface Knowledge {
  id: string
  caseID?: number | null
  title: string
  summary?: string | null
  sourcedAt?: string | null
  createdAt: string
  updatedAt: string
  case?: { id: number; title: string } | null
}

function formatDate(iso?: string | null) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}/${mm}/${dd}`
}

export default function KnowledgeList() {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  const { data } = useQuery(GET_KNOWLEDGES, {
    variables: { workspaceId: currentWorkspace?.id, limit: 200, offset: 0 },
    skip: !currentWorkspace,
  })

  const items: Knowledge[] = data?.knowledges?.items || []
  const total: number = data?.knowledges?.totalCount ?? items.length

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>Knowledge</h1>
          <div className="sub">{total} entries</div>
        </div>
        <div className="actions">
          <div className="h-search" style={{ marginLeft: 0, width: 240 }}>
            <IconSearch size={13} />
            <span>{t('placeholderSearch')}</span>
          </div>
        </div>
      </div>

      <div className="card" style={{ overflow: 'hidden' }}>
        <table className="h-table">
          <thead>
            <tr>
              <th>{t('headerTitle')}</th>
              <th style={{ width: 200 }}>{t('navCases')}</th>
              <th style={{ width: 120 }}>{t('headerCreated')}</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={3} style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}>
                  {t('noDataAvailable')}
                </td>
              </tr>
            )}
            {items.map((k) => (
              <tr
                key={k.id}
                onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledges/${k.id}`)}
                style={{ cursor: 'pointer' }}
              >
                <td>
                  <div style={{ fontWeight: 500 }}>{k.title}</div>
                  {k.summary && (
                    <div className="soft" style={{ fontSize: 11.5, marginTop: 2, maxWidth: 540 }}>
                      {k.summary}
                    </div>
                  )}
                </td>
                <td className="mono soft" style={{ fontSize: 12 }}>
                  {k.case ? `#${k.case.id} ${k.case.title}` : '—'}
                </td>
                <td className="mono soft" style={{ fontSize: 12 }}>{formatDate(k.createdAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
