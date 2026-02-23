import { useState, useEffect } from 'react'
import { useMutation, useLazyQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { CheckCircle, AlertCircle, Loader, X } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import { CREATE_GITHUB_SOURCE, VALIDATE_GITHUB_REPO, GET_SOURCES } from '../../graphql/source'
import styles from './source.module.css'

interface GitHubFormProps {
  isOpen: boolean
  onClose: () => void
}

interface ValidatedRepo {
  owner: string
  repo: string
  fullName: string
}

interface FormErrors {
  repository?: string
  repositories?: string
  name?: string
  form?: string
}

export default function GitHubForm({ isOpen, onClose }: GitHubFormProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [repoInput, setRepoInput] = useState('')
  const [repositories, setRepositories] = useState<ValidatedRepo[]>([])
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [errors, setErrors] = useState<FormErrors>({})

  const [validateRepo, { loading: validating }] = useLazyQuery(VALIDATE_GITHUB_REPO, {
    fetchPolicy: 'network-only',
    onCompleted: (data) => {
      const result = data.validateGitHubRepo
      if (result.valid) {
        const fullName = `${result.owner}/${result.repo}`
        if (repositories.some((r) => r.fullName === fullName)) {
          setErrors((prev) => ({ ...prev, repository: 'This repository is already added' }))
          return
        }
        setRepositories((prev) => [...prev, { owner: result.owner, repo: result.repo, fullName }])
        setRepoInput('')
        setErrors((prev) => ({ ...prev, repository: undefined }))
      } else {
        setErrors((prev) => ({ ...prev, repository: result.errorMessage || 'Invalid repository' }))
      }
    },
    onError: (error) => {
      setErrors((prev) => ({ ...prev, repository: error.message || 'Failed to validate repository' }))
    },
  })

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
        form: error.message || 'Failed to create source. Please try again.',
      }))
    },
  })

  useEffect(() => {
    if (!isOpen) {
      resetForm()
    }
  }, [isOpen])

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

  const handleRepoKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleAddRepo()
    }
  }

  const handleRemoveRepo = (fullName: string) => {
    setRepositories((prev) => prev.filter((r) => r.fullName !== fullName))
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (repositories.length === 0) {
      newErrors.repositories = 'At least one repository is required'
    }

    if (!name.trim()) {
      newErrors.name = 'Name is required'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) return

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

  const handleClose = () => {
    resetForm()
    onClose()
  }

  const loading = creating

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title="Add GitHub Source"
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={loading || repositories.length === 0}
          >
            {loading ? 'Creating...' : 'Create Source'}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        {errors.form && <div className={styles.formError}>{errors.form}</div>}

        <div className={styles.field}>
          <label htmlFor="name" className={styles.label}>
            Name *
          </label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
            placeholder="Enter source name"
            disabled={loading}
          />
          {errors.name && <span className={styles.error}>{errors.name}</span>}
        </div>

        <div className={styles.field}>
          <label className={styles.label}>Repositories *</label>
          <div className={styles.inputWithButton}>
            <input
              type="text"
              value={repoInput}
              onChange={(e) => setRepoInput(e.target.value)}
              onKeyDown={handleRepoKeyDown}
              className={`${styles.input} ${errors.repository ? styles.inputError : ''}`}
              placeholder="owner/repo (e.g., octocat/Hello-World)"
              disabled={loading || validating}
            />
            <Button
              variant="outline"
              onClick={handleAddRepo}
              disabled={loading || validating || !repoInput.trim()}
            >
              {validating ? (
                <Loader size={16} className={styles.spinner} />
              ) : (
                'Add'
              )}
            </Button>
          </div>
          {errors.repository && (
            <div className={styles.validationError}>
              <AlertCircle size={16} />
              <span className={styles.validationDetail}>{errors.repository}</span>
            </div>
          )}
          {errors.repositories && <span className={styles.error}>{errors.repositories}</span>}

          {repositories.length > 0 && (
            <div className={styles.repoTags}>
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
          <p className={styles.hint}>
            Enter repository in owner/repo format. Each repository will be validated before adding.
          </p>
        </div>

        <div className={styles.field}>
          <label htmlFor="description" className={styles.label}>
            Description
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className={styles.textarea}
            placeholder="Enter source description (optional)"
            rows={3}
            disabled={loading}
          />
        </div>

        <div className={styles.checkboxField}>
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className={styles.checkbox}
              disabled={loading}
            />
            <span>Enable this source</span>
          </label>
        </div>
      </form>
    </Modal>
  )
}
