import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { ArrowLeft, ExternalLink, BookOpen } from 'lucide-react'
import Button from '../components/Button'
import { GET_KNOWLEDGE } from '../graphql/knowledge'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './KnowledgeDetail.module.css'

interface Knowledge {
  id: string
  caseID: number
  sourceID: string
  sourceURLs: string[]
  title: string
  summary: string
  sourcedAt: string
  createdAt: string
  updatedAt: string
  case?: {
    id: number
    title: string
    description: string
  }
}

export default function KnowledgeDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const { data, loading, error } = useQuery(GET_KNOWLEDGE, {
    variables: { workspaceId: currentWorkspace!.id, id: id || '' },
    skip: !id || !currentWorkspace,
  })

  const knowledge: Knowledge | undefined = data?.knowledge

  const handleBack = () => {
    navigate(`/ws/${currentWorkspace!.id}/knowledges`)
  }

  const handleCaseClick = () => {
    if (knowledge?.case) {
      navigate(`/ws/${currentWorkspace!.id}/cases/${knowledge.case.id}`)
    }
  }

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>{t('loading')}</div>
      </div>
    )
  }

  if (error || !knowledge) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {error ? `${t('errorPrefix')} ${error.message}` : t('errorKnowledgeNotFound')}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          {t('btnBackToList')}
        </Button>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button variant="ghost" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          {t('btnBack')}
        </Button>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <div className={styles.titleRow}>
            <BookOpen size={24} className={styles.icon} />
            <h1>{knowledge.title}</h1>
          </div>
          {knowledge.sourceURLs?.length > 0 && (
            <a
              href={knowledge.sourceURLs[0]}
              target="_blank"
              rel="noopener noreferrer"
              className={styles.sourceLink}
            >
              <ExternalLink size={16} />
              {t('linkViewSource')}
            </a>
          )}
        </div>

        <div className={styles.section}>
          <h2>{t('sectionSummary')}</h2>
          <p className={styles.summary}>{knowledge.summary}</p>
        </div>

        {knowledge.case && (
          <div className={styles.section}>
            <h2>{t('sectionRelatedCase')}</h2>
            <div className={styles.riskCard} onClick={handleCaseClick}>
              <h3>{knowledge.case.title}</h3>
              <p>{knowledge.case.description}</p>
            </div>
          </div>
        )}

        <div className={styles.section}>
          <h2>{t('sectionSourceInfo')}</h2>
          <div className={styles.sourceInfo}>
            <div className={styles.sourceItem}>
              <span className={styles.sourceLabel}>{t('labelSourceId')}</span>
              <span className={styles.sourceValue}>{knowledge.sourceID}</span>
            </div>
            <div className={styles.sourceItem}>
              <span className={styles.sourceLabel}>{t('labelSourcedAt')}</span>
              <span className={styles.sourceValue}>
                {new Date(knowledge.sourcedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>

        <div className={styles.metadata}>
          <div>
            <strong>{t('labelCreatedTimestamp')}</strong> {new Date(knowledge.createdAt).toLocaleString()}
          </div>
          <div>
            <strong>{t('labelUpdatedTimestamp')}</strong> {new Date(knowledge.updatedAt).toLocaleString()}
          </div>
        </div>
      </div>
    </div>
  )
}
