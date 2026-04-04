import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
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
  const { t } = useTranslation()
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
      setErrors({ pageID: t('errorPageIdRequired') })
      return
    }

    const parsedID = parseNotionID(pageID)
    if (!parsedID) {
      setErrors({ pageID: t('errorInvalidPageId') })
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
        errorMessage: t('errorValidatePage'),
      })
    } finally {
      setIsValidating(false)
    }
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!pageID.trim()) {
      newErrors.pageID = t('errorPageIdRequired')
    }

    if (!name.trim()) {
      newErrors.name = t('errorNameRequired')
    }

    if (!validationResult?.valid) {
      newErrors.pageID = t('errorValidatePageFirst')
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
      title={t('titleAddNotionPageSource')}
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            {t('btnCancel')}
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={loading || !validationResult?.valid}
          >
            {loading ? t('btnCreating') : t('btnCreateSource')}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.field}>
          <label htmlFor="pageID" className={styles.label}>
            {t('labelPageIdRequired')}
          </label>
          <div className={styles.inputWithButton}>
            <input
              id="pageID"
              type="text"
              value={pageID}
              onChange={(e) => handlePageIDChange(e.target.value)}
              className={`${styles.input} ${errors.pageID ? styles.inputError : ''}`}
              placeholder={t('placeholderNotionPageId')}
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
                t('btnValidate')
              )}
            </Button>
          </div>
          {errors.pageID && <span className={styles.error}>{errors.pageID}</span>}
          <p className={styles.hint}>
            {t('hintNotionPageId')}
          </p>
        </div>

        {validationResult && (
          <div className={validationResult.valid ? styles.validationSuccess : styles.validationError}>
            {validationResult.valid ? (
              <>
                <CheckCircle size={20} />
                <div className={styles.validationContent}>
                  <span className={styles.validationTitle}>{t('validationPageFound')}</span>
                  <span className={styles.validationDetail}>{validationResult.pageTitle}</span>
                  {validationResult.pageURL && (
                    <a
                      href={validationResult.pageURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.validationLink}
                    >
                      {t('linkOpenNotion')}
                    </a>
                  )}
                </div>
              </>
            ) : (
              <>
                <AlertCircle size={20} />
                <div className={styles.validationContent}>
                  <span className={styles.validationTitle}>{t('validationFailed')}</span>
                  <span className={styles.validationDetail}>{validationResult.errorMessage}</span>
                </div>
              </>
            )}
          </div>
        )}

        <div className={styles.field}>
          <label htmlFor="name" className={styles.label}>
            {t('labelNameRequired')}
          </label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
            placeholder={t('placeholderSourceName')}
            disabled={loading}
          />
          {errors.name && <span className={styles.error}>{errors.name}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="description" className={styles.label}>
            {t('labelDescription')}
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className={styles.textarea}
            placeholder={t('placeholderSourceDescription')}
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
            <span>{t('labelIncludeChildPages')}</span>
          </label>
        </div>

        {recursive && (
          <div className={styles.field}>
            <label htmlFor="maxDepth" className={styles.label}>
              {t('labelMaxDepth')}
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
              {t('hintMaxDepth')}
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
            <span>{t('labelEnableSource')}</span>
          </label>
        </div>
      </form>
    </Modal>
  )
}
