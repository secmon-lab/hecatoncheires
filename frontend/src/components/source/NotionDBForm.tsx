import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
import { CheckCircle, AlertCircle, Loader2 } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import { CREATE_NOTION_DB_SOURCE, VALIDATE_NOTION_DB, GET_SOURCES } from '../../graphql/source'
import { parseNotionID } from '../../utils/notion'
import styles from './source.module.css'

interface NotionDBFormProps {
  isOpen: boolean
  onClose: () => void
}

interface FormErrors {
  databaseID?: string
  name?: string
}

interface ValidationResult {
  valid: boolean
  databaseTitle: string | null
  databaseURL: string | null
  errorMessage: string | null
}

export default function NotionDBForm({ isOpen, onClose }: NotionDBFormProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [databaseID, setDatabaseID] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [errors, setErrors] = useState<FormErrors>({})
  const [validationResult, setValidationResult] = useState<ValidationResult | null>(null)
  const [isValidating, setIsValidating] = useState(false)

  const [validateNotionDB] = useMutation(VALIDATE_NOTION_DB)

  const [createSource, { loading: creating }] = useMutation(CREATE_NOTION_DB_SOURCE, {
    update(cache, { data }) {
      if (!data?.createNotionDBSource) return
      const existingData = cache.readQuery<{ sources: unknown[] }>({ query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existingData.sources, data.createNotionDBSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createNotionDBSource.id}`)
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
    setDatabaseID('')
    setName('')
    setDescription('')
    setEnabled(true)
    setErrors({})
    setValidationResult(null)
    setIsValidating(false)
  }

  const handleDatabaseIDChange = (value: string) => {
    setDatabaseID(value)
    setValidationResult(null)
  }

  const handleValidate = async () => {
    if (!databaseID.trim()) {
      setErrors({ databaseID: t('errorDatabaseIdRequired') })
      return
    }

    const parsedID = parseNotionID(databaseID)
    if (!parsedID) {
      setErrors({ databaseID: t('errorInvalidDatabaseId') })
      return
    }

    setErrors({})
    setIsValidating(true)

    try {
      const result = await validateNotionDB({
        variables: { workspaceId: currentWorkspace!.id, databaseID: parsedID },
      })

      const validation: ValidationResult | null = result.data?.validateNotionDB ?? null
      setValidationResult(validation)

      if (validation?.valid && validation.databaseTitle && !name) {
        setName(validation.databaseTitle)
      }
    } catch (error) {
      console.error('Validation error:', error)
      setValidationResult({
        valid: false,
        databaseTitle: null,
        databaseURL: null,
        errorMessage: t('errorValidateDatabase'),
      })
    } finally {
      setIsValidating(false)
    }
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!databaseID.trim()) {
      newErrors.databaseID = t('errorDatabaseIdRequired')
    }

    if (!name.trim()) {
      newErrors.name = t('errorNameRequired')
    }

    if (!validationResult?.valid) {
      newErrors.databaseID = t('errorValidateDatabaseFirst')
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    const parsedID = parseNotionID(databaseID)
    await createSource({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          databaseID: parsedID ?? databaseID.trim(),
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
      title={t('titleAddNotionDbSource')}
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
          <label htmlFor="databaseID" className={styles.label}>
            {t('labelDatabaseIdRequired')}
          </label>
          <div className={styles.inputWithButton}>
            <input
              id="databaseID"
              type="text"
              value={databaseID}
              onChange={(e) => handleDatabaseIDChange(e.target.value)}
              className={`${styles.input} ${errors.databaseID ? styles.inputError : ''}`}
              placeholder={t('placeholderNotionDbId')}
              disabled={loading}
            />
            <Button
              variant="outline"
              onClick={handleValidate}
              disabled={loading || isValidating || !databaseID.trim()}
              type="button"
            >
              {isValidating ? (
                <Loader2 size={16} className={styles.spinner} />
              ) : (
                t('btnValidate')
              )}
            </Button>
          </div>
          {errors.databaseID && <span className={styles.error}>{errors.databaseID}</span>}
          <p className={styles.hint}>
            {t('hintNotionDbId')}
          </p>
        </div>

        {validationResult && (
          <div className={validationResult.valid ? styles.validationSuccess : styles.validationError}>
            {validationResult.valid ? (
              <>
                <CheckCircle size={20} />
                <div className={styles.validationContent}>
                  <span className={styles.validationTitle}>{t('validationDatabaseFound')}</span>
                  <span className={styles.validationDetail}>{validationResult.databaseTitle}</span>
                  {validationResult.databaseURL && (
                    <a
                      href={validationResult.databaseURL}
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
