import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { GET_SOURCES } from '../graphql/source'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import { IconPlus, IconSlack, IconGitHub, IconNotion, IconDots } from '../components/Icons'
import { Badge } from '../components/Primitives'
import { FORM_STEP, SOURCE_TYPE, type FormStep } from '../constants/source'
import SourceTypeSelector from '../components/source/SourceTypeSelector'
import NotionDBForm from '../components/source/NotionDBForm'
import NotionPageForm from '../components/source/NotionPageForm'
import SlackForm from '../components/source/SlackForm'
import GitHubForm from '../components/source/GitHubForm'

interface SourceRow {
  id: string
  name: string
  sourceType: string
  description?: string | null
  enabled: boolean
  createdAt: string
  updatedAt: string
  config?: any
}

function sourceIcon(type: string) {
  if (type === SOURCE_TYPE.SLACK) return <IconSlack size={16} />
  if (type === SOURCE_TYPE.GITHUB) return <IconGitHub size={16} />
  return <IconNotion size={16} />
}

function sourceTypeLabel(type: string) {
  switch (type) {
    case SOURCE_TYPE.SLACK: return 'Slack'
    case SOURCE_TYPE.GITHUB: return 'GitHub'
    case SOURCE_TYPE.NOTION_DB: return 'Notion DB'
    case SOURCE_TYPE.NOTION_PAGE: return 'Notion Page'
    default: return type
  }
}

function scopeText(s: SourceRow) {
  if (!s.config) return ''
  if (s.sourceType === SOURCE_TYPE.SLACK) {
    const channels = s.config.channels?.map((c: any) => `#${c.name}`).join(', ')
    return channels || ''
  }
  if (s.sourceType === SOURCE_TYPE.GITHUB) {
    const repos = s.config.repositories?.map((r: any) => `${r.owner}/${r.repo}`).join(', ')
    return repos || ''
  }
  if (s.sourceType === SOURCE_TYPE.NOTION_DB) return s.config.databaseTitle || s.config.databaseID || ''
  if (s.sourceType === SOURCE_TYPE.NOTION_PAGE) return s.config.pageTitle || s.config.pageID || ''
  return ''
}

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}/${mm}/${dd}`
}

export default function SourceList() {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [step, setStep] = useState<FormStep>(FORM_STEP.CLOSED)

  const { data } = useQuery(GET_SOURCES, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const sources: SourceRow[] = data?.sources || []

  const handleSelectType = (type: string) => {
    if (type === SOURCE_TYPE.NOTION_DB)   setStep(FORM_STEP.NOTION_DB_FORM)
    else if (type === SOURCE_TYPE.NOTION_PAGE) setStep(FORM_STEP.NOTION_PAGE_FORM)
    else if (type === SOURCE_TYPE.SLACK)  setStep(FORM_STEP.SLACK_FORM)
    else if (type === SOURCE_TYPE.GITHUB) setStep(FORM_STEP.GITHUB_FORM)
  }
  const close = () => setStep(FORM_STEP.CLOSED)

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('titleSources')}</h1>
          <div className="sub">{t('subtitleSources')}</div>
        </div>
        <div className="actions">
          <Button variant="primary" icon={<IconPlus size={14} />} onClick={() => setStep(FORM_STEP.SELECT_TYPE)}>
            New Source
          </Button>
        </div>
      </div>

      <div className="card" style={{ overflow: 'hidden' }}>
        <table className="h-table">
          <thead>
            <tr>
              <th style={{ width: 36 }}></th>
              <th>{t('headerName')}</th>
              <th style={{ width: 120 }}>{t('labelType')}</th>
              <th style={{ width: 110 }}>{t('labelStatus')}</th>
              <th style={{ width: 130 }}>{t('headerCreated')}</th>
              <th style={{ width: 38 }}></th>
            </tr>
          </thead>
          <tbody>
            {sources.length === 0 && (
              <tr>
                <td colSpan={6} style={{ padding: 32, textAlign: 'center', color: 'var(--fg-soft)' }}>
                  {t('noDataAvailable')}
                </td>
              </tr>
            )}
            {sources.map((s) => (
              <tr
                key={s.id}
                onClick={() => navigate(`/ws/${currentWorkspace!.id}/sources/${s.id}`)}
                style={{ cursor: 'pointer' }}
              >
                <td>{sourceIcon(s.sourceType)}</td>
                <td>
                  <div style={{ fontWeight: 500 }}>{s.name}</div>
                  {scopeText(s) && (
                    <div className="soft" style={{ fontSize: 11.5, marginTop: 2 }}>{scopeText(s)}</div>
                  )}
                </td>
                <td><Badge>{sourceTypeLabel(s.sourceType)}</Badge></td>
                <td>{s.enabled ? <Badge kind="open">{t('statusEnabled')}</Badge> : <Badge>{t('statusDisabled')}</Badge>}</td>
                <td className="mono soft" style={{ fontSize: 12 }}>{formatDate(s.createdAt)}</td>
                <td>
                  <button
                    className="h-icon-btn"
                    style={{ width: 24, height: 24 }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <IconDots size={14} />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <SourceTypeSelector isOpen={step === FORM_STEP.SELECT_TYPE} onClose={close} onSelect={handleSelectType} />
      <NotionDBForm   isOpen={step === FORM_STEP.NOTION_DB_FORM}   onClose={close} />
      <NotionPageForm isOpen={step === FORM_STEP.NOTION_PAGE_FORM} onClose={close} />
      <SlackForm      isOpen={step === FORM_STEP.SLACK_FORM}       onClose={close} />
      <GitHubForm     isOpen={step === FORM_STEP.GITHUB_FORM}      onClose={close} />
    </div>
  )
}
