import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useLazyQuery } from '@apollo/client'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { ArrowLeft, Edit, MoreVertical, Trash2, Database, FileText, GitBranch, MessageSquare, CheckCircle, XCircle, ExternalLink, Hash, Loader, X, AlertCircle } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import Chip from '../components/Chip'
import Modal from '../components/Modal'
import SourceDeleteDialog from '../components/source/SourceDeleteDialog'
import { GET_SOURCE, GET_SOURCES, UPDATE_SOURCE, UPDATE_GITHUB_SOURCE, VALIDATE_GITHUB_REPO } from '../graphql/source'
import { SOURCE_TYPE } from '../constants/source'
import styles from './SourceDetail.module.css'

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

interface FormErrors {
  name?: string
  repository?: string
}

export default function SourceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // Edit form state
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editEnabled, setEditEnabled] = useState(true)
  const [formErrors, setFormErrors] = useState<FormErrors>({})

  // GitHub edit state
  const [editRepos, setEditRepos] = useState<{ owner: string; repo: string }[]>([])
  const [repoInput, setRepoInput] = useState('')

  const { data, loading, error } = useQuery(GET_SOURCE, {
    variables: { workspaceId: currentWorkspace!.id, id },
    skip: !id || !currentWorkspace,
  })

  const refetchQueries = [
    { query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } },
    { query: GET_SOURCE, variables: { workspaceId: currentWorkspace!.id, id } },
  ]

  const [updateSource, { loading: updating }] = useMutation(UPDATE_SOURCE, {
    refetchQueries,
    onCompleted: () => {
      setIsEditModalOpen(false)
    },
    onError: (err) => {
      console.error('Update source error:', err)
    },
  })

  const [updateGitHubSource, { loading: updatingGitHub }] = useMutation(UPDATE_GITHUB_SOURCE, {
    refetchQueries,
    onCompleted: () => {
      setIsEditModalOpen(false)
    },
    onError: (err) => {
      console.error('Update GitHub source error:', err)
    },
  })

  const [validateRepo, { loading: validatingRepo }] = useLazyQuery(VALIDATE_GITHUB_REPO, {
    fetchPolicy: 'network-only',
    onCompleted: (data) => {
      const result = data.validateGitHubRepo
      if (result.valid) {
        const fullName = `${result.owner}/${result.repo}`
        if (editRepos.some((r) => `${r.owner}/${r.repo}` === fullName)) {
          setFormErrors((prev) => ({ ...prev, repository: t('errorRepoAlreadyAdded') }))
          return
        }
        setEditRepos((prev) => [...prev, { owner: result.owner, repo: result.repo }])
        setRepoInput('')
        setFormErrors((prev) => ({ ...prev, repository: undefined }))
      } else {
        setFormErrors((prev) => ({ ...prev, repository: result.errorMessage || t('errorInvalidRepo') }))
      }
    },
    onError: (error) => {
      setFormErrors((prev) => ({ ...prev, repository: error.message || t('errorValidateRepo') }))
    },
  })

  const source: Source | undefined = data?.source

  useEffect(() => {
    if (source) {
      setEditName(source.name)
      setEditDescription(source.description || '')
      setEditEnabled(source.enabled)
      if (source.sourceType === SOURCE_TYPE.GITHUB && source.config && source.config.__typename === 'GitHubConfig') {
        setEditRepos(source.config.repositories.map((r) => ({ owner: r.owner, repo: r.repo })))
      }
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
      setRepoInput('')
      if (source.sourceType === SOURCE_TYPE.GITHUB && source.config && source.config.__typename === 'GitHubConfig') {
        setEditRepos(source.config.repositories.map((r) => ({ owner: r.owner, repo: r.repo })))
      }
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
      newErrors.name = t('errorNameRequired')
    }

    setFormErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleAddRepo = () => {
    const trimmed = repoInput.trim()
    if (!trimmed) return

    validateRepo({
      variables: {
        workspaceId: currentWorkspace!.id,
        repository: trimmed,
      },
    })
  }

  const handleRepoKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleAddRepo()
    }
  }

  const handleRemoveRepo = (owner: string, repo: string) => {
    setEditRepos((prev) => prev.filter((r) => !(r.owner === owner && r.repo === repo)))
  }

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validateEditForm() || !source) return

    if (source.sourceType === SOURCE_TYPE.GITHUB) {
      await updateGitHubSource({
        variables: {
          workspaceId: currentWorkspace!.id,
          input: {
            id: source.id,
            name: editName.trim(),
            description: editDescription.trim() || null,
            repositories: editRepos.map((r) => `${r.owner}/${r.repo}`),
            enabled: editEnabled,
          },
        },
      })
    } else {
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
  }

  const renderSourceType = (sourceType: string) => {
    const typeLabels: Record<string, { label: string; icon: React.ReactNode }> = {
      [SOURCE_TYPE.NOTION_DB]: { label: t('sourceTypeNotionDB'), icon: <Database size={20} /> },
      [SOURCE_TYPE.NOTION_PAGE]: { label: t('sourceTypeNotionPage'), icon: <FileText size={20} /> },
      [SOURCE_TYPE.SLACK]: { label: t('sourceTypeSlack'), icon: <MessageSquare size={20} /> },
      [SOURCE_TYPE.GITHUB]: { label: t('sourceTypeGitHub'), icon: <GitBranch size={20} /> },
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
        <div className={styles.loading}>{t('loading')}</div>
      </div>
    )
  }

  if (error || !source) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {error ? `${t('errorPrefix')} ${error.message}` : t('errorSourceNotFound')}
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
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          {t('btnBack')}
        </Button>
        <div className={styles.actions}>
          <Button variant="outline" icon={<Edit size={20} />} onClick={handleEdit}>
            {t('btnEdit')}
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
                  <span>{t('btnDelete')}</span>
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
                <span>{t('statusEnabled')}</span>
              </Chip>
            ) : (
              <Chip variant="status" colorIndex={4}>
                <XCircle size={12} />
                <span>{t('statusDisabled')}</span>
              </Chip>
            )}
          </div>
          {source.description && (
            <p className={styles.description}>{source.description}</p>
          )}
        </div>

        <div className={styles.sections}>
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>{t('sectionSourceType')}</h3>
            {renderSourceType(source.sourceType)}
          </div>

          {source.sourceType === SOURCE_TYPE.NOTION_DB && source.config && source.config.__typename === 'NotionDBConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>{t('sourceTypeNotionDB')}</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>{t('labelDatabaseId')}</span>
                  <code className={styles.configValue}>{source.config.databaseID}</code>
                </div>
                {source.config.databaseTitle && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>{t('labelDatabaseTitle')}</span>
                    <span className={styles.configValue}>{source.config.databaseTitle}</span>
                  </div>
                )}
                {source.config.databaseURL && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>{t('labelDatabaseUrl')}</span>
                    <a
                      href={source.config.databaseURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.configLink}
                    >
                      {t('linkOpenNotion')}
                      <ExternalLink size={14} />
                    </a>
                  </div>
                )}
              </div>
            </div>
          )}

          {source.sourceType === SOURCE_TYPE.NOTION_PAGE && source.config && source.config.__typename === 'NotionPageConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>{t('sourceTypeNotionPage')}</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>{t('labelPageId')}</span>
                  <code className={styles.configValue}>{source.config.pageID}</code>
                </div>
                {source.config.pageTitle && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>{t('labelPageTitle')}</span>
                    <span className={styles.configValue}>{source.config.pageTitle}</span>
                  </div>
                )}
                {source.config.pageURL && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>{t('labelPageUrl')}</span>
                    <a
                      href={source.config.pageURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.configLink}
                    >
                      {t('linkOpenNotion')}
                      <ExternalLink size={14} />
                    </a>
                  </div>
                )}
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>{t('labelRecursive')}</span>
                  <span className={styles.configValue}>{source.config.recursive ? t('labelYes') : t('labelNo')}</span>
                </div>
                {source.config.recursive && (
                  <div className={styles.configItem}>
                    <span className={styles.configLabel}>{t('labelMaxDepth')}</span>
                    <span className={styles.configValue}>
                      {source.config.maxDepth === 0 ? t('labelUnlimited') : source.config.maxDepth}
                    </span>
                  </div>
                )}
              </div>
            </div>
          )}

          {source.sourceType === SOURCE_TYPE.SLACK && source.config && source.config.__typename === 'SlackConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>{t('sourceTypeSlack')}</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>{t('labelMonitoredChannels')}</span>
                  <div className={styles.channelList}>
                    {source.config.channels.length > 0 ? (
                      source.config.channels.map((channel) => (
                        <div key={channel.id} className={styles.channelTag}>
                          <Hash size={14} />
                          <span>{channel.name || channel.id}</span>
                        </div>
                      ))
                    ) : (
                      <span className={styles.configValue}>{t('emptyNoChannelsConfigured')}</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          {source.sourceType === SOURCE_TYPE.GITHUB && source.config && source.config.__typename === 'GitHubConfig' && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>{t('sourceTypeGitHub')}</h3>
              <div className={styles.configCard}>
                <div className={styles.configItem}>
                  <span className={styles.configLabel}>{t('labelMonitoredRepositories')}</span>
                  <div className={styles.channelList}>
                    {source.config.repositories.length > 0 ? (
                      source.config.repositories.map((repo) => (
                        <div key={`${repo.owner}/${repo.repo}`} className={styles.channelTag}>
                          <GitBranch size={14} />
                          <a
                            href={`https://github.com/${repo.owner}/${repo.repo}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className={styles.configLink}
                          >
                            {repo.owner}/{repo.repo}
                            <ExternalLink size={12} />
                          </a>
                        </div>
                      ))
                    ) : (
                      <span className={styles.configValue}>{t('emptyNoRepositoriesConfigured')}</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          <div className={styles.metadata}>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>{t('labelCreated')}</span>
              <span className={styles.metadataValue}>
                {new Date(source.createdAt).toLocaleString()}
              </span>
            </div>
            <div className={styles.metadataItem}>
              <span className={styles.metadataLabel}>{t('labelUpdated')}</span>
              <span className={styles.metadataValue}>
                {new Date(source.updatedAt).toLocaleString()}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Edit Modal */}
      {(() => {
        const isSaving = updating || updatingGitHub
        const isGitHub = source.sourceType === SOURCE_TYPE.GITHUB
        return (
          <Modal
            isOpen={isEditModalOpen}
            onClose={() => setIsEditModalOpen(false)}
            title={t('titleEditSource')}
            footer={
              <>
                <Button variant="outline" onClick={() => setIsEditModalOpen(false)} disabled={isSaving}>
                  {t('btnCancel')}
                </Button>
                <Button variant="primary" onClick={handleEditSubmit} disabled={isSaving || (isGitHub && editRepos.length === 0)}>
                  {isSaving ? t('btnSaving') : t('btnSave')}
                </Button>
              </>
            }
          >
            <form onSubmit={handleEditSubmit} className={styles.form}>
              <div className={styles.field}>
                <label htmlFor="editName" className={styles.label}>
                  {t('labelNameRequired')}
                </label>
                <input
                  id="editName"
                  type="text"
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className={`${styles.input} ${formErrors.name ? styles.inputError : ''}`}
                  placeholder={t('placeholderSourceName')}
                  disabled={isSaving}
                />
                {formErrors.name && <span className={styles.formError}>{formErrors.name}</span>}
              </div>

              {isGitHub && (
                <div className={styles.field}>
                  <label className={styles.label}>{t('labelRepositories')}</label>
                  <div className={styles.inputWithButton}>
                    <input
                      type="text"
                      value={repoInput}
                      onChange={(e) => setRepoInput(e.target.value)}
                      onKeyDown={handleRepoKeyDown}
                      className={`${styles.input} ${formErrors.repository ? styles.inputError : ''}`}
                      placeholder={t('placeholderGitHubRepo')}
                      disabled={isSaving || validatingRepo}
                    />
                    <Button
                      variant="outline"
                      onClick={handleAddRepo}
                      disabled={isSaving || validatingRepo || !repoInput.trim()}
                    >
                      {validatingRepo ? (
                        <Loader size={16} className={styles.spinner} />
                      ) : (
                        t('btnAdd')
                      )}
                    </Button>
                  </div>
                  {formErrors.repository && (
                    <div className={styles.validationError}>
                      <AlertCircle size={14} />
                      <span>{formErrors.repository}</span>
                    </div>
                  )}
                  {editRepos.length > 0 && (
                    <div className={styles.repoTags}>
                      {editRepos.map((r) => (
                        <div key={`${r.owner}/${r.repo}`} className={styles.repoTag}>
                          <CheckCircle size={14} />
                          <span>{r.owner}/{r.repo}</span>
                          <button
                            type="button"
                            className={styles.repoTagRemove}
                            onClick={() => handleRemoveRepo(r.owner, r.repo)}
                            disabled={isSaving}
                          >
                            <X size={14} />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                  <p className={styles.hint}>
                    {t('hintGitHubRepo')}
                  </p>
                </div>
              )}

              <div className={styles.field}>
                <label htmlFor="editDescription" className={styles.label}>
                  {t('labelDescription')}
                </label>
                <textarea
                  id="editDescription"
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                  className={styles.textarea}
                  placeholder={t('placeholderSourceDescription')}
                  rows={3}
                  disabled={isSaving}
                />
              </div>

              <div className={styles.checkboxField}>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={editEnabled}
                    onChange={(e) => setEditEnabled(e.target.checked)}
                    className={styles.checkbox}
                    disabled={isSaving}
                  />
                  <span>{t('labelEnableSource')}</span>
                </label>
              </div>
            </form>
          </Modal>
        )
      })()}

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
