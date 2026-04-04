import { useState, useEffect } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { CREATE_ACTION, UPDATE_ACTION, GET_OPEN_CASE_ACTIONS } from '../graphql/action'
import { GET_CASES } from '../graphql/case'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './ActionForm.module.css'

interface Action {
  id: number
  caseID: number
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  slackMessageTS: string
  status: string
  dueDate?: string
}

interface ActionFormProps {
  isOpen: boolean
  onClose: () => void
  action?: Action | null
  initialCaseID?: number
}

interface FormErrors {
  caseID?: string
  title?: string
}

export default function ActionForm({ isOpen, onClose, action, initialCaseID }: ActionFormProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [caseID, setCaseID] = useState<number | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [assigneeIDs, setAssigneeIDs] = useState<string[]>([])
  const [selectedAssignees, setSelectedAssignees] = useState<Array<{ value: string; label: string; image?: string }>>([])
  const [status, setStatus] = useState('BACKLOG')
  const [dueDate, setDueDate] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})

  const { data: casesData } = useQuery(GET_CASES, {
    variables: { workspaceId: currentWorkspace!.id, status: 'OPEN' },
    skip: !currentWorkspace,
  })
  const { data: usersData } = useQuery(GET_SLACK_USERS)
  const [createAction, { loading: creating }] = useMutation(CREATE_ACTION, {
    refetchQueries: [
      { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace!.id } },
    ],
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Create error:', error)
    },
  })

  const [updateAction, { loading: updating }] = useMutation(UPDATE_ACTION, {
    refetchQueries: [
      { query: GET_OPEN_CASE_ACTIONS, variables: { workspaceId: currentWorkspace!.id } },
    ],
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Update error:', error)
    },
  })

  useEffect(() => {
    if (action) {
      setCaseID(action.caseID)
      setTitle(action.title)
      setDescription(action.description)
      setAssigneeIDs(action.assigneeIDs || [])
      setSelectedAssignees(
        (action.assignees || []).map((a) => ({
          value: a.id,
          label: a.realName || a.name,
          image: a.imageUrl,
        }))
      )
      setStatus(action.status || 'BACKLOG')
      setDueDate(action.dueDate ? action.dueDate.split('T')[0] : '')
    } else if (initialCaseID) {
      setCaseID(initialCaseID)
      resetForm(false)
    } else {
      resetForm()
    }
  }, [action, initialCaseID, isOpen])

  const resetForm = (resetCaseID = true) => {
    if (resetCaseID) {
      setCaseID(null)
    }
    setTitle('')
    setDescription('')
    setAssigneeIDs([])
    setSelectedAssignees([])
    setStatus('BACKLOG')
    setDueDate('')
    setErrors({})
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!caseID) {
      newErrors.caseID = t('errorCaseRequired')
    }

    if (!title.trim()) {
      newErrors.title = t('errorTitleRequired')
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    if (action) {
      await updateAction({
        variables: {
          workspaceId: currentWorkspace!.id,
          input: {
            id: action.id,
            caseID: caseID!,
            title: title.trim(),
            description: description.trim(),
            assigneeIDs,
            status,
            dueDate: dueDate ? new Date(dueDate).toISOString() : undefined,
            clearDueDate: !dueDate && !!action.dueDate,
          },
        },
      })
    } else {
      await createAction({
        variables: {
          workspaceId: currentWorkspace!.id,
          input: {
            caseID: caseID!,
            title: title.trim(),
            description: description.trim(),
            assigneeIDs,
            status,
            dueDate: dueDate ? new Date(dueDate).toISOString() : undefined,
          },
        },
      })
    }
  }

  const handleClose = () => {
    resetForm()
    onClose()
  }

  const loading = creating || updating

  const caseOptions = (casesData?.cases || []).map((c: any) => ({
    value: c.id,
    label: `${c.title} (ID: ${c.id})`,
  }))

  const statusOptions = [
    { value: 'BACKLOG', label: t('statusBacklog') },
    { value: 'TODO', label: t('statusTodo') },
    { value: 'IN_PROGRESS', label: t('statusInProgress') },
    { value: 'BLOCKED', label: t('statusBlocked') },
    { value: 'COMPLETED', label: t('statusCompleted') },
  ]

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={action ? t('titleActionFormEdit') : t('titleActionFormNew')}
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            {t('btnCancel')}
          </Button>
          <Button variant="primary" onClick={handleSubmit} disabled={loading}>
            {loading ? t('btnSaving') : t('btnSave')}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.field}>
          <label htmlFor="caseID" className={styles.label}>
            {t('labelCaseRequired', { caseLabel: t('navCases') })}
          </label>
          <Select
            inputId="caseID"
            value={caseOptions.find((opt: any) => opt.value === caseID)}
            onChange={(selected) => setCaseID(selected?.value || null)}
            options={caseOptions}
            isDisabled={loading}
            placeholder={t('placeholderSelectCase', { caseLabelLower: t('navCases').toLowerCase() })}
          />
          {errors.caseID && <span className={styles.error}>{errors.caseID}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="title" className={styles.label}>
            {t('labelTitleRequired')}
          </label>
          <input
            id="title"
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className={`${styles.input} ${errors.title ? styles.inputError : ''}`}
            placeholder={t('placeholderActionTitle')}
            disabled={loading}
          />
          {errors.title && <span className={styles.error}>{errors.title}</span>}
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
            placeholder={t('placeholderActionDescription')}
            rows={4}
            disabled={loading}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="assigneeIDs" className={styles.label}>{t('labelAssignees')}</label>
          <Select
            inputId="assigneeIDs"
            isMulti
            isClearable
            value={selectedAssignees}
            onChange={(selected) => {
              const selectedOptions = [...(selected || [])]
              setSelectedAssignees(selectedOptions)
              setAssigneeIDs(selectedOptions.map(s => s.value))
            }}
            options={(usersData?.slackUsers || []).map((user: { id: string; name: string; realName: string; imageUrl?: string }) => ({
              value: user.id,
              label: user.realName || user.name,
              name: user.name,
              realName: user.realName,
              image: user.imageUrl,
            }))}
            isDisabled={loading}
            placeholder={t('placeholderSelectAssignees')}
            filterOption={(option, inputValue) => {
              const search = inputValue.toLowerCase()
              const data = option.data as unknown as { label: string; name: string; realName: string }
              return (
                data.label.toLowerCase().includes(search) ||
                data.name.toLowerCase().includes(search) ||
                data.realName.toLowerCase().includes(search)
              )
            }}
            formatOptionLabel={(option: { value: string; label: string; image?: string }) => (
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                {option.image && (
                  <img
                    src={option.image}
                    alt={option.label}
                    style={{ width: '24px', height: '24px', borderRadius: '50%' }}
                  />
                )}
                <span>{option.label}</span>
              </div>
            )}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="status" className={styles.label}>
            {t('labelStatusRequired')}
          </label>
          <Select
            inputId="status"
            value={statusOptions.find((opt) => opt.value === status)}
            onChange={(selected) => setStatus(selected?.value || 'BACKLOG')}
            options={statusOptions}
            isDisabled={loading}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="dueDate" className={styles.label}>
            {t('labelDueDate')}
          </label>
          <input
            id="dueDate"
            type="date"
            value={dueDate}
            onChange={(e) => setDueDate(e.target.value)}
            className={styles.input}
            disabled={loading}
          />
        </div>

      </form>
    </Modal>
  )
}
