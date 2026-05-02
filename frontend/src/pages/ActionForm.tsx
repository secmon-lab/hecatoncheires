import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import UserSelect from '../components/UserSelect'
import { buildSelectStyles, portalProps } from '../components/selectStyles'
import { CREATE_ACTION, UPDATE_ACTION, GET_ACTIONS, GET_ACTION, GET_OPEN_CASE_ACTIONS } from '../graphql/action'
import { GET_CASE, GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
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
  const [assigneeIDs, setAssigneeIDs] = useState<string[]>(action?.assigneeIDs || [])
  const [errors, setErrors] = useState<Record<string, string>>({})

  const { data: usersData } = useQuery(GET_SLACK_USERS)
  const users = usersData?.slackUsers || []
  const userOptions = users.map((u: any) => ({
    value: u.id as string,
    label: u.realName || u.name,
    name: u.name,
    realName: u.realName,
    imageUrl: u.imageUrl,
  }))
  const selectedAssignees = userOptions.filter((o: any) => assigneeIDs.includes(o.value))

  const { data: casesData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace?.id, status: 'OPEN' },
    skip: !currentWorkspace,
  })
  const { data: configData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  const createRefetch: any[] = [
    { query: GET_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
    { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
  ]
  if (defaultCaseID) {
    createRefetch.push({
      query: GET_CASE,
      variables: { workspaceId: currentWorkspace?.id, id: defaultCaseID },
    })
  }
  const [createAction, { loading: creating }] = useMutation(CREATE_ACTION, {
    refetchQueries: createRefetch,
  })
  const updateRefetch: any[] = [
    { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace?.id } },
  ]
  if (action) {
    updateRefetch.push({
      query: GET_ACTION,
      variables: { workspaceId: currentWorkspace?.id, id: action.id },
    })
    updateRefetch.push({
      query: GET_CASE,
      variables: { workspaceId: currentWorkspace?.id, id: action.caseID },
    })
  }
  const [updateAction, { loading: updating }] = useMutation(UPDATE_ACTION, {
    refetchQueries: updateRefetch,
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
            input: { id: action.id, title, description, status, assigneeIDs },
          },
        })
      } else {
        await createAction({
          variables: {
            workspaceId: currentWorkspace!.id,
            input: { caseID: Number(caseID), title, description, status, assigneeIDs },
          },
        })
      }
      onClose()
    } catch (e: any) {
      console.error('Action mutation failed', e)
      const msg = e?.graphQLErrors?.[0]?.message || e?.message || String(e)
      setErrors({ submit: msg })
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
              classNamePrefix="rs"
              {...portalProps}
              styles={buildSelectStyles()}
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
        <div>
          <label htmlFor="action-assignees" className="field-label">{t('labelAssignees')}</label>
          <UserSelect
            inputId="action-assignees"
            aria-label={t('labelAssignees')}
            isMulti
            options={userOptions}
            value={selectedAssignees}
            onChange={(opts: any) => setAssigneeIDs((opts || []).map((o: any) => o.value))}
            placeholder={t('placeholderAddAssignees')}
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
