import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
import Modal from '../Modal'
import Button from '../Button'
import { IconCheck, IconExt, IconWarn } from '../Icons'
import { CREATE_NOTION_PAGE_SOURCE, VALIDATE_NOTION_PAGE, GET_SOURCES } from '../../graphql/source'
import { parseNotionID } from '../../utils/notion'

interface NotionPageFormProps {
  isOpen: boolean
  onClose: () => void
}

interface FormErrors {
  pageID?: string
  name?: string
  form?: string
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
      const existing = cache.readQuery<{ sources: unknown[] }>({
        query: GET_SOURCES,
        variables: { workspaceId: currentWorkspace!.id },
      })
      if (existing) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existing.sources, data.createNotionPageSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createNotionPageSource.id}`)
    },
    onError: (error) => {
      setErrors((p) => ({ ...p, form: error.message || t('errorCreateSource') }))
    },
  })

  useEffect(() => { if (!isOpen) resetForm() }, [isOpen])

  const resetForm = () => {
    setPageID('')
    setName('')
    setDescription('')
    setEnabled(true)
    setRecursive(false)
    setMaxDepth(4)
    setErrors({})
    setValidationResult(null)
    setIsValidating(false)
  }

  const handleValidate = async () => {
    if (!pageID.trim()) { setErrors({ pageID: t('errorPageIdRequired') }); return }
    const parsed = parseNotionID(pageID)
    if (!parsed) { setErrors({ pageID: t('errorInvalidPageId') }); return }
    setErrors({})
    setIsValidating(true)
    try {
      const result = await validateNotionPage({
        variables: { workspaceId: currentWorkspace!.id, pageID: parsed },
      })
      const v: ValidationResult | null = result.data?.validateNotionPage ?? null
      setValidationResult(v)
      if (v?.valid && v.pageTitle && !name) setName(v.pageTitle)
    } catch {
      setValidationResult({
        valid: false,
        pageTitle: null,
        pageURL: null,
        errorMessage: t('errorValidatePage'),
      })
    } finally { setIsValidating(false) }
  }

  const validate = () => {
    const e: FormErrors = {}
    if (!pageID.trim()) e.pageID = t('errorPageIdRequired')
    if (!name.trim()) e.name = t('errorNameRequired')
    if (!validationResult?.valid) e.pageID = t('errorValidatePageFirst')
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const handleSubmit = async () => {
    if (!validate()) return
    const parsed = parseNotionID(pageID)
    await createSource({
      variables: {
        workspaceId: currentWorkspace!.id,
        input: {
          pageID: parsed ?? pageID.trim(),
          name: name.trim(),
          description: description.trim() || undefined,
          enabled,
          recursive,
          maxDepth: recursive ? maxDepth : 0,
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
      title={t('titleAddNotionPageSource')}
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
          <label htmlFor="np-id" className="field-label">{t('labelPageIdRequired')}</label>
          <div className="row" style={{ gap: 6 }}>
            <input
              id="np-id"
              className="input mono"
              value={pageID}
              onChange={(e) => { setPageID(e.target.value); setValidationResult(null) }}
              placeholder={t('placeholderNotionPageId')}
              disabled={creating}
              style={{ flex: 1 }}
            />
            <Button onClick={handleValidate} disabled={creating || isValidating || !pageID.trim()}>
              {isValidating ? '…' : t('btnValidate')}
            </Button>
          </div>
          {errors.pageID && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.pageID}</div>}
          <div className="soft" style={{ fontSize: 11.5, marginTop: 4 }}>{t('hintNotionPageId')}</div>
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
              <b>{validationResult.valid ? t('validationPageFound') : t('validationFailed')}</b>
              <span className="spacer" />
              {validationResult.pageURL && (
                <a
                  href={validationResult.pageURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{ color: 'var(--accent)', fontSize: 11 }}
                >
                  {t('linkOpenNotion')}<IconExt size={10} style={{ verticalAlign: -1, marginLeft: 3 }} />
                </a>
              )}
            </div>
            {(validationResult.pageTitle || validationResult.errorMessage) && (
              <div className="soft" style={{ fontSize: 11, marginTop: 2 }}>
                {validationResult.pageTitle || validationResult.errorMessage}
              </div>
            )}
          </div>
        )}
        <div>
          <label htmlFor="np-name" className="field-label">{t('labelNameRequired')}</label>
          <input
            id="np-name"
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('placeholderSourceName')}
            disabled={creating}
          />
          {errors.name && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.name}</div>}
        </div>
        <div>
          <label htmlFor="np-desc" className="field-label">{t('labelDescription')}</label>
          <textarea
            id="np-desc"
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
            checked={recursive}
            onChange={(e) => setRecursive(e.target.checked)}
            disabled={creating}
          />
          <span>{t('labelIncludeChildPages')}</span>
        </label>
        {recursive && (
          <div>
            <label htmlFor="np-depth" className="field-label">{t('labelMaxDepth')}</label>
            <input
              id="np-depth"
              type="number"
              min={0}
              value={maxDepth}
              onChange={(e) => setMaxDepth(Math.max(0, parseInt(e.target.value, 10) || 0))}
              className="input"
              disabled={creating}
              style={{ width: 100 }}
            />
            <div className="soft" style={{ fontSize: 11.5, marginTop: 4 }}>{t('hintMaxDepth')}</div>
          </div>
        )}
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
