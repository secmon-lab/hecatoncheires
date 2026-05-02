import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import { GET_SOURCE, DELETE_SOURCE, UPDATE_SOURCE, GET_SOURCES } from '../graphql/source'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import { IconChevLeft, IconSlack, IconGitHub, IconNotion, IconExt } from '../components/Icons'
import { Badge } from '../components/Primitives'
import SourceDeleteDialog from '../components/source/SourceDeleteDialog'
import { SOURCE_TYPE } from '../constants/source'

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString()
}

function sourceIcon(type: string) {
  if (type === SOURCE_TYPE.SLACK) return <IconSlack size={22} />
  if (type === SOURCE_TYPE.GITHUB) return <IconGitHub size={22} />
  return <IconNotion size={22} />
}

export default function SourceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [showDelete, setShowDelete] = useState(false)

  const { data, loading } = useQuery(GET_SOURCE, {
    variables: { workspaceId: currentWorkspace?.id, id },
    skip: !currentWorkspace || !id,
  })

  const [updateSource] = useMutation(UPDATE_SOURCE, {
    refetchQueries: [
      { query: GET_SOURCE, variables: { workspaceId: currentWorkspace?.id, id } },
      { query: GET_SOURCES, variables: { workspaceId: currentWorkspace?.id } },
    ],
  })
  const [deleteSource] = useMutation(DELETE_SOURCE, {
    refetchQueries: [{ query: GET_SOURCES, variables: { workspaceId: currentWorkspace?.id } }],
  })

  const s = data?.source
  if (loading) return <div className="h-main-inner muted">{t('loading')}</div>
  if (!s) return <div className="h-main-inner">{t('errorPrefix')}</div>

  const toggleEnabled = async () => {
    await updateSource({ variables: { workspaceId: currentWorkspace!.id, input: { id: s.id, enabled: !s.enabled } } })
  }

  return (
    <div className="h-main-inner" style={{ maxWidth: 900 }}>
      <div className="row" style={{ marginBottom: 12 }}>
        <Button variant="ghost" size="sm" icon={<IconChevLeft size={13} />} onClick={() => navigate(`/ws/${currentWorkspace!.id}/sources`)}>
          {t('btnBack')}
        </Button>
        <span className="spacer" />
        <Button size="sm" onClick={toggleEnabled}>{s.enabled ? t('statusEnabled') : t('statusDisabled')}</Button>
        <Button size="sm" variant="danger" onClick={() => setShowDelete(true)}>{t('btnDelete')}</Button>
      </div>

      <div className="card" style={{ padding: 24 }}>
        <div className="row" style={{ gap: 12, marginBottom: 4 }}>
          <div style={{ width: 44, height: 44, borderRadius: 10, background: 'var(--bg-sunken)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            {sourceIcon(s.sourceType)}
          </div>
          <div style={{ flex: 1 }}>
            <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>{s.name}</h1>
            <div className="soft" style={{ fontSize: 12, marginTop: 2 }}>
              {s.sourceType.replace('_', ' ')}{s.description ? ` · ${s.description}` : ''}
            </div>
          </div>
          {s.enabled ? <Badge kind="open">{t('statusEnabled')}</Badge> : <Badge>{t('statusDisabled')}</Badge>}
        </div>
        <hr />
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12, marginBottom: 16 }}>
          <div><div className="field-label">{t('headerCreated')}</div><div className="mono">{formatDate(s.createdAt)}</div></div>
          <div><div className="field-label">{t('labelUpdated')}</div><div className="mono">{formatDate(s.updatedAt)}</div></div>
          <div><div className="field-label">{t('labelType')}</div><div>{s.sourceType.replace('_', ' ')}</div></div>
        </div>

        {s.sourceType === SOURCE_TYPE.SLACK && s.config?.channels && (
          <>
            <div className="field-label">Channels ({s.config.channels.length})</div>
            <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
              {s.config.channels.map((c: any) => (
                <span key={c.id} className="chip"><IconSlack size={11} />#{c.name}</span>
              ))}
            </div>
          </>
        )}

        {s.sourceType === SOURCE_TYPE.GITHUB && s.config?.repositories && (
          <>
            <div className="field-label">Repositories ({s.config.repositories.length})</div>
            <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
              {s.config.repositories.map((r: any) => (
                <span key={`${r.owner}/${r.repo}`} className="chip"><IconGitHub size={11} />{r.owner}/{r.repo}</span>
              ))}
            </div>
          </>
        )}

        {(s.sourceType === SOURCE_TYPE.NOTION_DB || s.sourceType === SOURCE_TYPE.NOTION_PAGE) && s.config && (
          <>
            <div className="field-label">{t('linkViewSource')}</div>
            <a
              href={s.config.databaseURL || s.config.pageURL || '#'}
              target="_blank"
              rel="noreferrer noopener"
              style={{ color: 'var(--accent)', display: 'inline-flex', alignItems: 'center', gap: 4 }}
            >
              {s.config.databaseTitle || s.config.pageTitle || s.config.databaseID || s.config.pageID}
              <IconExt size={11} />
            </a>
          </>
        )}
      </div>

      <SourceDeleteDialog
        isOpen={showDelete}
        onClose={() => setShowDelete(false)}
        sourceId={s.id}
        sourceName={s.name}
        onConfirm={async () => {
          await deleteSource({ variables: { workspaceId: currentWorkspace!.id, id: s.id } })
          setShowDelete(false)
          navigate(`/ws/${currentWorkspace!.id}/sources`)
        }}
      />
    </div>
  )
}
