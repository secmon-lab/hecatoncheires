import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import Modal from '../Modal'
import Button from '../Button'
import ChannelSelector from './ChannelSelector'
import { CREATE_SLACK_SOURCE, GET_SOURCES } from '../../graphql/source'
import styles from './source.module.css'

interface SlackFormProps {
  isOpen: boolean
  onClose: () => void
}

interface Channel {
  id: string
  name: string
}

interface FormErrors {
  channels?: string
  name?: string
}

export default function SlackForm({ isOpen, onClose }: SlackFormProps) {
  const navigate = useNavigate()
  const [selectedChannels, setSelectedChannels] = useState<Channel[]>([])
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [errors, setErrors] = useState<FormErrors>({})

  const [createSource, { loading: creating }] = useMutation(CREATE_SLACK_SOURCE, {
    update(cache, { data }) {
      if (!data?.createSlackSource) return
      const existingData = cache.readQuery<{ sources: unknown[] }>({ query: GET_SOURCES })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          data: { sources: [...existingData.sources, data.createSlackSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/sources/${data.createSlackSource.id}`)
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
    setSelectedChannels([])
    setName('')
    setDescription('')
    setEnabled(true)
    setErrors({})
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (selectedChannels.length === 0) {
      newErrors.channels = 'At least one channel is required'
    }

    if (!name.trim()) {
      newErrors.name = 'Name is required'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    await createSource({
      variables: {
        input: {
          channelIDs: selectedChannels.map((c) => c.id),
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
      title="Add Slack Source"
      footer={
        <>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={loading || selectedChannels.length === 0}
          >
            {loading ? 'Creating...' : 'Create Source'}
          </Button>
        </>
      }
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.field}>
          <label htmlFor="name" className={styles.label}>
            Name *
          </label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
            placeholder="Enter source name"
            disabled={loading}
          />
          {errors.name && <span className={styles.error}>{errors.name}</span>}
        </div>

        <div className={styles.field}>
          <label className={styles.label}>Channels *</label>
          <ChannelSelector
            selectedChannels={selectedChannels}
            onChange={setSelectedChannels}
            disabled={loading}
            error={errors.channels}
          />
          <p className={styles.hint}>
            Select the Slack channels to monitor. The bot must be invited to the channels first.
          </p>
        </div>

        <div className={styles.field}>
          <label htmlFor="description" className={styles.label}>
            Description
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className={styles.textarea}
            placeholder="Enter source description (optional)"
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
            <span>Enable this source</span>
          </label>
        </div>
      </form>
    </Modal>
  )
}
