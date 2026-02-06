import { useState, useEffect } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import Modal from '../components/Modal'
import Button from '../components/Button'
import CustomFieldRenderer from '../components/fields/CustomFieldRenderer'
import { CREATE_CASE, UPDATE_CASE, GET_CASES } from '../graphql/case'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import styles from './CaseForm.module.css'

interface Case {
  id: number
  title: string
  description: string
  assigneeIDs: string[]
  assignees: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  fields: Array<{ fieldId: string; value: any }>
}

interface CaseFormProps {
  isOpen: boolean
  onClose: () => void
  caseItem?: Case | null
}

interface FormErrors {
  title?: string
  description?: string
  [key: string]: string | undefined
}

export default function CaseForm({ isOpen, onClose, caseItem }: CaseFormProps) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [assigneeIDs, setAssigneeIDs] = useState<string[]>([])
  const [selectedAssignees, setSelectedAssignees] = useState<Array<{ value: string; label: string; image?: string }>>([])
  const [fieldValues, setFieldValues] = useState<Record<string, any>>({})
  const [errors, setErrors] = useState<FormErrors>({})

  const { data: configData, loading: configLoading } = useQuery(GET_FIELD_CONFIGURATION)
  const { data: usersData } = useQuery(GET_SLACK_USERS)

  const [createCase, { loading: creating }] = useMutation(CREATE_CASE, {
    update(cache, { data }) {
      if (!data?.createCase) return
      const existingCases = cache.readQuery<{ cases: Case[] }>({ query: GET_CASES })
      if (existingCases) {
        cache.writeQuery({
          query: GET_CASES,
          data: { cases: [...existingCases.cases, data.createCase] },
        })
      }
    },
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Create error:', error)
    },
  })

  const [updateCase, { loading: updating }] = useMutation(UPDATE_CASE, {
    update(cache, { data }) {
      if (!data?.updateCase) return
      cache.modify({
        fields: {
          cases(existingCases = []) {
            return existingCases.map((caseRef: Case) =>
              caseRef.id === data.updateCase.id ? data.updateCase : caseRef
            )
          },
        },
      })
    },
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Update error:', error)
    },
  })

  useEffect(() => {
    if (caseItem) {
      setTitle(caseItem.title)
      setDescription(caseItem.description)
      setAssigneeIDs(caseItem.assigneeIDs || [])
      setSelectedAssignees(
        (caseItem.assignees || []).map((a) => ({
          value: a.id,
          label: a.realName || a.name,
          image: a.imageUrl,
        }))
      )
      const values: Record<string, any> = {}
      caseItem.fields.forEach((f) => {
        values[f.fieldId] = f.value
      })
      setFieldValues(values)
    } else {
      resetForm()
    }
  }, [caseItem, isOpen])

  const resetForm = () => {
    setTitle('')
    setDescription('')
    setAssigneeIDs([])
    setSelectedAssignees([])
    setFieldValues({})
    setErrors({})
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!title.trim()) {
      newErrors.title = 'Title is required'
    }

    if (!description.trim()) {
      newErrors.description = 'Description is required'
    }

    // Validate custom fields
    const fieldDefs = configData?.fieldConfiguration?.fields || []
    fieldDefs.forEach((field: any) => {
      if (field.required) {
        const value = fieldValues[field.id]
        if (value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0)) {
          newErrors[field.id] = `${field.name} is required`
        }
      }
    })

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleFieldChange = (fieldID: string, value: any) => {
    setFieldValues((prev) => ({
      ...prev,
      [fieldID]: value,
    }))
    // Clear error for this field
    if (errors[fieldID]) {
      setErrors((prev) => {
        const newErrors = { ...prev }
        delete newErrors[fieldID]
        return newErrors
      })
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    // Convert field values to GraphQL input format
    const fields = Object.entries(fieldValues).map(([fieldId, value]) => ({
      fieldId,
      value,
    }))

    if (caseItem) {
      await updateCase({
        variables: {
          input: {
            id: caseItem.id,
            title: title.trim(),
            description: description.trim(),
            assigneeIDs,
            fields,
          },
        },
      })
    } else {
      await createCase({
        variables: {
          input: {
            title: title.trim(),
            description: description.trim(),
            assigneeIDs,
            fields,
          },
        },
      })
    }
  }

  const handleClose = () => {
    resetForm()
    onClose()
  }

  const loading = creating || updating || configLoading
  const fieldDefs = configData?.fieldConfiguration?.fields || []
  const caseLabel = configData?.fieldConfiguration?.labels?.case || 'Case'

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={caseItem ? `Edit ${caseLabel}` : `New ${caseLabel}`}
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleSubmit} disabled={loading}>
            {loading ? 'Saving...' : 'Save'}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.field}>
          <label htmlFor="title" className={styles.label}>
            Title *
          </label>
          <input
            id="title"
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className={`${styles.input} ${errors.title ? styles.inputError : ''}`}
            placeholder={`Enter ${caseLabel.toLowerCase()} title`}
            disabled={loading}
          />
          {errors.title && <span className={styles.error}>{errors.title}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="description" className={styles.label}>
            Description *
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className={`${styles.textarea} ${errors.description ? styles.inputError : ''}`}
            placeholder={`Enter ${caseLabel.toLowerCase()} description`}
            rows={4}
            disabled={loading}
          />
          {errors.description && (
            <span className={styles.error}>{errors.description}</span>
          )}
        </div>

        <div className={styles.field}>
          <label htmlFor="assigneeIDs" className={styles.label}>Assignees</label>
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
            placeholder="Select assignees..."
            filterOption={(option, inputValue) => {
              const search = inputValue.toLowerCase()
              const data = option.data as { label: string; name: string; realName: string }
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

        {fieldDefs.map((field: any) => (
          <CustomFieldRenderer
            key={field.id}
            field={field}
            value={fieldValues[field.id]}
            onChange={handleFieldChange}
            users={usersData?.slackUsers || []}
            error={errors[field.id]}
            disabled={loading}
            showMetadata={true}
          />
        ))}
      </form>
    </Modal>
  )
}
