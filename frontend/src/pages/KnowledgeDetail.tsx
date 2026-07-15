import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import {
  GET_KNOWLEDGE,
  GET_KNOWLEDGES,
  CREATE_KNOWLEDGE,
  UPDATE_KNOWLEDGE,
  DELETE_KNOWLEDGE,
} from '../graphql/knowledge'
import { GET_TAGS, CREATE_TAG } from '../graphql/tag'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import Modal from '../components/Modal'
import { IconChevLeft } from '../components/Icons'
import TagInput from '../components/knowledge/TagInput'
import KnowledgeMarkdownView from '../components/knowledge/KnowledgeMarkdownView'
import { commitOnEnter } from '../utils/keyboard'

const CLAIM_MAX = 8000

function formatDateTime(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

export default function KnowledgeDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()

  const isNew = id === 'new'

  // form state
  const [title, setTitle] = useState('')
  const [claim, setClaim] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [previewMode, setPreviewMode] = useState(false)
  const [showDelete, setShowDelete] = useState(false)
  const [saving, setSaving] = useState(false)

  // Track if we've loaded existing data into the form
  const [initialized, setInitialized] = useState(isNew)

  const { data: tagsData } = useQuery(GET_TAGS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })
  const availableTags: { id: string; name: string }[] = (
    tagsData?.tags ?? []
  ).map((tag: { id: string; name: string | null }) => ({ id: tag.id, name: tag.name ?? '' }))

  const { data, loading } = useQuery(GET_KNOWLEDGE, {
    variables: { workspaceId: currentWorkspace?.id, id },
    skip: !currentWorkspace || !id || isNew,
  })

  // Initialize the form from loaded data via useEffect rather than the query's
  // onCompleted callback: onCompleted does not fire on an Apollo cache hit, so a
  // cached entry would leave the form blank. useEffect re-syncs on every data
  // change and the `initialized` guard keeps user edits from being clobbered.
  useEffect(() => {
    if (!initialized && data?.knowledge) {
      setTitle(data.knowledge.title)
      setClaim(data.knowledge.claim ?? '')
      setTags(data.knowledge.tags.map((tag: { id: string }) => tag.id))
      setInitialized(true)
    }
  }, [data, initialized])

  const refetchList = [
    { query: GET_KNOWLEDGES, variables: { workspaceId: currentWorkspace?.id } },
    { query: GET_TAGS, variables: { workspaceId: currentWorkspace?.id } },
  ]
  const refetchDetail = isNew
    ? refetchList
    : [
        ...refetchList,
        { query: GET_KNOWLEDGE, variables: { workspaceId: currentWorkspace?.id, id } },
      ]

  const [createKnowledge] = useMutation(CREATE_KNOWLEDGE, { refetchQueries: refetchList })
  const [updateKnowledge] = useMutation(UPDATE_KNOWLEDGE, { refetchQueries: refetchDetail })
  const [deleteKnowledge] = useMutation(DELETE_KNOWLEDGE, { refetchQueries: refetchList })
  const [createTag] = useMutation(CREATE_TAG, {
    refetchQueries: [{ query: GET_TAGS, variables: { workspaceId: currentWorkspace?.id } }],
  })

  // handleCreateTag creates a new tag and returns its id so TagInput can select
  // it. Tags must exist before knowledge can reference them.
  const handleCreateTag = async (name: string): Promise<string> => {
    if (!currentWorkspace) return ''
    const res = await createTag({ variables: { workspaceId: currentWorkspace.id, name } })
    return res.data?.createTag?.id ?? ''
  }

  const knowledge = data?.knowledge

  const titleError = title.trim() === ''
  const tagsError = tags.length === 0
  const claimOverLimit = claim.length > CLAIM_MAX
  const canSave = !titleError && !tagsError && !claimOverLimit && !saving

  const handleSave = async () => {
    if (!canSave || !currentWorkspace) return
    setSaving(true)
    try {
      if (isNew) {
        const result = await createKnowledge({
          variables: {
            workspaceId: currentWorkspace.id,
            input: { title: title.trim(), claim: claim || undefined, tagIds: tags },
          },
        })
        const newId = result.data?.createKnowledge?.id
        if (newId) {
          navigate(`/ws/${currentWorkspace.id}/knowledge/${newId}`, { replace: true })
        }
      } else {
        await updateKnowledge({
          variables: {
            workspaceId: currentWorkspace.id,
            input: { id: id!, title: title.trim(), claim: claim || undefined, tagIds: tags },
          },
        })
      }
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!currentWorkspace || !id) return
    await deleteKnowledge({ variables: { workspaceId: currentWorkspace.id, id } })
    navigate(`/ws/${currentWorkspace.id}/knowledge`)
  }

  const titleKeyDown = commitOnEnter({ onCommit: () => { /* no-op: title is single-line input */ } })
  const claimKeyDown = commitOnEnter({ onCommit: handleSave, requireModifier: true })

  if (!isNew && loading) {
    return <div className="h-main-inner muted">{t('loading')}</div>
  }
  if (!isNew && initialized && !knowledge) {
    return <div className="h-main-inner">{t('errorKnowledgeNotFound')}</div>
  }

  const claimNearLimit = claim.length > CLAIM_MAX * 0.9

  return (
    <div className="h-main-inner" style={{ maxWidth: 900 }}>
      {/* Back + action bar */}
      <div className="row" style={{ marginBottom: 12 }}>
        <Button
          variant="ghost"
          size="sm"
          icon={<IconChevLeft size={13} />}
          onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledge`)}
        >
          {t('btnBack')}
        </Button>
        <span className="spacer" />
        {!isNew && (
          <Button size="sm" variant="danger" onClick={() => setShowDelete(true)}>
            {t('btnDelete')}
          </Button>
        )}
        <Button
          size="sm"
          variant="primary"
          onClick={handleSave}
          disabled={!canSave}
        >
          {saving ? t('btnSaving') : t('btnSave')}
        </Button>
      </div>

      {/* Main content + right rail */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 240px', gap: 16, alignItems: 'start' }}>
        {/* Left: title + claim */}
        <div>
          {/* Title */}
          <div className="card" style={{ padding: '16px 20px', marginBottom: 12 }}>
            <div className="field-label">{t('labelTitle')}</div>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={titleKeyDown}
              placeholder={t('placeholderKnowledgeTitle')}
              style={{
                width: '100%',
                background: 'none',
                border: 'none',
                outline: 'none',
                fontSize: 'var(--t-xl)',
                fontWeight: 600,
                color: 'var(--fg)',
                padding: '4px 0',
              }}
              data-testid="knowledge-title-input"
            />
            {titleError && title !== '' && (
              <div style={{ color: 'var(--danger)', fontSize: 'var(--t-xs)', marginTop: 4 }}>
                {t('errorKnowledgeTitleRequired')}
              </div>
            )}
            {!isNew && knowledge && (
              <div className="row" style={{ gap: 16, marginTop: 8 }}>
                <div>
                  <span className="field-label">{t('labelCreated')}</span>
                  <span className="mono soft" style={{ fontSize: 'var(--t-xs)', marginLeft: 4 }}>
                    {formatDateTime(knowledge.createdAt)}
                  </span>
                </div>
                <div>
                  <span className="field-label">{t('labelUpdated')}</span>
                  <span className="mono soft" style={{ fontSize: 'var(--t-xs)', marginLeft: 4 }}>
                    {formatDateTime(knowledge.updatedAt)}
                  </span>
                </div>
              </div>
            )}
          </div>

          {/* Claim editor */}
          <div className="card" style={{ padding: '16px 20px' }}>
            <div className="row" style={{ marginBottom: 10 }}>
              <div className="field-label" style={{ margin: 0 }}>{t('labelKnowledgeClaim')}</div>
              <span className="spacer" />
              <div className="row" style={{ gap: 4 }}>
                <button
                  type="button"
                  className="btn sm"
                  onClick={() => setPreviewMode(false)}
                  data-testid="knowledge-claim-tab-write"
                  style={{
                    background: !previewMode ? 'var(--accent)' : undefined,
                    color: !previewMode ? 'var(--bg-elev)' : undefined,
                  }}
                >
                  {t('tabWrite')}
                </button>
                <button
                  type="button"
                  className="btn sm"
                  onClick={() => setPreviewMode(true)}
                  data-testid="knowledge-claim-tab-preview"
                  style={{
                    background: previewMode ? 'var(--accent)' : undefined,
                    color: previewMode ? 'var(--bg-elev)' : undefined,
                  }}
                >
                  {t('labelPreview')}
                </button>
              </div>
            </div>

            {previewMode ? (
              <div style={{ minHeight: '12rem' }} data-testid="knowledge-claim-preview">
                <KnowledgeMarkdownView source={claim} placeholder={t('labelPreviewEmpty')} />
              </div>
            ) : (
              <>
                <textarea
                  value={claim}
                  onChange={(e) => setClaim(e.target.value)}
                  onKeyDown={claimKeyDown}
                  placeholder={t('placeholderKnowledgeClaim')}
                  rows={14}
                  style={{
                    width: '100%',
                    resize: 'vertical',
                    background: 'var(--bg-sunken)',
                    border: `1px solid ${claimOverLimit ? 'var(--danger)' : 'var(--line)'}`,
                    borderRadius: 6,
                    padding: 10,
                    fontFamily: 'var(--font-mono)',
                    fontSize: 'var(--t-sm)',
                    color: 'var(--fg)',
                    outline: 'none',
                    lineHeight: 1.6,
                    boxSizing: 'border-box',
                  }}
                  data-testid="knowledge-claim-input"
                />
                <div
                  className="row"
                  style={{
                    marginTop: 6,
                    fontSize: 'var(--t-xs)',
                    color: claimOverLimit ? 'var(--danger)' : claimNearLimit ? 'var(--warn)' : 'var(--fg-soft)',
                  }}
                >
                  <span className="spacer" />
                  {claimOverLimit && (
                    <span style={{ marginRight: 8 }}>{t('errorKnowledgeClaimTooLong')}</span>
                  )}
                  {!claimOverLimit && claimNearLimit && (
                    <span style={{ marginRight: 8 }}>{t('knowledgeCharWarn')}</span>
                  )}
                  <span className="mono">
                    {t('knowledgeCharCount', { count: claim.length, max: CLAIM_MAX })}
                  </span>
                </div>
                <div style={{ fontSize: 'var(--t-xs)', color: 'var(--fg-soft)', marginTop: 2, textAlign: 'right' }}>
                  {t('btnSaveHint')}
                </div>
              </>
            )}
          </div>
        </div>

        {/* Right rail */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {/* Tags */}
          <div className="card" style={{ padding: '14px 16px' }}>
            <div className="field-label" style={{ marginBottom: 8 }}>
              {t('labelKnowledgeTagsRequired')}
            </div>
            <TagInput
              tags={tags}
              onChange={setTags}
              availableTags={availableTags}
              onCreateTag={handleCreateTag}
              error={tagsError}
            />
            {tagsError && (
              <div style={{ color: 'var(--danger)', fontSize: 'var(--t-xs)', marginTop: 6 }}>
                {t('errorKnowledgeTagsRequired')}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Delete confirm dialog */}
      <Modal
        isOpen={showDelete}
        onClose={() => setShowDelete(false)}
        title={t('titleDeleteKnowledge')}
        footer={
          <>
            <Button variant="outline" onClick={() => setShowDelete(false)}>
              {t('btnCancel')}
            </Button>
            <Button variant="danger" onClick={handleDelete}>
              {t('btnDelete')}
            </Button>
          </>
        }
      >
        <p
          dangerouslySetInnerHTML={{
            __html: t('msgDeleteKnowledgeConfirm', { title: knowledge?.title ?? '' }),
          }}
        />
        <p>{t('warningDeleteKnowledgePermanent')}</p>
      </Modal>
    </div>
  )
}
