import { useState, useEffect } from 'react'
import { useQuery, useMutation } from '@apollo/client'
import Modal from '../Modal'
import Button from '../Button'
import CustomFieldRenderer from '../fields/CustomFieldRenderer'
import { IconWarn } from '../Icons'
import { useTranslation } from '../../i18n'
import { commitOnEnter } from '../../utils/keyboard'
import { sanitizeFieldValues } from '../../utils/sanitizeFieldValues'
import { GET_MEMO, CREATE_MEMO, UPDATE_MEMO, GET_MEMOS_BY_CASE } from '../../graphql/memo'

interface FieldOption {
  id: string
  name: string
  description?: string
  metadata?: Record<string, unknown>
}

interface FieldDef {
  id: string
  name: string
  type: string
  required: boolean
  description?: string
  options?: FieldOption[]
}

interface Props {
  workspaceId: string
  caseId: number
  memoId?: string
  memoFields: FieldDef[]
  onClose: () => void
  onSaved: () => void
  activeFilter: string | null
}

export default function MemoFormModal({
  workspaceId,
  caseId,
  memoId,
  memoFields,
  onClose,
  onSaved,
  activeFilter,
}: Props) {
  const { t } = useTranslation()
  const isEdit = !!memoId

  const [title, setTitle] = useState('')
  const [fieldValues, setFieldValues] = useState<Record<string, any>>({})
  const [titleError, setTitleError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [genericError, setGenericError] = useState('')

  const { data: existingData } = useQuery(GET_MEMO, {
    variables: { workspaceId, caseID: caseId, id: memoId! },
    skip: !memoId,
    fetchPolicy: 'cache-and-network',
  })

  // Pre-fill form when editing
  useEffect(() => {
    if (!existingData?.memo) return
    const m = existingData.memo
    setTitle(m.title || '')
    const vals: Record<string, unknown> = {}
    if (m.fields) {
      for (const fv of m.fields) {
        vals[fv.fieldId] = fv.value
      }
    }
    setFieldValues(vals)
  }, [existingData])

  const refetchMemos = [
    {
      query: GET_MEMOS_BY_CASE,
      variables: { workspaceId, caseID: caseId, filter: activeFilter },
    },
  ]

  const [createMemo, { loading: creating }] = useMutation(CREATE_MEMO, {
    refetchQueries: refetchMemos,
  })
  const [updateMemo, { loading: updating }] = useMutation(UPDATE_MEMO, {
    refetchQueries: refetchMemos,
  })

  const saving = creating || updating

  const validate = (): boolean => {
    let valid = true
    const newFieldErrors: Record<string, string> = {}
    setTitleError('')
    setGenericError('')

    if (!title.trim()) {
      setTitleError(t('memoFormErrorTitleRequired'))
      valid = false
    }

    for (const f of memoFields) {
      if (f.required) {
        const v = fieldValues[f.id]
        const isEmpty =
          v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)
        if (isEmpty) {
          newFieldErrors[f.id] = t('errorFieldRequired', { fieldName: f.name })
          valid = false
        }
      }
    }

    setFieldErrors(newFieldErrors)
    if (!valid) setGenericError(t('memoFormErrorGeneric'))
    return valid
  }

  const handleSubmit = async () => {
    if (!validate()) return

    const fieldArr = Object.entries(fieldValues).map(([fieldId, value]) => ({ fieldId, value }))
    const sanitized = sanitizeFieldValues(fieldArr, memoFields)

    try {
      if (isEdit) {
        await updateMemo({
          variables: {
            workspaceId,
            input: {
              id: memoId,
              caseID: caseId,
              title: title.trim(),
              fields: sanitized,
            },
          },
        })
      } else {
        await createMemo({
          variables: {
            workspaceId,
            input: {
              caseID: caseId,
              title: title.trim(),
              fields: sanitized,
            },
          },
        })
      }
      onSaved()
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setGenericError(msg)
    }
  }

  const handleFieldChange = (fieldId: string, value: any) => {
    setFieldValues((prev) => ({ ...prev, [fieldId]: value }))
    if (fieldErrors[fieldId]) {
      setFieldErrors((prev) => {
        const next = { ...prev }
        delete next[fieldId]
        return next
      })
    }
  }

  const keyHandler = commitOnEnter({ onCommit: () => { void handleSubmit() }, requireModifier: true })

  return (
    <Modal
      open
      onClose={onClose}
      title={isEdit ? t('memoFormTitleEdit') : t('memoFormTitleNew')}
      width={640}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            {t('btnCancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => { void handleSubmit() }}
            disabled={saving}
          >
            {isEdit ? t('memoFormSave') : t('memoFormCreate')}
          </Button>
        </>
      }
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-4)' }}>
        {/* Generic error banner */}
        {genericError && (
          <div
            role="alert"
            style={{
              display: 'flex',
              gap: 'var(--sp-2)',
              alignItems: 'flex-start',
              background: 'var(--bg-sunken)',
              border: '1px solid var(--border-default)',
              borderRadius: '0.375rem',
              padding: 'var(--sp-3) var(--sp-4)',
              fontSize: 12,
              color: 'var(--color-error)',
              lineHeight: 1.6,
            }}
          >
            <IconWarn size={14} style={{ flexShrink: 0, marginTop: 1 }} />
            <span>{genericError}</span>
          </div>
        )}

        {/* Title field */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-1)' }}>
          <label
            htmlFor="memo-title"
            style={{ fontSize: 12, fontWeight: 600, color: 'var(--fg-muted)' }}
          >
            {t('memoFormLabelTitleRequired')}
          </label>
          <input
            id="memo-title"
            type="text"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value)
              if (titleError) setTitleError('')
            }}
            onKeyDown={keyHandler}
            placeholder={t('memoFormPlaceholderTitle')}
            disabled={saving}
            style={{
              padding: 'var(--sp-2) var(--sp-3)',
              border: `1px solid ${titleError ? 'var(--color-error)' : 'var(--border-default)'}`,
              borderRadius: '0.375rem',
              background: 'var(--bg-paper)',
              color: 'var(--fg)',
              fontSize: 13,
              fontFamily: 'inherit',
              outline: 'none',
              width: '100%',
              boxSizing: 'border-box',
            }}
          />
          {titleError && (
            <span style={{ fontSize: 12, color: 'var(--color-error)' }}>{titleError}</span>
          )}
        </div>

        {/* Dynamic fields */}
        {memoFields.map((f) => (
          <CustomFieldRenderer
            key={f.id}
            field={{ ...f, options: f.options ?? undefined }}
            value={fieldValues[f.id]}
            onChange={handleFieldChange}
            error={fieldErrors[f.id]}
            disabled={saving}
          />
        ))}

        {/* Save hint */}
        <p style={{ margin: 0, fontSize: 11, color: 'var(--fg-muted)' }}>
          {t('memoFormSaveHint')}
        </p>
      </div>
    </Modal>
  )
}
