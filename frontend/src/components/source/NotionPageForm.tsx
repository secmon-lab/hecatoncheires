import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { CheckCircle, AlertCircle, Loader2 } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import { CREATE_NOTION_PAGE_SOURCE, VALIDATE_NOTION_PAGE, GET_SOURCES } from '../../graphql/source'
import { parseNotionID } from '../../utils/notion'
import styles from './source.module.css'

interface NotionPageFormProps {
  isOpen: boolean
  onClose: () => void
}

interface FormErrors {
  pageID?: string
  name?: string
}

interface ValidationResult {
  valid: boolean
  pageTitle: string | null
  pageURL: string | null
  errorMessage: string | null
}

export default function NotionPageForm({ isOpen, onClose }: NotionPageFormProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const [pageID, setPageID] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [recursive, setRecursive] = useState(false)
  const [maxDepth, setMaxDepth] = useState(4)
  const [errors, setErrors] = useState<FormErrors>({})
  const [validationResult, setValidationResult] = useState<ValidationResult | null>(null)
  const [isValidating, setIsValidating] = useState(false)

  const [validateNotionPage] = useMutation(VALIDATE_NOTION_PAGE)

  const [createSource, { loading: creating }] = useMutation(CREATE_NOTION_PAGE_SOURCE, {
    update(cache, { data }) {
      if (!data?.createNotionPageSource) return
      const existingData = cache.readQuery<{ sources: unknown[] }>({ query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existingData.sources, data.createNotionPageSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createNotionPageSource.id}`)
    },
    onError: (error) => {
      console.error('Create source error:', error)
    },
  })

  useEffect(() => {
    if (!isOpen) {
      resetForm()
    }
  }, [isOpen])

  const resetForm = () => {
    setPageID('')
    setName('')
    setDescription('')
    setEnabled(true)
    setRecursive(false)
    setMaxDepth(0)
    setErrors({})
    setValidationResult(null)
    setIsValidating(false)
  }

  const handlePageIDChange = (value: string) => {
    setPageID(value)
    setValidationResult(null)
  }

  const handleValidate = async () => {
    if (!pageID.trim()) {
      setErrors({ pageID: 'Page ID or URL is required' })
      return
    }

    const parsedID = parseNotionID(pageID)
    if (!parsedID) {
      setErrors({ pageID: 'Invalid page ID or URL format' })
      return
    }

    setErrors({})
    setIsValidating(true)

    try {
      const result = await validateNotionPage({
        variables: { workspaceId: currentWorkspace!.id, pageID: parsedID },
      })

      const validation: ValidationResult | null = result.data?.validateNotionPage ?? null
      setValidationResult(validation)

      if (validation?.valid && validation.pageTitle && !name) {
        setName(validation.pageTitle)
      }
    } catch (error) {
      console.error('Validation error:', error)
      setValidationResult({
        valid: false,
        pageTitle: null,
        pageURL: null,
        errorMessage: 'Failed to validate page. Please try again.',
      })
    } finally {
      setIsValidating(false)
    }
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!pageID.trim()) {
      newErrors.pageID = 'Page ID is required'
    }

    if (!name.trim()) {
      newErrors.name = 'Name is required'
    }

    if (!validationResult?.valid) {
      newErrors.pageID = 'Please validate the page ID first'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    const parsedID = parseNotionID(pageID)
    await createSource({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          pageID: parsedID ?? pageID.trim(),
          name: name.trim(),
          description: description.trim() || undefined,
          enabled,
          recursive,
          maxDepth: recursive ? maxDepth : 0,
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
      title="Add Notion Page Source"
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={loading || !validationResult?.valid}
          >
            {loading ? 'Creating...' : 'Create Source'}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.field}>
          <label htmlFor="pageID" className={styles.label}>
            Page ID / URL *
          </label>
          <div className={styles.inputWithButton}>
            <input
              id="pageID"
              type="text"
              value={pageID}
              onChange={(e) => handlePageIDChange(e.target.value)}
              className={`${styles.input} ${errors.pageID ? styles.inputError : ''}`}
              placeholder="Enter Notion page ID or paste URL"
              disabled={loading}
            />
            <Button
              variant="outline"
              onClick={handleValidate}
              disabled={loading || isValidating || !pageID.trim()}
              type="button"
            >
              {isValidating ? (
                <Loader2 size={16} className={styles.spinner} />
              ) : (
                'Validate'
              )}
            </Button>
          </div>
          {errors.pageID && <span className={styles.error}>{errors.pageID}</span>}
          <p className={styles.hint}>
            Paste a Notion page URL or enter the page ID directly
          </p>
        </div>

        {validationResult && (
          <div className={validationResult.valid ? styles.validationSuccess : styles.validationError}>
            {validationResult.valid ? (
              <>
                <CheckCircle size={20} />
                <div className={styles.validationContent}>
                  <span className={styles.validationTitle}>Page found</span>
                  <span className={styles.validationDetail}>{validationResult.pageTitle}</span>
                  {validationResult.pageURL && (
                    <a
                      href={validationResult.pageURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.validationLink}
                    >
                      Open in Notion
                    </a>
                  )}
                </div>
              </>
            ) : (
              <>
                <AlertCircle size={20} />
                <div className={styles.validationContent}>
                  <span className={styles.validationTitle}>Validation failed</span>
                  <span className={styles.validationDetail}>{validationResult.errorMessage}</span>
                </div>
              </>
            )}
          </div>
        )}

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
              checked={recursive}
              onChange={(e) => setRecursive(e.target.checked)}
              className={styles.checkbox}
              disabled={loading}
            />
            <span>Include child pages recursively</span>
          </label>
        </div>

        {recursive && (
          <div className={styles.field}>
            <label htmlFor="maxDepth" className={styles.label}>
              Max Depth (0 = unlimited)
            </label>
            <input
              id="maxDepth"
              type="number"
              min={0}
              value={maxDepth}
              onChange={(e) => setMaxDepth(Math.max(0, parseInt(e.target.value, 10) || 0))}
              className={styles.input}
              disabled={loading}
            />
            <p className={styles.hint}>
              Set to 0 to traverse all child pages without limit
            </p>
          </div>
        )}

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
