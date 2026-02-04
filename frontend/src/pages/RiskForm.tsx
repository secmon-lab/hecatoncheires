import { useState, useEffect } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import Select from 'react-select'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { CREATE_RISK, UPDATE_RISK, GET_RISKS, GET_RISK_CONFIGURATION, GET_SLACK_USERS } from '../graphql/risk'
import styles from './RiskForm.module.css'

interface Risk {
  id: number
  name: string
  description: string
  categoryIDs: string[]
  specificImpact: string
  likelihoodID: string
  impactID: string
  responseTeamIDs: string[]
  assigneeIDs: string[]
  detectionIndicators: string
}

interface RiskFormProps {
  isOpen: boolean
  onClose: () => void
  risk?: Risk | null
}

interface FormErrors {
  name?: string
  description?: string
  likelihoodID?: string
  impactID?: string
}

export default function RiskForm({ isOpen, onClose, risk }: RiskFormProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [categoryIDs, setCategoryIDs] = useState<string[]>([])
  const [specificImpact, setSpecificImpact] = useState('')
  const [likelihoodID, setLikelihoodID] = useState('')
  const [impactID, setImpactID] = useState('')
  const [responseTeamIDs, setResponseTeamIDs] = useState<string[]>([])
  const [assigneeIDs, setAssigneeIDs] = useState<string[]>([])
  const [selectedAssignees, setSelectedAssignees] = useState<Array<{ value: string; label: string; image?: string }>>([])
  const [detectionIndicators, setDetectionIndicators] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})
  const [isAssigneesOpen, setIsAssigneesOpen] = useState(false)

  const { data: configData, loading: configLoading } = useQuery(GET_RISK_CONFIGURATION)
  const { data: usersData, loading: usersLoading } = useQuery(GET_SLACK_USERS, {
    skip: !isAssigneesOpen,
  })

  const [createRisk, { loading: creating }] = useMutation(CREATE_RISK, {
    update(cache, { data }) {
      if (!data?.createRisk) return
      const existingRisks = cache.readQuery<{ risks: Risk[] }>({ query: GET_RISKS })
      if (existingRisks) {
        cache.writeQuery({
          query: GET_RISKS,
          data: { risks: [...existingRisks.risks, data.createRisk] },
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

  const [updateRisk, { loading: updating }] = useMutation(UPDATE_RISK, {
    update(cache, { data }) {
      if (!data?.updateRisk) return
      cache.modify({
        fields: {
          risks(existingRisks = []) {
            return existingRisks.map((riskRef: Risk) =>
              riskRef.id === data.updateRisk.id ? data.updateRisk : riskRef
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
    if (risk) {
      setName(risk.name)
      setDescription(risk.description)
      setCategoryIDs(risk.categoryIDs || [])
      setSpecificImpact(risk.specificImpact || '')
      setLikelihoodID(risk.likelihoodID || '')
      setImpactID(risk.impactID || '')
      setResponseTeamIDs(risk.responseTeamIDs || [])
      setAssigneeIDs(risk.assigneeIDs || [])
      setDetectionIndicators(risk.detectionIndicators || '')
    } else {
      resetForm()
    }
  }, [risk, isOpen])

  const resetForm = () => {
    setName('')
    setDescription('')
    setCategoryIDs([])
    setSpecificImpact('')
    setLikelihoodID('')
    setImpactID('')
    setResponseTeamIDs([])
    setAssigneeIDs([])
    setSelectedAssignees([])
    setDetectionIndicators('')
    setErrors({})
  }

  const validate = () => {
    const newErrors: FormErrors = {}

    if (!name.trim()) {
      newErrors.name = 'Name is required'
    }

    if (!description.trim()) {
      newErrors.description = 'Description is required'
    }

    if (!likelihoodID) {
      newErrors.likelihoodID = 'Likelihood is required'
    }

    if (!impactID) {
      newErrors.impactID = 'Impact is required'
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
            categoryIDs,
            specificImpact: specificImpact.trim(),
            likelihoodID,
            impactID,
            responseTeamIDs,
            assigneeIDs,
            detectionIndicators: detectionIndicators.trim(),
          },
        },
      })
    } else {
      await createRisk({
        variables: {
          input: {
            name: name.trim(),
            description: description.trim(),
            categoryIDs,
            specificImpact: specificImpact.trim(),
            likelihoodID,
            impactID,
            responseTeamIDs,
            assigneeIDs,
            detectionIndicators: detectionIndicators.trim(),
          },
        },
      })
    }
  }

  const handleClose = () => {
    resetForm()
    onClose()
  }

  const loading = creating || updating || configLoading || usersLoading

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

        <div className={styles.field}>
          <label htmlFor="categoryIDs" className={styles.label}>Categories</label>
          <Select
            inputId="categoryIDs"
            isMulti
            value={(configData?.riskConfiguration?.categories || [])
              .filter((cat: { id: string; name: string }) => categoryIDs.includes(cat.id))
              .map((cat: { id: string; name: string }) => ({ value: cat.id, label: cat.name }))}
            onChange={(selected) => {
              setCategoryIDs(selected ? selected.map(s => s.value) : [])
            }}
            options={(configData?.riskConfiguration?.categories || []).map((cat: { id: string; name: string }) => ({
              value: cat.id,
              label: cat.name,
            }))}
            isDisabled={loading}
            placeholder="Select categories..."
            menuPortalTarget={document.body}
            styles={{ menuPortal: (base) => ({ ...base, zIndex: 10000 }) }}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="specificImpact" className={styles.label}>
            Specific Impact
          </label>
          <textarea
            id="specificImpact"
            value={specificImpact}
            onChange={(e) => setSpecificImpact(e.target.value)}
            className={styles.textarea}
            placeholder="Describe the specific impact of this risk"
            rows={3}
            disabled={loading}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="likelihoodID" className={styles.label}>
            Likelihood *
          </label>
          <select
            id="likelihoodID"
            value={likelihoodID}
            onChange={(e) => setLikelihoodID(e.target.value)}
            className={`${styles.select} ${errors.likelihoodID ? styles.inputError : ''}`}
            disabled={loading}
          >
            <option value="">Select likelihood level</option>
            {(configData?.riskConfiguration?.likelihoodLevels || []).map((level: { id: string; name: string; score: number }) => (
              <option key={level.id} value={level.id}>
                {level.name} (Score: {level.score})
              </option>
            ))}
          </select>
          {errors.likelihoodID && <span className={styles.error}>{errors.likelihoodID}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="impactID" className={styles.label}>
            Impact *
          </label>
          <select
            id="impactID"
            value={impactID}
            onChange={(e) => setImpactID(e.target.value)}
            className={`${styles.select} ${errors.impactID ? styles.inputError : ''}`}
            disabled={loading}
          >
            <option value="">Select impact level</option>
            {(configData?.riskConfiguration?.impactLevels || []).map((level: { id: string; name: string; score: number }) => (
              <option key={level.id} value={level.id}>
                {level.name} (Score: {level.score})
              </option>
            ))}
          </select>
          {errors.impactID && <span className={styles.error}>{errors.impactID}</span>}
        </div>

        <div className={styles.field}>
          <label htmlFor="responseTeamIDs" className={styles.label}>Response Teams</label>
          <Select
            inputId="responseTeamIDs"
            isMulti
            value={(configData?.riskConfiguration?.teams || [])
              .filter((team: { id: string; name: string }) => responseTeamIDs.includes(team.id))
              .map((team: { id: string; name: string }) => ({ value: team.id, label: team.name }))}
            onChange={(selected) => {
              setResponseTeamIDs(selected ? selected.map(s => s.value) : [])
            }}
            options={(configData?.riskConfiguration?.teams || []).map((team: { id: string; name: string }) => ({
              value: team.id,
              label: team.name,
            }))}
            isDisabled={loading}
            placeholder="Select response teams..."
            menuPortalTarget={document.body}
            styles={{ menuPortal: (base) => ({ ...base, zIndex: 10000 }) }}
          />
        </div>

        <div className={styles.field}>
          <label htmlFor="assigneeIDs" className={styles.label}>Assignees</label>
          <Select
            inputId="assigneeIDs"
            isMulti
            value={selectedAssignees}
            onChange={(selected) => {
              const selectedOptions = selected || []
              setSelectedAssignees(selectedOptions)
              setAssigneeIDs(selectedOptions.map(s => s.value))
            }}
            options={(usersData?.slackUsers || []).map((user: { id: string; realName: string; imageUrl?: string }) => ({
              value: user.id,
              label: user.realName,
              image: user.imageUrl,
            }))}
            isDisabled={loading}
            isLoading={usersLoading}
            loadingMessage={() => 'Loading users...'}
            placeholder="Select assignees..."
            menuPortalTarget={document.body}
            styles={{ menuPortal: (base) => ({ ...base, zIndex: 10000 }) }}
            onMenuOpen={() => setIsAssigneesOpen(true)}
            onMenuClose={() => setIsAssigneesOpen(false)}
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
          <label htmlFor="detectionIndicators" className={styles.label}>
            Detection Indicators / Triggers
          </label>
          <textarea
            id="detectionIndicators"
            value={detectionIndicators}
            onChange={(e) => setDetectionIndicators(e.target.value)}
            className={styles.textarea}
            placeholder="Describe detection indicators or triggers for this risk"
            rows={3}
            disabled={loading}
          />
        </div>
      </form>
    </Modal>
  )
}
