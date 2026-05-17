import { useState, useEffect } from 'react'
import { useMutation, useLazyQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
import { CheckCircle, AlertCircle, Loader, X } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import {
  CREATE_GITHUB_SOURCE,
  UPDATE_GITHUB_SOURCE,
  VALIDATE_GITHUB_REPO,
  GET_SOURCE,
  GET_SOURCES,
} from '../../graphql/source'
import { commitOnEnter } from '../../utils/keyboard'
import styles from './source.module.css'
import {
  reposFromSource,
  type GitHubSourceForEdit,
  type ValidatedRepo,
} from './sourceFormHelpers'

interface GitHubFormProps {
  isOpen: boolean
  onClose: () => void
  mode?: 'create' | 'edit'
  source?: GitHubSourceForEdit
}

interface FormErrors {
  repository?: string
  repositories?: string
  name?: string
  form?: string
}

export default function GitHubForm({ isOpen, onClose, mode = 'create', source }: GitHubFormProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const isEdit = mode === 'edit'

  const [repoInput, setRepoInput] = useState('')
  const [repositories, setRepositories] = useState<ValidatedRepo[]>(isEdit ? reposFromSource(source) : [])
  const [name, setName] = useState(isEdit && source ? source.name : '')
  const [description, setDescription] = useState(isEdit && source ? source.description ?? '' : '')
  const [enabled, setEnabled] = useState(true)
  const [errors, setErrors] = useState<FormErrors>({})

  const [validateRepo, { loading: validating }] = useLazyQuery(VALIDATE_GITHUB_REPO, {
    fetchPolicy: 'network-only',
    onCompleted: (data) => {
      const result = data.validateGitHubRepo
      if (result.valid) {
        const fullName = `${result.owner}/${result.repo}`
        if (repositories.some((r) => r.fullName === fullName)) {
          setErrors((prev) => ({ ...prev, repository: t('errorRepoAlreadyAdded') }))
          return
        }
        setRepositories((prev) => [...prev, { owner: result.owner, repo: result.repo, fullName }])
        setRepoInput('')
        setErrors((prev) => ({ ...prev, repository: undefined }))
      } else {
        setErrors((prev) => ({ ...prev, repository: result.errorMessage || t('errorInvalidRepo') }))
      }
    },
    onError: (error) => {
      setErrors((prev) => ({ ...prev, repository: error.message || t('errorValidateRepo') }))
    },
  })

  const refetchAfterEdit = isEdit && source
    ? [
        { query: GET_SOURCE, variables: { workspaceId: currentWorkspace?.id, id: source.id } },
        { query: GET_SOURCES, variables: { workspaceId: currentWorkspace?.id } },
      ]
    : undefined

  const [createSource, { loading: creating }] = useMutation(CREATE_GITHUB_SOURCE, {
    update(cache, { data }) {
      if (!data?.createGitHubSource) return
      const existingData = cache.readQuery<{ sources: unknown[] }>({ query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existingData.sources, data.createGitHubSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createGitHubSource.id}`)
    },
    onError: (error) => {
      setErrors((prev) => ({
        ...prev,
        form: error.message || t('errorCreateSource'),
      }))
    },
  })

  const [updateSource, { loading: updating }] = useMutation(UPDATE_GITHUB_SOURCE, {
    refetchQueries: refetchAfterEdit,
    onCompleted: () => {
      onClose()
    },
    onError: (error) => {
      setErrors((prev) => ({ ...prev, form: error.message || t('errorUpdateSource') }))
    },
  })

  useEffect(() => {
    if (!isOpen) return
    if (isEdit && source) {
      setRepoInput('')
      setRepositories(reposFromSource(source))
      setName(source.name)
      setDescription(source.description ?? '')
      setEnabled(true)
      setErrors({})
    } else if (!isEdit) {
      resetForm()
    }
  }, [isOpen, isEdit, source?.id])

  const resetForm = () => {
    setRepoInput('')
    setRepositories([])
    setName('')
    setDescription('')
    setEnabled(true)
    setErrors({})
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

  const handleRepoKeyDown = commitOnEnter({
    onCommit: handleAddRepo,
  })

  const handleRemoveRepo = (fullName: string) => {
    setRepositories((prev) => prev.filter((r) => r.fullName !== fullName))
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (repositories.length === 0) {
      newErrors.repositories = t('errorRepositoriesRequired')
    }

    if (!name.trim()) {
      newErrors.name = t('errorNameRequired')
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async () => {
    if (!validate()) return

    if (isEdit && source) {
      await updateSource({
        variables: {
          workspaceId: currentWorkspace!.id,
          input: {
            id: source.id,
            name: name.trim(),
            description: description.trim(),
            repositories: repositories.map((r) => r.fullName),
          },
        },
      })
    } else {
      await createSource({
        variables: {
          workspaceId: currentWorkspace!.id,
          input: {
            repositories: repositories.map((r) => r.fullName),
            name: name.trim(),
            description: description.trim() || undefined,
            enabled,
          },
        },
      })
    }
  }

  const handleClose = () => {
    if (!isEdit) resetForm()
    onClose()
  }

  const loading = creating || updating
  const title = isEdit ? `${t('titleEditSource')} · GITHUB` : t('titleAddGithubSource')
  const submitLabel = isEdit
    ? loading ? t('btnSaving') : t('btnSave')
    : loading ? t('btnCreating') : t('btnCreateSource')

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={title}
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            {t('btnCancel')}
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={loading || repositories.length === 0 || !name.trim()}
          >
            {submitLabel}
          </Button>
        </>
      }
    >
      <div className="col" style={{ gap: 14 }}>
        {errors.form && (
          <div style={{
            padding: '8px 10px', borderRadius: 6,
            background: 'color-mix(in oklch, var(--danger) 10%, transparent)',
            border: '1px solid color-mix(in oklch, var(--danger) 30%, transparent)',
            color: 'var(--danger)', fontSize: 12,
          }}>{errors.form}</div>
        )}
        <div>
          <label htmlFor="gh-name" className="field-label">{t('labelNameRequired')}</label>
          <input
            id="gh-name"
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('placeholderSourceName')}
            disabled={loading}
          />
          {errors.name && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.name}</div>}
        </div>
        <div>
          <label htmlFor="gh-desc" className="field-label">{t('labelDescription')}</label>
          <textarea
            id="gh-desc"
            className="textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('placeholderSourceDescription')}
            rows={3}
            disabled={loading}
          />
        </div>
        <div>
          <label htmlFor="gh-repo" className="field-label">{t('labelRepositoriesRequired')}</label>
          <div className="row" style={{ gap: 6 }}>
            <input
              id="gh-repo"
              type="text"
              value={repoInput}
              onChange={(e) => setRepoInput(e.target.value)}
              onKeyDown={handleRepoKeyDown}
              className="input"
              placeholder={t('placeholderGitHubRepo')}
              disabled={loading || validating}
              style={{ flex: 1 }}
            />
            <Button
              variant="outline"
              onClick={handleAddRepo}
              disabled={loading || validating || !repoInput.trim()}
            >
              {validating ? (
                <Loader size={16} className={styles.spinner} />
              ) : (
                t('btnAdd')
              )}
            </Button>
          </div>
          {errors.repository && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>
              <AlertCircle size={14} />
              <span>{errors.repository}</span>
            </div>
          )}
          {errors.repositories && (
            <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.repositories}</div>
          )}
          {repositories.length > 0 && (
            <div className={styles.repoTags} style={{ marginTop: 8 }}>
              {repositories.map((repo) => (
                <div key={repo.fullName} className={styles.repoTag}>
                  <CheckCircle size={14} />
                  <span>{repo.fullName}</span>
                  <button
                    type="button"
                    className={styles.repoTagRemove}
                    onClick={() => handleRemoveRepo(repo.fullName)}
                    disabled={loading}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
          <div className="soft" style={{ fontSize: 11.5, marginTop: 4 }}>
            {t('hintGitHubRepo')}
          </div>
        </div>

        {!isEdit && (
          <label className="row" style={{ gap: 8, fontSize: 13 }}>
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              disabled={loading}
            />
            <span>{t('labelEnableSource')}</span>
          </label>
        )}
      </div>
    </Modal>
  )
}
