import { useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { GET_KNOWLEDGE } from '../graphql/knowledge'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import { IconChevLeft, IconExt } from '../components/Icons'

function formatDate(iso?: string | null) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString()
}

export default function KnowledgeDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const { data, loading } = useQuery(GET_KNOWLEDGE, {
    variables: { workspaceId: currentWorkspace?.id, id },
    skip: !currentWorkspace || !id,
  })

  const k = data?.knowledge
  if (loading) return <div className="h-main-inner muted">{t('loading')}</div>
  if (!k) return <div className="h-main-inner">{t('errorPrefix')}</div>

  return (
    <div className="h-main-inner" data-testid="knowledge-content" style={{ maxWidth: 900 }}>
      <div className="row" style={{ marginBottom: 12 }}>
        <Button variant="ghost" size="sm" icon={<IconChevLeft size={13} />} onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledges`)}>
          {t('btnBack')}
        </Button>
      </div>

      <div className="card" style={{ padding: 24 }}>
        <h1 style={{ margin: 0, fontSize: 22, fontWeight: 600 }}>{k.title}</h1>
        <div className="soft" style={{ fontSize: 12, marginTop: 6 }}>
          {formatDate(k.sourcedAt || k.createdAt)}
          {k.case && (
            <>
              {' · '}
              <a className="slack-link" href={`/ws/${currentWorkspace!.id}/cases/${k.case.id}`}>
                #{k.case.id} {k.case.title}
                <IconExt size={10} />
              </a>
            </>
          )}
        </div>
        <hr />
        {k.summary && (
          <>
            <div className="field-label">{t('labelDescription')}</div>
            <p style={{ fontSize: 13, lineHeight: 1.6, margin: '0 0 16px 0' }}>{k.summary}</p>
          </>
        )}
        {k.sourceURLs && k.sourceURLs.length > 0 && (
          <>
            <div className="field-label">{t('linkViewSource')}</div>
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {k.sourceURLs.map((url: string) => (
                <li key={url}>
                  <a className="slack-link" href={url} target="_blank" rel="noreferrer noopener">
                    {url}
                    <IconExt size={10} />
                  </a>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </div>
  )
}
