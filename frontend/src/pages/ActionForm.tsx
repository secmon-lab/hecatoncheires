import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import { CREATE_ACTION, UPDATE_ACTION, GET_ACTIONS, GET_ACTION } from '../graphql/action'
import { GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Modal from '../components/Modal'
import Button from '../components/Button'

interface ActionItem {
  id: number
  caseID: number
  title: string
  description: string
  status: string
  assigneeIDs: string[]
  dueDate?: string | null
}

interface ActionFormProps {
  action: ActionItem | null
  defaultCaseID?: number
  onClose: () => void
}

const STATUSES = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'BLOCKED', 'COMPLETED'] as const

const statusKeyMap = {
  BACKLOG: 'statusBacklog',
  TODO: 'statusTodo',
  IN_PROGRESS: 'statusInProgress',
  BLOCKED: 'statusBlocked',
  COMPLETED: 'statusCompleted',
} as const

export default function ActionForm({ action, defaultCaseID, onClose }: ActionFormProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const isEdit = action !== null

  const [title, setTitle] = useState(action?.title || '')
  const [description, setDescription] = useState(action?.description || '')
  const [caseID, setCaseID] = useState<number | null>(action?.caseID ?? defaultCaseID ?? null)
  const [status, setStatus] = useState(action?.status || 'BACKLOG')
  const [errors, setErrors] = useState<Record<string, string>>({})

  const { data: casesData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' },
    skip: !currentWorkspace,
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  const [createAction, { loading: creating }] = useMutation(CREATE_ACTION, {
    refetchQueries: [{ query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } }],
  })
  const [updateAction, { loading: updating }] = useMutation(UPDATE_ACTION, {
    refetchQueries: action ? [{ query: GET_ACTION, variables: { workspaceId: currentWorkspace?.id, id: action.id } }] : [],
  })

  const submit = async () => {
    const errs: Record<string, string> = {}
    if (!title.trim()) errs.title = t('errorTitleRequired')
    if (!isEdit && !caseID) errs.caseID = t('errorCaseRequired')
    if (Object.keys(errs).length > 0) { setErrors(errs); return }

    try {
      if (isEdit && action) {
        await updateAction({
          variables: {
            workspaceId: currentWorkspace!.id,
            input: { id: action.id, title, description, status },
          },
        })
      } else {
        await createAction({
          variables: {
            workspaceId: currentWorkspace!.id,
            input: { caseID: Number(caseID), title, description, status },
          },
        })
      }
      onClose()
    } catch (e) {
      console.error('Action mutation failed', e)
    }
  }

  const submitting = creating || updating
  const cases = casesData?.cases || []
  const caseOptions = cases.map((c: any) => ({
    value: c.id as number,
    label: c.accessDenied ? `#${c.id}` : `#${c.id} ${c.title}`,
  }))
  const selectedCase = caseOptions.find((o: any) => o.value === caseID) || null

  return (
    <Modal
      open
      onClose={onClose}
      width={560}
      title={isEdit ? t('titleActionFormEdit') : t('titleActionFormNew')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{t('btnCancel')}</Button>
          <Button variant="primary" onClick={submit} disabled={submitting}>
            {submitting ? (isEdit ? t('btnSaving') : t('btnCreating')) : (isEdit ? t('btnSave') : t('btnCreate'))}
          </Button>
        </>
      }
    >
      <div className="col" style={{ gap: 14 }}>
        {!isEdit && (
          <div>
            <label htmlFor="action-case" className="field-label">{t('labelCaseRequired', { caseLabel })}</label>
            <Select
              inputId="action-case"
              aria-label={caseLabel}
              options={caseOptions}
              value={selectedCase}
              onChange={(opt: any) => {
                setCaseID(opt ? opt.value : null)
                if (errors.caseID) setErrors((p) => { const n = { ...p }; delete n.caseID; return n })
              }}
              placeholder={t('placeholderSelectCase', { caseLabelLower: caseLabel.toLowerCase() })}
              isClearable
            />
            {errors.caseID && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.caseID}</div>}
          </div>
        )}
        <div>
          <label htmlFor="title" className="field-label">{t('labelTitleRequired')}</label>
          <input
            id="title"
            className="input"
            value={title}
            onChange={(e) => { setTitle(e.target.value); if (errors.title) setErrors((p) => ({ ...p, title: '' })) }}
            placeholder={t('placeholderActionTitle')}
          />
          {errors.title && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.title}</div>}
        </div>
        <div>
          <label htmlFor="description" className="field-label">{t('labelDescription')}</label>
          <textarea
            id="description"
            className="textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('placeholderActionDescription')}
          />
        </div>
        {isEdit && (
          <div>
            <label htmlFor="action-status" className="field-label">{t('labelStatusRequired')}</label>
            <select
              id="action-status"
              className="select"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            >
              {STATUSES.map((s) => (
                <option key={s} value={s}>{t(statusKeyMap[s])}</option>
              ))}
            </select>
          </div>
        )}
      </div>
    </Modal>
  )
}
