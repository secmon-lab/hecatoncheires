import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { CREATE_RISK, UPDATE_RISK } from '../graphql/risk'
import styles from './RiskForm.module.css'

interface Risk {
  id: number
  name: string
  description: string
}

interface RiskFormProps {
  isOpen: boolean
  onClose: () => void
  risk?: Risk | null
}

export default function RiskForm({ isOpen, onClose, risk }: RiskFormProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [errors, setErrors] = useState<{ name?: string; description?: string }>({})

  const [createRisk, { loading: creating }] = useMutation(CREATE_RISK, {
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Create error:', error)
    },
  })

  const [updateRisk, { loading: updating }] = useMutation(UPDATE_RISK, {
    onCompleted: () => {
      onClose()
      resetForm()
    },
    onError: (error) => {
      console.error('Update error:', error)
    },
  })

  useEffect(() => {
    if (risk) {
      setName(risk.name)
      setDescription(risk.description)
    } else {
      resetForm()
    }
  }, [risk, isOpen])

  const resetForm = () => {
    setName('')
    setDescription('')
    setErrors({})
  }

  const validate = () => {
    const newErrors: { name?: string; description?: string } = {}

    if (!name.trim()) {
      newErrors.name = 'Name is required'
    }

    if (!description.trim()) {
      newErrors.description = 'Description is required'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validate()) {
      return
    }

    if (risk) {
      await updateRisk({
        variables: {
          input: {
            id: risk.id,
            name: name.trim(),
            description: description.trim(),
          },
        },
      })
    } else {
      await createRisk({
        variables: {
          input: {
            name: name.trim(),
            description: description.trim(),
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

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={risk ? 'Edit Risk' : 'New Risk'}
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
          <label htmlFor="name" className={styles.label}>
            Name *
          </label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={`${styles.input} ${errors.name ? styles.inputError : ''}`}
            placeholder="Enter risk name"
            disabled={loading}
          />
          {errors.name && <span className={styles.error}>{errors.name}</span>}
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
            placeholder="Enter risk description"
            rows={4}
            disabled={loading}
          />
          {errors.description && (
            <span className={styles.error}>{errors.description}</span>
          )}
        </div>
      </form>
    </Modal>
  )
}
