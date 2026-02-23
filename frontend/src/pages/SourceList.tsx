import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import { Plus, Database, FileText, GitBranch, MessageSquare, CheckCircle, XCircle } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import SourceTypeSelector from '../components/source/SourceTypeSelector'
import NotionDBForm from '../components/source/NotionDBForm'
import NotionPageForm from '../components/source/NotionPageForm'
import SlackForm from '../components/source/SlackForm'
import GitHubForm from '../components/source/GitHubForm'
import { GET_SOURCES } from '../graphql/source'
import { SOURCE_TYPE, FORM_STEP, type FormStep } from '../constants/source'
import styles from './SourceList.module.css'
import type { ReactElement } from 'react'

interface NotionDBConfig {
  __typename: 'NotionDBConfig'
  databaseID: string
  databaseTitle: string
  databaseURL: string
}

interface NotionPageConfig {
  __typename: 'NotionPageConfig'
  pageID: string
  pageTitle: string
  pageURL: string
  recursive: boolean
  maxDepth: number
}

interface SlackChannel {
  id: string
  name: string
}

interface SlackConfig {
  __typename: 'SlackConfig'
  channels: SlackChannel[]
}

interface GitHubRepository {
  owner: string
  repo: string
}

interface GitHubConfig {
  __typename: 'GitHubConfig'
  repositories: GitHubRepository[]
}

type SourceConfig = NotionDBConfig | NotionPageConfig | SlackConfig | GitHubConfig | null

interface Source {
  id: string
  name: string
  sourceType: string
  description: string
  enabled: boolean
  config: SourceConfig
  createdAt: string
  updatedAt: string
}

export default function SourceList() {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [formStep, setFormStep] = useState<FormStep>(FORM_STEP.CLOSED)

  const { data, loading, error } = useQuery(GET_SOURCES, {
    variables: { workspaceId: currentWorkspace!.id },
    skip: !currentWorkspace,
  })

  const handleOpenForm = () => {
    setFormStep(FORM_STEP.SELECT_TYPE)
  }

  const handleFormClose = () => {
    setFormStep(FORM_STEP.CLOSED)
  }

  const handleTypeSelect = (type: string) => {
    if (type === SOURCE_TYPE.NOTION_DB) {
      setFormStep(FORM_STEP.NOTION_DB_FORM)
    } else if (type === SOURCE_TYPE.NOTION_PAGE) {
      setFormStep(FORM_STEP.NOTION_PAGE_FORM)
    } else if (type === SOURCE_TYPE.SLACK) {
      setFormStep(FORM_STEP.SLACK_FORM)
    } else if (type === SOURCE_TYPE.GITHUB) {
      setFormStep(FORM_STEP.GITHUB_FORM)
    }
  }

  const handleRowClick = (source: Source) => {
    navigate(`/ws/${currentWorkspace!.id}/sources/${source.id}`)
  }

  const renderSourceType = (sourceType: string): ReactElement => {
    const typeLabels: Record<string, { label: string; icon: ReactElement }> = {
      [SOURCE_TYPE.NOTION_DB]: { label: 'Notion DB', icon: <Database size={14} /> },
      [SOURCE_TYPE.NOTION_PAGE]: { label: 'Notion Page', icon: <FileText size={14} /> },
      [SOURCE_TYPE.SLACK]: { label: 'Slack', icon: <MessageSquare size={14} /> },
      [SOURCE_TYPE.GITHUB]: { label: 'GitHub', icon: <GitBranch size={14} /> },
    }
    const typeInfo = typeLabels[sourceType] || { label: sourceType, icon: null }

    return (
      <div className={styles.sourceType}>
        {typeInfo.icon}
        <span>{typeInfo.label}</span>
      </div>
    )
  }

  const renderEnabled = (enabled: boolean): ReactElement => {
    return enabled ? (
      <Chip variant="status" colorIndex={0}>
        <CheckCircle size={12} />
        <span>Enabled</span>
      </Chip>
    ) : (
      <Chip variant="status" colorIndex={4}>
        <XCircle size={12} />
        <span>Disabled</span>
      </Chip>
    )
  }

  const columns = [
    {
      header: 'Name',
      accessor: 'name' as keyof Source,
      width: '200px',
    },
    {
      header: 'Type',
      accessor: ((source: Source) => renderSourceType(source.sourceType)) as (row: Source) => ReactElement,
      width: '120px',
    },
    {
      header: 'Description',
      accessor: 'description' as keyof Source,
    },
    {
      header: 'Status',
      accessor: ((source: Source) => renderEnabled(source.enabled)) as (row: Source) => ReactElement,
      width: '100px',
    },
    {
      header: 'Created',
      accessor: ((source: Source) => new Date(source.createdAt).toLocaleDateString()) as (row: Source) => string,
      width: '120px',
    },
  ]

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>Error: {error.message}</div>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div>
          <h2 className={styles.title}>Sources</h2>
          <p className={styles.subtitle}>Manage external data sources for risk monitoring</p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={20} />}
          onClick={handleOpenForm}
        >
          Add Source
        </Button>
      </div>

      <div className={styles.tableWrapper}>
        <Table columns={columns} data={data?.sources || []} onRowClick={handleRowClick} />
      </div>

      <SourceTypeSelector
        isOpen={formStep === FORM_STEP.SELECT_TYPE}
        onClose={handleFormClose}
        onSelect={handleTypeSelect}
      />

      <NotionDBForm
        isOpen={formStep === FORM_STEP.NOTION_DB_FORM}
        onClose={handleFormClose}
      />

      <NotionPageForm
        isOpen={formStep === FORM_STEP.NOTION_PAGE_FORM}
        onClose={handleFormClose}
      />

      <SlackForm
        isOpen={formStep === FORM_STEP.SLACK_FORM}
        onClose={handleFormClose}
      />

      <GitHubForm
        isOpen={formStep === FORM_STEP.GITHUB_FORM}
        onClose={handleFormClose}
      />
    </div>
  )
}
