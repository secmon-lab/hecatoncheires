import { useState, useEffect } from 'react'
import { useMutation } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
import Modal from '../Modal'
import Button from '../Button'
import ChannelSelector from './ChannelSelector'
import { CREATE_SLACK_SOURCE, GET_SOURCES } from '../../graphql/source'

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
  form?: string
}

export default function SlackForm({ isOpen, onClose }: SlackFormProps) {
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [selectedChannels, setSelectedChannels] = useState<Channel[]>([])
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [errors, setErrors] = useState<FormErrors>({})

  const [createSource, { loading: creating }] = useMutation(CREATE_SLACK_SOURCE, {
    update(cache, { data }) {
      if (!data?.createSlackSource) return
      const existingData = cache.readQuery<{ sources: unknown[] }>({
        query: GET_SOURCES,
        variables: { workspaceId: currentWorkspace!.id },
      })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
          data: { sources: [...existingData.sources, data.createSlackSource] },
        })
      }
    },
    onCompleted: (data) => {
      onClose()
      resetForm()
      navigate(`/ws/${currentWorkspace!.id}/sources/${data.createSlackSource.id}`)
    },
    onError: (error) => {
      setErrors((prev) => ({
        ...prev,
        form: error.message || t('errorCreateSource'),
      }))
    },
  })

  useEffect(() => {
    if (!isOpen) resetForm()
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
    if (selectedChannels.length === 0) newErrors.channels = t('errorChannelsRequired')
    if (!name.trim()) newErrors.name = t('errorNameRequired')
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async () => {
    if (!validate()) return
    await createSource({
      variables: {
        workspaceId: currentWorkspace!.id,
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

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      width={580}
      title={t('titleAddSlackSource')}
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} disabled={creating}>
            {t('btnCancel')}
          </Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={creating || selectedChannels.length === 0}
          >
            {creating ? t('btnCreating') : t('btnCreateSource')}
          </Button>
        </>
      }
    >
      <div className="col" style={{ gap: 14 }}>
        {errors.form && (
          <div style={{
            padding: '8px 10px', borderRadius: 6,
            background: 'color-mix(in oklch, var(--danger) 10%, transparent)',
            border: '1px solid color-mix(in oklch, var(--danger) 30%, transparent)',
            color: 'var(--danger)', fontSize: 12,
          }}>{errors.form}</div>
        )}
        <div>
          <label htmlFor="slack-name" className="field-label">{t('labelNameRequired')}</label>
          <input
            id="slack-name"
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('placeholderSourceName')}
            disabled={creating}
          />
          {errors.name && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 4 }}>{errors.name}</div>}
        </div>
        <div>
          <label className="field-label">{t('labelChannelsRequired')}</label>
          <ChannelSelector
            selectedChannels={selectedChannels}
            onChange={setSelectedChannels}
            disabled={creating}
            error={errors.channels}
          />
          <div className="soft" style={{ fontSize: 11.5, marginTop: 4 }}>
            {t('hintSlackChannels')}
          </div>
        </div>
        <div>
          <label htmlFor="slack-desc" className="field-label">{t('labelDescription')}</label>
          <textarea
            id="slack-desc"
            className="textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('placeholderSourceDescription')}
            rows={3}
            disabled={creating}
          />
        </div>
        <label className="row" style={{ gap: 8, fontSize: 13 }}>
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            disabled={creating}
          />
          <span>{t('labelEnableSource')}</span>
        </label>
      </div>
    </Modal>
  )
}
