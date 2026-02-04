import { useState, useEffect } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import Modal from '../components/Modal'
import Button from '../components/Button'
import {
  CREATE_RESPONSE,
  UPDATE_RESPONSE,
  GET_RESPONSES,
  GET_SLACK_USERS,
} from '../graphql/response'
import { GET_RISKS } from '../graphql/risk'
import styles from './ResponseForm.module.css'

interface Response {
  id: number
  title: string
  description: string
  responders: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  url?: string
  status: string
  risks: Array<{ id: number; name: string }>
}

interface ResponseFormProps {
  response?: Response | null
  initialRiskID?: number
  onClose: () => void
}

interface FormErrors {
  title?: string
  description?: string
}

export default function ResponseForm({ response, initialRiskID, onClose }: ResponseFormProps) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [responderIDs, setResponderIDs] = useState<string[]>([])
  const [selectedResponders, setSelectedResponders] = useState<Array<{ value: string; label: string; image?: string }>>([])
  const [url, setUrl] = useState('')
  const [status, setStatus] = useState('BACKLOG')
  const [riskIDs, setRiskIDs] = useState<number[]>([])
  const [errors, setErrors] = useState<FormErrors>({})
  const [isRespondersOpen, setIsRespondersOpen] = useState(false)

  const { data: usersData, loading: usersLoading } = useQuery(GET_SLACK_USERS, {
    skip: !isRespondersOpen,
  })
  const { data: risksData, loading: risksLoading } = useQuery(GET_RISKS)

  const [createResponse, { loading: creating }] = useMutation(CREATE_RESPONSE, {
    update(cache, { data }) {
      if (!data?.createResponse) return
      const existingResponses = cache.readQuery<{ responses: Response[] }>({
        query: GET_RESPONSES,
      })
      if (existingResponses) {
        cache.writeQuery({
          query: GET_RESPONSES,
          data: { responses: [...existingResponses.responses, data.createResponse] },
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

  const [updateResponse, { loading: updating }] = useMutation(UPDATE_RESPONSE, {
    update(cache, { data }) {
      if (!data?.updateResponse) return
      const existing = cache.readQuery<{ responses: Response[] }>({ query: GET_RESPONSES })
      if (existing) {
        cache.writeQuery({
          query: GET_RESPONSES,
          data: {
            responses: existing.responses.map((r) =>
              r.id === data.updateResponse.id ? data.updateResponse : r
            ),
          },
        })
      }
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
    if (response) {
      setTitle(response.title)
      setDescription(response.description)
      setResponderIDs(response.responders.map((r) => r.id))
      setSelectedResponders(
        response.responders.map((r) => ({
          value: r.id,
          label: r.realName,
          image: r.imageUrl,
        }))
      )
      setUrl(response.url || '')
      setStatus(response.status)
      setRiskIDs(response.risks.map((r) => r.id))
    } else {
      resetForm()
      if (initialRiskID) {
        setRiskIDs([initialRiskID])
      }
    }
  }, [response, initialRiskID])

  const resetForm = () => {
    setTitle('')
    setDescription('')
    setResponderIDs([])
    setSelectedResponders([])
    setUrl('')
    setStatus('BACKLOG')
    setRiskIDs([])
    setErrors({})
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!title.trim()) {
      newErrors.title = 'Title is required'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    if (response) {
      await updateResponse({
        variables: {
          input: {
            id: response.id,
            title: title.trim(),
            description: description.trim(),
            responderIDs,
            url: url.trim(),
            status,
            riskIDs,
          },
        },
      })
    } else {
      await createResponse({
        variables: {
          input: {
            title: title.trim(),
            description: description.trim(),
            responderIDs,
            url: url.trim(),
            status,
            riskIDs,
          },
        },
      })
    }
  }

  const handleClose = () => {
    resetForm()
    onClose()
  }

  const loading = creating || updating || usersLoading || risksLoading

  return (
    <Modal
      isOpen={true}
      onClose={handleClose}
      title={response ? 'Edit Response' : 'New Response'}
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
            placeholder="Enter response title"
            disabled={loading}
          />
          {errors.title && <span className={styles.error}>{errors.title}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="description" className={styles.label}>
            Description
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className={`${styles.textarea} ${errors.description ? styles.inputError : ''}`}
            placeholder="Enter response description"
            rows={4}
            disabled={loading}
          />
          {errors.description && <span className={styles.error}>{errors.description}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="status" className={styles.label}>
            Status
          </label>
          <select
            id="status"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className={styles.select}
            disabled={loading}
          >
            <option value="BACKLOG">Backlog</option>
            <option value="TODO">To Do</option>
            <option value="IN_PROGRESS">In Progress</option>
            <option value="BLOCKED">Blocked</option>
            <option value="COMPLETED">Completed</option>
            <option value="ABANDONED">Abandoned</option>
          </select>
        </div>

        <div className={styles.field}>
          <label htmlFor="responderIDs" className={styles.label}>
            Responders
          </label>
          <Select
            inputId="responderIDs"
            isMulti
            value={selectedResponders}
            onChange={(selected) => {
              const selectedOptions = selected || []
              setSelectedResponders(selectedOptions)
              setResponderIDs(selectedOptions.map((s) => s.value))
            }}
            options={(usersData?.slackUsers || []).map(
              (user: { id: string; realName: string; imageUrl?: string }) => ({
                value: user.id,
                label: user.realName,
                image: user.imageUrl,
              })
            )}
            isDisabled={loading}
            isLoading={usersLoading}
            loadingMessage={() => 'Loading users...'}
            placeholder="Select responders..."
            menuPortalTarget={document.body}
            styles={{ menuPortal: (base) => ({ ...base, zIndex: 10000 }) }}
            onMenuOpen={() => setIsRespondersOpen(true)}
            onMenuClose={() => setIsRespondersOpen(false)}
            formatOptionLabel={(option: { value: string; label: string; image?: string }) => (
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                {option.image && (
                  <img
                    src={option.image}
                    alt={option.label}
                    style={{ width: '24px', height: '24px', borderRadius: '4px' }}
                  />
                )}
                <span>{option.label}</span>
              </div>
            )}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="url" className={styles.label}>
            URL
          </label>
          <input
            id="url"
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            className={styles.input}
            placeholder="https://..."
            disabled={loading}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="riskIDs" className={styles.label}>
            Related Risks
          </label>
          <Select
            inputId="riskIDs"
            isMulti
            value={(risksData?.risks || [])
              .filter((risk: { id: number; name: string }) => riskIDs.includes(risk.id))
              .map((risk: { id: number; name: string }) => ({
                value: risk.id,
                label: risk.name,
              }))}
            onChange={(selected) => {
              setRiskIDs(selected ? selected.map((s) => s.value) : [])
            }}
            options={(risksData?.risks || []).map((risk: { id: number; name: string }) => ({
              value: risk.id,
              label: risk.name,
            }))}
            isDisabled={loading}
            placeholder="Select related risks..."
            menuPortalTarget={document.body}
            styles={{ menuPortal: (base) => ({ ...base, zIndex: 10000 }) }}
          />
        </div>
      </form>
    </Modal>
  )
}
