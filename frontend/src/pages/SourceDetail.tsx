import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import { useWorkspace } from '../contexts/workspace-context'
import { ArrowLeft, Edit, MoreVertical, Trash2, Database, MessageSquare, CheckCircle, XCircle, ExternalLink, Hash } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import Modal from '../components/Modal'
import SourceDeleteDialog from '../components/source/SourceDeleteDialog'
import { GET_SOURCE, GET_SOURCES, UPDATE_SOURCE } from '../graphql/source'
import { SOURCE_TYPE } from '../constants/source'
import styles from './SourceDetail.module.css'

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

interface FormErrors {
  name?: string
}

export default function SourceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // Edit form state
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editEnabled, setEditEnabled] = useState(true)
  const [formErrors, setFormErrors] = useState<FormErrors>({})

  const { data, loading, error } = useQuery(GET_SOURCE, {
    variables: { workspaceId: currentWorkspace!.id, id },
    skip: !id || !currentWorkspace,
  })

  const [updateSource, { loading: updating }] = useMutation(UPDATE_SOURCE, {
    refetchQueries: [
      { query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } },
      { query: GET_SOURCE, variables: { workspaceId: currentWorkspace!.id, id } },
    ],
    onCompleted: () => {
      setIsEditModalOpen(false)
    },
    onError: (err) => {
      console.error('Update source error:', err)
    },
  })

  const source: Source | undefined = data?.source

  useEffect(() => {
    if (source) {
      setEditName(source.name)
      setEditDescription(source.description || '')
      setEditEnabled(source.enabled)
    }
  }, [source])

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsMenuOpen(false)
      }
    }

    if (isMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isMenuOpen])

  const handleBack = () => {
    navigate(`/ws/${currentWorkspace!.id}/sources`)
  }

  const handleEdit = () => {
    if (source) {
      setEditName(source.name)
      setEditDescription(source.description || '')
      setEditEnabled(source.enabled)
      setFormErrors({})
    }
    setIsEditModalOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate(`/ws/${currentWorkspace!.id}/sources`)
  }

  const validateEditForm = () => {
    const newErrors: FormErrors = {}

    if (!editName.trim()) {
      newErrors.name = 'Name is required'
    }

    setFormErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validateEditForm() || !source) return

    await updateSource({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          id: source.id,
          name: editName.trim(),
          description: editDescription.trim() || null,
          enabled: editEnabled,
        },
      },
    })
  }

  const renderSourceType = (sourceType: string) => {
    const typeLabels: Record<string, { label: string; icon: React.ReactNode }> = {
      [SOURCE_TYPE.NOTION_DB]: { label: 'Notion Database', icon: <Database size={20} /> },
      [SOURCE_TYPE.SLACK]: { label: 'Slack', icon: <MessageSquare size={20} /> },
    }
    const typeInfo = typeLabels[sourceType] || { label: sourceType, icon: null }

    return (
      <div className={styles.sourceTypeDisplay}>
        {typeInfo.icon}
        <span>{typeInfo.label}</span>
      </div>
    )
  }

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (error || !source) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {error ? `Error: ${error.message}` : 'Source not found'}
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
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back
        </Button>
        <div className={styles.actions}>
          <Button variant="outline" icon={<Edit size={20} />} onClick={handleEdit}>
            Edit
          </Button>
          <div style={{ position: 'relative' }} ref={menuRef}>
            <Button
              variant="outline"
              icon={<MoreVertical size={20} />}
              onClick={() => setIsMenuOpen(!isMenuOpen)}
            />
            {isMenuOpen && (
              <div className={styles.menu}>
                <button
                  className={styles.menuItem}
                  onClick={() => {
                    setIsMenuOpen(false)
                    handleDelete()
                  }}
                >
                  <Trash2 size={16} />
                  <span>Delete</span>
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <div className={styles.titleRow}>
            <h1 className={styles.title}>{source.name}</h1>
            {source.enabled ? (
              <Chip variant="status" colorIndex={0}>
                <CheckCircle size={12} />
                <span>Enabled</span>
              </Chip>
            ) : (
              <Chip variant="status" colorIndex={4}>
                <XCircle size={12} />
                <span>Disabled</span>
              </Chip>
            )}
          </div>
          {source.description && (
            <p className={styles.description}>{source.description}</p>
          )}
        </div>

        <div className={styles.sections}>
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Source Type</h3>
            {renderSourceType(source.sourceType)}
          </div>

          {source.sourceType === SOURCE_TYPE.NOTION_DB && source.config && source.config.__typename === 'NotionDBConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Notion Database</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>Database ID</span>
                  <code className={styles.configValue}>{source.config.databaseID}</code>
                </div>
                {source.config.databaseTitle && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>Database Title</span>
                    <span className={styles.configValue}>{source.config.databaseTitle}</span>
                  </div>
                )}
                {source.config.databaseURL && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>Database URL</span>
                    <a
                      href={source.config.databaseURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.configLink}
                    >
                      Open in Notion
                      <ExternalLink size={14} />
                    </a>
                  </div>
                )}
              </div>
            </div>
          )}

          {source.sourceType === SOURCE_TYPE.SLACK && source.config && source.config.__typename === 'SlackConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Slack Channels</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>Monitored Channels</span>
                  <div className={styles.channelList}>
                    {source.config.channels.length > 0 ? (
                      source.config.channels.map((channel) => (
                        <div key={channel.id} className={styles.channelTag}>
                          <Hash size={14} />
                          <span>{channel.name || channel.id}</span>
                        </div>
                      ))
                    ) : (
                      <span className={styles.configValue}>No channels configured</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          <div className={styles.metadata}>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Created</span>
              <span className={styles.metadataValue}>
                {new Date(source.createdAt).toLocaleString()}
              </span>
            </div>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>Updated</span>
              <span className={styles.metadataValue}>
                {new Date(source.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Edit Modal */}
      <Modal
        isOpen={isEditModalOpen}
        onClose={() => setIsEditModalOpen(false)}
        title="Edit Source"
        footer={
          <>
            <Button variant="outline" onClick={() => setIsEditModalOpen(false)} disabled={updating}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleEditSubmit} disabled={updating}>
              {updating ? 'Saving...' : 'Save'}
            </Button>
          </>
        }
      >
        <form onSubmit={handleEditSubmit} className={styles.form}>
          <div className={styles.field}>
            <label htmlFor="editName" className={styles.label}>
              Name *
            </label>
            <input
              id="editName"
              type="text"
              value={editName}
              onChange={(e) => setEditName(e.target.value)}
              className={`${styles.input} ${formErrors.name ? styles.inputError : ''}`}
              placeholder="Enter source name"
              disabled={updating}
            />
            {formErrors.name && <span className={styles.formError}>{formErrors.name}</span>}
          </div>

          <div className={styles.field}>
            <label htmlFor="editDescription" className={styles.label}>
              Description
            </label>
            <textarea
              id="editDescription"
              value={editDescription}
              onChange={(e) => setEditDescription(e.target.value)}
              className={styles.textarea}
              placeholder="Enter source description (optional)"
              rows={3}
              disabled={updating}
            />
          </div>

          <div className={styles.checkboxField}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={editEnabled}
                onChange={(e) => setEditEnabled(e.target.checked)}
                className={styles.checkbox}
                disabled={updating}
              />
              <span>Enable this source</span>
            </label>
          </div>
        </form>
      </Modal>

      <SourceDeleteDialog
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={handleDeleteConfirm}
        sourceId={source.id}
        sourceName={source.name}
      />
    </div>
  )
}
