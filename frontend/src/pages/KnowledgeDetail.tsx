import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { ArrowLeft, ExternalLink, BookOpen } from 'lucide-react'
import Button from '../components/Button'
import { GET_KNOWLEDGE } from '../graphql/knowledge'
import styles from './KnowledgeDetail.module.css'

interface Knowledge {
  id: string
  riskID: number
  sourceID: string
  sourceURL: string
  title: string
  summary: string
  sourcedAt: string
  createdAt: string
  updatedAt: string
  risk?: {
    id: number
    name: string
    description: string
  }
}

export default function KnowledgeDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data, loading, error } = useQuery(GET_KNOWLEDGE, {
    variables: { id: id || '' },
    skip: !id,
  })

  const knowledge: Knowledge | undefined = data?.knowledge

  const handleBack = () => {
    navigate('/knowledges')
  }

  const handleRiskClick = () => {
    if (knowledge?.risk) {
      navigate(`/risks/${knowledge.risk.id}`)
    }
  }

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (error || !knowledge) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {error ? `Error: ${error.message}` : 'Knowledge not found'}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back to List
        </Button>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button variant="ghost" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back
        </Button>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <div className={styles.titleRow}>
            <BookOpen size={24} className={styles.icon} />
            <h1>{knowledge.title}</h1>
          </div>
          <a
            href={knowledge.sourceURL}
            target="_blank"
            rel="noopener noreferrer"
            className={styles.sourceLink}
          >
            <ExternalLink size={16} />
            View Source
          </a>
        </div>

        <div className={styles.section}>
          <h2>Summary</h2>
          <p className={styles.summary}>{knowledge.summary}</p>
        </div>

        {knowledge.risk && (
          <div className={styles.section}>
            <h2>Related Risk</h2>
            <div className={styles.riskCard} onClick={handleRiskClick}>
              <h3>{knowledge.risk.name}</h3>
              <p>{knowledge.risk.description}</p>
            </div>
          </div>
        )}

        <div className={styles.section}>
          <h2>Source Information</h2>
          <div className={styles.sourceInfo}>
            <div className={styles.sourceItem}>
              <span className={styles.sourceLabel}>Source ID:</span>
              <span className={styles.sourceValue}>{knowledge.sourceID}</span>
            </div>
            <div className={styles.sourceItem}>
              <span className={styles.sourceLabel}>Sourced At:</span>
              <span className={styles.sourceValue}>
                {new Date(knowledge.sourcedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>

        <div className={styles.metadata}>
          <div>
            <strong>Created:</strong> {new Date(knowledge.createdAt).toLocaleString()}
          </div>
          <div>
            <strong>Updated:</strong> {new Date(knowledge.updatedAt).toLocaleString()}
          </div>
        </div>
      </div>
    </div>
  )
}
