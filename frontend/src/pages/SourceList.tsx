import { useState } from 'react'
import { useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { Plus, Database, CheckCircle, XCircle } from 'lucide-react'
import Table from '../components/Table'
import Button from '../components/Button'
import Chip from '../components/Chip'
import SourceTypeSelector from '../components/source/SourceTypeSelector'
import NotionDBForm from '../components/source/NotionDBForm'
import { GET_SOURCES } from '../graphql/source'
import styles from './SourceList.module.css'
import type { ReactElement } from 'react'

interface NotionDBConfig {
  databaseID: string
  databaseTitle: string
  databaseURL: string
}

interface Source {
  id: string
  name: string
  sourceType: string
  description: string
  enabled: boolean
  config: NotionDBConfig | null
  createdAt: string
  updatedAt: string
}

type SourceFormStep = 'closed' | 'select-type' | 'notion-db-form'

export default function SourceList() {
  const navigate = useNavigate()
  const [formStep, setFormStep] = useState<SourceFormStep>('closed')

  const { data, loading, error } = useQuery(GET_SOURCES)

  const handleOpenForm = () => {
    setFormStep('select-type')
  }

  const handleFormClose = () => {
    setFormStep('closed')
  }

  const handleTypeSelect = (type: string) => {
    if (type === 'NOTION_DB') {
      setFormStep('notion-db-form')
    }
  }

  const handleRowClick = (source: Source) => {
    navigate(`/sources/${source.id}`)
  }

  const renderSourceType = (sourceType: string): ReactElement => {
    const typeLabels: Record<string, { label: string; icon: ReactElement }> = {
      NOTION_DB: { label: 'Notion DB', icon: <Database size={14} /> },
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
        isOpen={formStep === 'select-type'}
        onClose={handleFormClose}
        onSelect={handleTypeSelect}
      />

      <NotionDBForm
        isOpen={formStep === 'notion-db-form'}
        onClose={handleFormClose}
      />
    </div>
  )
}
