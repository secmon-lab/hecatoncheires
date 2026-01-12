import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus, Database, MessageSquare, CheckCircle, XCircle } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import SourceTypeSelector from '../components/source/SourceTypeSelector'
import NotionDBForm from '../components/source/NotionDBForm'
import SlackForm from '../components/source/SlackForm'
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

interface SlackChannel {
  id: string
  name: string
}

interface SlackConfig {
  __typename: 'SlackConfig'
  channels: SlackChannel[]
}

type SourceConfig = NotionDBConfig | SlackConfig | null

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
  const [formStep, setFormStep] = useState<FormStep>(FORM_STEP.CLOSED)

  const { data, loading, error } = useQuery(GET_SOURCES)

  const handleOpenForm = () => {
    setFormStep(FORM_STEP.SELECT_TYPE)
  }

  const handleFormClose = () => {
    setFormStep(FORM_STEP.CLOSED)
  }

  const handleTypeSelect = (type: string) => {
    if (type === SOURCE_TYPE.NOTION_DB) {
      setFormStep(FORM_STEP.NOTION_DB_FORM)
    } else if (type === SOURCE_TYPE.SLACK) {
      setFormStep(FORM_STEP.SLACK_FORM)
    }
  }

  const handleRowClick = (source: Source) => {
    navigate(`/sources/${source.id}`)
  }

  const renderSourceType = (sourceType: string): ReactElement => {
    const typeLabels: Record<string, { label: string; icon: ReactElement }> = {
      [SOURCE_TYPE.NOTION_DB]: { label: 'Notion DB', icon: <Database size={14} /> },
      [SOURCE_TYPE.SLACK]: { label: 'Slack', icon: <MessageSquare size={14} /> },
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

      <SlackForm
        isOpen={formStep === FORM_STEP.SLACK_FORM}
        onClose={handleFormClose}
      />
    </div>
  )
}
