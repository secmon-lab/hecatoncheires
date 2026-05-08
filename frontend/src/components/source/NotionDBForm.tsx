import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
import Modal from '../Modal'
import Button from '../Button'
import { IconCheck, IconExt, IconWarn } from '../Icons'
import { CREATE_NOTION_DB_SOURCE, VALIDATE_NOTION_DB, GET_SOURCES } from '../../graphql/source'
import { parseNotionID } from '../../utils/notion'

interface NotionDBFormProps {
  isOpen: boolean
  onClose: () => void
}

interface FormErrors {
  databaseID?: string
  name?: string
  form?: string
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
      const existing = cache.readQuery<{ sources: unknown[] }>({
        query: GET_SOURCES,
        variables: { workspaceId: currentWorkspace!.id },
      })
      if (existing) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existing.sources, data.createNotionDBSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createNotionDBSource.id}`)
    },
    onError: (error) => {
      setErrors((p) => ({ ...p, form: error.message || t('errorCreateSource') }))
    },
  })

  useEffect(() => { if (!isOpen) resetForm() }, [isOpen])

  const resetForm = () => {
    setDatabaseID('')
    setName('')
    setDescription('')
    setEnabled(true)
    setErrors({})
    setValidationResult(null)
    setIsValidating(false)
  }

  const handleValidate = async () => {
    if (!databaseID.trim()) { setErrors({ databaseID: t('errorDatabaseIdRequired') }); return }
    const parsed = parseNotionID(databaseID)
    if (!parsed) { setErrors({ databaseID: t('errorInvalidDatabaseId') }); return }
    setErrors({})
    setIsValidating(true)
    try {
      const result = await validateNotionDB({
        variables: { workspaceId: currentWorkspace!.id, databaseID: parsed },
      })
      const v: ValidationResult | null = result.data?.validateNotionDB ?? null
      setValidationResult(v)
      if (v?.valid && v.databaseTitle && !name) setName(v.databaseTitle)
    } catch {
      setValidationResult({
        valid: false,
        databaseTitle: null,
        databaseURL: null,
        errorMessage: t('errorValidateDatabase'),
      })
    } finally { setIsValidating(false) }
  }

  const validate = () => {
    const e: FormErrors = {}
    if (!databaseID.trim()) e.databaseID = t('errorDatabaseIdRequired')
    if (!name.trim()) e.name = t('errorNameRequired')
    if (!validationResult?.valid) e.databaseID = t('errorValidateDatabaseFirst')
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const handleSubmit = async () => {
    if (!validate()) return
    const parsed = parseNotionID(databaseID)
    await createSource({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          databaseID: parsed ?? databaseID.trim(),
          name: name.trim(),
          description: description.trim() || undefined,
          enabled,
        },
      },
    })
  }

  const handleClose = () => { resetForm(); onClose() }

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      width={600}
      title={t('titleAddNotionDbSource')}
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} disabled={creating}>{t('btnCancel')}</Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={creating || !validationResult?.valid}
          >
            {creating ? t('btnCreating') : t('btnCreateSource')}
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
          <label htmlFor="ndb-id" className="field-label">{t('labelDatabaseIdRequired')}</label>
          <div className="row" style={{ gap: 6 }}>
            <input
              id="ndb-id"
              className="input mono"
              value={databaseID}
              onChange={(e) => { setDatabaseID(e.target.value); setValidationResult(null) }}
              placeholder={t('placeholderNotionDbId')}
              disabled={creating}
              style={{ flex: 1 }}
            />
            <Button onClick={handleValidate} disabled={creating || isValidating || !databaseID.trim()}>
              {isValidating ? '…' : t('btnValidate')}
            </Button>
          </div>
          {errors.databaseID && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.databaseID}</div>}
          <div className="soft" style={{ fontSize: 11.5, marginTop: 4 }}>{t('hintNotionDbId')}</div>
        </div>
        {validationResult && (
          <div style={{
            padding: 10, borderRadius: 6, fontSize: 12,
            background: validationResult.valid
              ? 'color-mix(in oklch, var(--ok) 10%, transparent)'
              : 'color-mix(in oklch, var(--danger) 10%, transparent)',
            border: `1px solid color-mix(in oklch, ${validationResult.valid ? 'var(--ok)' : 'var(--danger)'} 30%, transparent)`,
          }}>
            <div className="row" style={{ gap: 8 }}>
              <span style={{ color: validationResult.valid ? 'var(--ok)' : 'var(--danger)' }}>
                {validationResult.valid ? <IconCheck size={13} /> : <IconWarn size={13} />}
              </span>
              <b>{validationResult.valid ? t('validationDatabaseFound') : t('validationFailed')}</b>
              <span className="spacer" />
              {validationResult.databaseURL && (
                <a
                  href={validationResult.databaseURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{ color: 'var(--accent)', fontSize: 11 }}
                >
                  {t('linkOpenNotion')}<IconExt size={10} style={{ verticalAlign: -1, marginLeft: 3 }} />
                </a>
              )}
            </div>
            {(validationResult.databaseTitle || validationResult.errorMessage) && (
              <div className="soft" style={{ fontSize: 11, marginTop: 2 }}>
                {validationResult.databaseTitle || validationResult.errorMessage}
              </div>
            )}
          </div>
        )}
        <div>
          <label htmlFor="ndb-name" className="field-label">{t('labelNameRequired')}</label>
          <input
            id="ndb-name"
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('placeholderSourceName')}
            disabled={creating}
          />
          {errors.name && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.name}</div>}
        </div>
        <div>
          <label htmlFor="ndb-desc" className="field-label">{t('labelDescription')}</label>
          <textarea
            id="ndb-desc"
            className="textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('placeholderSourceDescription')}
            rows={3}
            disabled={creating}
          />
        </div>
        <label className="row" style={{ gap: 8, fontSize: 13 }}>
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            disabled={creating}
          />
          <span>{t('labelEnableSource')}</span>
        </label>
      </div>
    </Modal>
  )
}
