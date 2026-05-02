import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import UserSelect from '../components/UserSelect'
import { CREATE_CASE, UPDATE_CASE, GET_CASE, GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Modal from '../components/Modal'
import Button from '../components/Button'
import CustomFieldRenderer from '../components/fields/CustomFieldRenderer'
import { IconLock } from '../components/Icons'
import { sanitizeFieldValues } from '../utils/sanitizeFieldValues'

interface User {
  id: string
  name: string
  realName: string
  imageUrl?: string
}

interface CaseItem {
  id: number
  title: string
  description: string
  isPrivate: boolean
  assigneeIDs: string[]
  fields: Array<{ fieldId: string; value: any }>
}

interface CaseFormProps {
  caseItem: CaseItem | null
  onClose: () => void
}

export default function CaseForm({ caseItem, onClose }: CaseFormProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const isEdit = caseItem !== null
  const [title, setTitle] = useState(caseItem?.title || '')
  const [description, setDescription] = useState(caseItem?.description || '')
  const [isPrivate, setIsPrivate] = useState(caseItem?.isPrivate ?? false)
  const [assigneeIDs, setAssigneeIDs] = useState<string[]>(caseItem?.assigneeIDs || [])
  const [fieldValues, setFieldValues] = useState<Record<string, any>>(() => {
    const map: Record<string, any> = {}
    caseItem?.fields?.forEach((f) => { map[f.fieldId] = f.value })
    return map
  })
  const [errors, setErrors] = useState<Record<string, string>>({})

  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const { data: usersData } = useQuery(GET_SLACK_USERS)
  const users: User[] = usersData?.slackUsers || []

  const [createCase, { loading: creating }] = useMutation(CREATE_CASE, {
    refetchQueries: [{ query: GET_CASES, variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' } }],
  })
  const [updateCase, { loading: updating }] = useMutation(UPDATE_CASE, {
    refetchQueries: caseItem ? [{ query: GET_CASE, variables: { workspaceId: currentWorkspace?.id, id: caseItem.id } }] : [],
  })

  const fields = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  const handleFieldChange = (fieldID: string, value: any) => {
    setFieldValues((prev) => ({ ...prev, [fieldID]: value }))
    if (errors[fieldID]) {
      setErrors((prev) => { const n = { ...prev }; delete n[fieldID]; return n })
    }
  }

  const handleSubmit = async () => {
    const errs: Record<string, string> = {}
    if (!title.trim()) errs.title = t('errorTitleRequired')
    fields.forEach((f: any) => {
      if (f.required && (fieldValues[f.id] === undefined || fieldValues[f.id] === null || fieldValues[f.id] === '')) {
        errs[f.id] = t('errorFieldRequired', { fieldName: f.name })
      }
    })
    if (Object.keys(errs).length > 0) { setErrors(errs); return }

    const fieldArr = sanitizeFieldValues(
      Object.entries(fieldValues)
        .filter(([, v]) => v !== undefined && v !== null && v !== '')
        .map(([fieldId, value]) => ({ fieldId, value })),
      fields,
    )

    try {
      if (isEdit && caseItem) {
        await updateCase({
          variables: {
            workspaceId: currentWorkspace!.id,
            input: {
              id: caseItem.id,
              title,
              description,
              assigneeIDs,
              fields: fieldArr,
            },
          },
        })
      } else {
        await createCase({
          variables: {
            workspaceId: currentWorkspace!.id,
            input: {
              title,
              description,
              isPrivate,
              assigneeIDs,
              fields: fieldArr,
            },
          },
        })
      }
      onClose()
    } catch (e: any) {
      console.error('Case mutation failed', e)
      const msg = e?.graphQLErrors?.[0]?.message || e?.message || String(e)
      setErrors({ submit: msg })
    }
  }

  const userOptions = users.map((u) => ({
    value: u.id,
    label: u.realName || u.name,
    name: u.name,
    realName: u.realName,
    imageUrl: u.imageUrl,
  }))
  const selectedAssignees = userOptions.filter((o) => assigneeIDs.includes(o.value))

  const submitting = creating || updating

  return (
    <Modal
      open
      onClose={onClose}
      width={580}
      title={isEdit ? t('titleCaseFormEdit', { caseLabel }) : t('titleCaseFormNew', { caseLabel })}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{t('btnCancel')}</Button>
          <Button variant="primary" onClick={handleSubmit} disabled={submitting}>
            {submitting ? (isEdit ? t('btnSaving') : t('btnCreating')) : (isEdit ? t('btnSave') : t('btnCreate'))}
          </Button>
        </>
      }
    >
      <div className="col" style={{ gap: 14 }}>
        <div>
          <div className="field-label">{t('labelTitleRequired')}</div>
          <input
            className="input"
            value={title}
            onChange={(e) => { setTitle(e.target.value); if (errors.title) setErrors((p) => ({ ...p, title: '' })) }}
            placeholder={t('placeholderCaseTitle', { caseLabelLower: caseLabel.toLowerCase() })}
            data-testid="case-title-input"
          />
          {errors.title && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.title}</div>}
        </div>
        <div>
          <div className="field-label">{t('labelDescription')}</div>
          <textarea
            className="textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('placeholderCaseDescription', { caseLabelLower: caseLabel.toLowerCase() })}
            data-testid="case-description-input"
          />
        </div>
        <div>
          <label htmlFor="case-assignees" className="field-label">{t('labelAssignees')}</label>
          <UserSelect
            inputId="case-assignees"
            aria-label={t('labelAssignees')}
            isMulti
            options={userOptions}
            value={selectedAssignees}
            onChange={(opts: any) => setAssigneeIDs((opts || []).map((o: any) => o.value))}
            placeholder={t('placeholderSelectAssignees')}
          />
        </div>
        {fields.length > 0 && (
          <div>
            <div className="field-label">{t('sectionFields')}</div>
            <div className="col" style={{ gap: 12 }}>
              {fields.map((f: any) => (
                <CustomFieldRenderer
                  key={f.id}
                  field={f}
                  value={fieldValues[f.id]}
                  onChange={handleFieldChange}
                  users={users}
                  error={errors[f.id]}
                  disabled={submitting}
                />
              ))}
            </div>
          </div>
        )}
        {!isEdit && (
          <label className="row" style={{ gap: 8, padding: 10, border: '1px solid color-mix(in oklch, var(--warn) 30%, var(--line))', borderRadius: 6, background: 'color-mix(in oklch, var(--warn) 8%, transparent)', alignItems: 'flex-start', cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={isPrivate}
              onChange={(e) => setIsPrivate(e.target.checked)}
              style={{ marginTop: 2 }}
              data-testid="private-case-checkbox"
            />
            <div>
              <div className="row" style={{ gap: 6, fontSize: 13, fontWeight: 500 }}>
                <IconLock size={12} sw={2} />
                {t('labelPrivateCase', { caseLabel })}
              </div>
              <div style={{ fontSize: 11.5, color: 'var(--fg-muted)', marginTop: 2 }}>
                {t('hintPrivateCase', { caseLabelLower: caseLabel.toLowerCase() })}
              </div>
            </div>
          </label>
        )}
        {errors.submit && (
          <div style={{
            padding: '8px 10px',
            borderRadius: 6,
            background: 'color-mix(in oklch, var(--danger) 10%, transparent)',
            border: '1px solid color-mix(in oklch, var(--danger) 30%, transparent)',
            color: 'var(--danger)',
            fontSize: 12,
          }}>
            {errors.submit}
          </div>
        )}
      </div>
    </Modal>
  )
}
