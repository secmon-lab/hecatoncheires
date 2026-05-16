import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client'
import {
  DISCARD_DRAFT,
  GET_DRAFT,
  GET_DRAFTS,
  SUBMIT_DRAFT,
} from '../graphql/drafts'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'

interface DraftFieldValue {
  fieldId: string
  value: unknown
}

interface DraftDetailModel {
  id: number
  title: string
  description: string | null
  status: 'DRAFT' | 'OPEN' | 'CLOSED'
  isPrivate: boolean
  reporterID: string
  assigneeIDs: string[]
  createdAt: string
  updatedAt: string
  fields: DraftFieldValue[]
}

interface FieldDef {
  id: string
  name: string
  type: string
}

interface SubmitResult {
  submitDraft: { id: number }
}

interface DiscardResult {
  discardDraft: boolean
}

export default function DraftDetail() {
  const { id } = useParams<{ id: string }>()
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  const draftId = id ? parseInt(id, 10) : NaN
  const [actionError, setActionError] = useState<string | null>(null)

  const { data, loading } = useQuery(GET_DRAFT, {
    variables: { workspaceId: currentWorkspace?.id, id: draftId },
    skip: !currentWorkspace || Number.isNaN(draftId),
    fetchPolicy: 'cache-and-network',
  })

  const { data: fieldConfData } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const [submitDraft, { loading: submitting }] = useMutation<SubmitResult>(SUBMIT_DRAFT, {
    refetchQueries: [{ query: GET_DRAFTS, variables: { workspaceId: currentWorkspace?.id } }],
    awaitRefetchQueries: true,
  })
  const [discardDraft, { loading: discarding }] = useMutation<DiscardResult>(DISCARD_DRAFT, {
    refetchQueries: [{ query: GET_DRAFTS, variables: { workspaceId: currentWorkspace?.id } }],
    awaitRefetchQueries: true,
  })

  const fieldDefs: FieldDef[] = useMemo(
    () => fieldConfData?.fieldConfiguration?.fields ?? [],
    [fieldConfData],
  )
  const fieldDefMap = useMemo(() => {
    const map = new Map<string, FieldDef>()
    for (const def of fieldDefs) map.set(def.id, def)
    return map
  }, [fieldDefs])

  const draft: DraftDetailModel | null = data?.case ?? null

  const handleSubmit = async () => {
    setActionError(null)
    if (!draft || !currentWorkspace) return
    if (!draft.title || draft.title.trim() === '') {
      setActionError(t('draftSubmitErrorMissingTitle'))
      return
    }
    try {
      const res = await submitDraft({
        variables: { workspaceId: currentWorkspace.id, id: draft.id },
      })
      const submittedId = res.data?.submitDraft.id ?? draft.id
      navigate(`/ws/${currentWorkspace.id}/cases/${submittedId}`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setActionError(t('draftSubmitErrorGeneric', { message: msg }))
    }
  }

  const handleDiscard = async () => {
    setActionError(null)
    if (!draft || !currentWorkspace) return
    if (!window.confirm(t('draftDiscardConfirm'))) return
    try {
      await discardDraft({
        variables: { workspaceId: currentWorkspace.id, id: draft.id },
      })
      navigate(`/ws/${currentWorkspace.id}/drafts`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setActionError(t('draftDiscardErrorGeneric', { message: msg }))
    }
  }

  if (loading && !draft) {
    return (
      <div className="h-main-inner">
        <div className="muted" style={{ padding: 24 }}>{t('noDataAvailable')}</div>
      </div>
    )
  }
  if (!draft) {
    return (
      <div className="h-main-inner">
        <div className="muted" style={{ padding: 24 }}>{t('draftsEmpty')}</div>
      </div>
    )
  }

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('draftDetailHeading', { id: draft.id })}</h1>
          <div className="sub">{draft.title || t('draftsUntitled')}</div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            type="button"
            className="btn"
            onClick={handleDiscard}
            disabled={discarding || submitting}
          >
            {t('draftDiscardButton')}
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={handleSubmit}
            disabled={discarding || submitting}
          >
            {t('draftSubmitButton')}
          </button>
        </div>
      </div>

      {actionError && (
        <div
          role="alert"
          className="card"
          style={{ padding: 12, marginBottom: 16, color: 'var(--color-error)' }}
        >
          {actionError}
        </div>
      )}

      <div className="card" style={{ padding: 16, marginBottom: 16 }}>
        <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          {t('draftDetailTitleLabel')}
        </div>
        <div style={{ marginTop: 4, fontSize: 16, fontWeight: 500 }}>
          {draft.title || <span className="soft">{t('draftsUntitled')}</span>}
        </div>

        <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 16 }}>
          {t('draftDetailDescriptionLabel')}
        </div>
        <div style={{ marginTop: 4, whiteSpace: 'pre-wrap' }}>
          {draft.description?.trim() ? (
            draft.description
          ) : (
            <span className="soft">{t('draftDetailNoDescription')}</span>
          )}
        </div>

        <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 16 }}>
          {t('draftDetailPrivateLabel')}
        </div>
        <div style={{ marginTop: 4 }}>
          {draft.isPrivate ? t('draftDetailPrivateOn') : t('draftDetailPrivateOff')}
        </div>
      </div>

      {draft.fields.length > 0 && (
        <div className="card" style={{ padding: 16 }}>
          <h2 style={{ marginTop: 0, fontSize: 14 }}>{t('draftDetailFieldsHeading')}</h2>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <tbody>
              {draft.fields.map((fv) => {
                const def = fieldDefMap.get(fv.fieldId)
                const label = def?.name ?? fv.fieldId
                return (
                  <tr key={fv.fieldId}>
                    <td
                      style={{
                        padding: '8px 0',
                        color: 'var(--text-muted)',
                        fontSize: 12,
                        width: 200,
                      }}
                    >
                      {label}
                    </td>
                    <td style={{ padding: '8px 0' }}>{renderValue(fv.value)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function renderValue(value: unknown): string {
  if (value == null) return '—'
  if (Array.isArray(value)) return value.map((v) => String(v)).join(', ')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
