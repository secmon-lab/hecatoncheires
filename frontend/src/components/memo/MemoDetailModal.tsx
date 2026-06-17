import { useQuery } from '@apollo/client'
import Modal from '../Modal'
import Button from '../Button'
import FieldDisplay from '../fields/FieldDisplay'
import { IconEdit, IconWarn } from '../Icons'
import { useTranslation } from '../../i18n'
import { GET_MEMO } from '../../graphql/memo'

interface FieldOption {
  id: string
  name: string
  description?: string | null
  metadata?: Record<string, unknown> | null
}

interface FieldDef {
  id: string
  name: string
  type: string
  required: boolean
  description?: string | null
  options?: FieldOption[] | null
}

interface Props {
  workspaceId: string
  caseId: number
  memoId: string
  memoFields: FieldDef[]
  onClose: () => void
  onEdit: () => void
  onArchive: () => void
  onUnarchive: () => void
}

function formatDate(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

export default function MemoDetailModal({
  workspaceId,
  caseId,
  memoId,
  memoFields,
  onClose,
  onEdit,
  onArchive,
  onUnarchive,
}: Props) {
  const { t } = useTranslation()

  const { data, loading } = useQuery(GET_MEMO, {
    variables: { workspaceId, caseID: caseId, id: memoId },
    fetchPolicy: 'cache-and-network',
  })

  const memo = data?.memo

  const isArchived = !!memo?.archivedAt

  const footer = isArchived ? (
    <>
      <span style={{ fontSize: 12, color: 'var(--fg-muted)', flex: 1 }}>
        {t('memoDetailRestoreHint')}
      </span>
      <Button variant="ghost" onClick={onClose}>
        {t('btnClose')}
      </Button>
      <Button variant="secondary" onClick={onUnarchive}>
        {t('btnUnarchive')}
      </Button>
    </>
  ) : (
    <>
      <Button variant="danger" size="sm" onClick={onArchive} style={{ marginRight: 'auto' }}>
        {t('btnArchive')}
      </Button>
      <Button variant="ghost" onClick={onClose}>
        {t('btnClose')}
      </Button>
      <Button variant="primary" icon={<IconEdit size={13} />} onClick={onEdit}>
        {t('btnEdit')}
      </Button>
    </>
  )

  return (
    <Modal
      open
      onClose={onClose}
      width={620}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-2)', flex: 1, minWidth: 0 }}>
          <span
            style={{
              fontSize: 15,
              fontWeight: 600,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {loading ? '…' : (memo?.title ?? '—')}
          </span>
          {isArchived && (
            <span
              style={{
                fontSize: 11,
                padding: '1px 8px',
                borderRadius: 999,
                border: '1px solid var(--border-default)',
                color: 'var(--fg-muted)',
                background: 'var(--bg-subtle)',
                flexShrink: 0,
              }}
            >
              {t('memoArchivedBadge')}
            </span>
          )}
        </div>
      }
      footer={footer}
    >
      {loading && (
        <div style={{ padding: 'var(--sp-6)', textAlign: 'center', color: 'var(--fg-muted)', fontSize: 13 }}>
          {t('loading')}
        </div>
      )}

      {!loading && memo && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-4)' }}>
          {/* Archived notice */}
          {isArchived && (
            <div
              style={{
                display: 'flex',
                gap: 'var(--sp-2)',
                alignItems: 'flex-start',
                background: 'var(--bg-sunken)',
                border: '1px solid var(--border-light)',
                borderRadius: '0.375rem',
                padding: 'var(--sp-3) var(--sp-4)',
                fontSize: 12,
                color: 'var(--fg-muted)',
                lineHeight: 1.6,
              }}
            >
              <IconWarn size={14} style={{ color: 'var(--warn)', flexShrink: 0, marginTop: 1 }} />
              <span>{t('memoDetailArchivedNotice')}</span>
            </div>
          )}

          {/* Meta row */}
          <div
            style={{
              display: 'flex',
              gap: 'var(--sp-6)',
              fontSize: 12,
              color: 'var(--fg-muted)',
              flexWrap: 'wrap',
            }}
          >
            <span>
              <span style={{ marginRight: 4 }}>{t('memoIdLabel')}:</span>
              <span className="mono" style={{ fontSize: 11 }}>{memo.id.slice(0, 8)}</span>
            </span>
            <span>
              <span style={{ marginRight: 4 }}>{t('memoCreatedLabel')}:</span>
              <span className="mono" style={{ fontSize: 11 }}>{formatDate(memo.createdAt)}</span>
            </span>
            <span>
              <span style={{ marginRight: 4 }}>{t('memoUpdatedLabel')}:</span>
              <span className="mono" style={{ fontSize: 11 }}>{formatDate(memo.updatedAt)}</span>
            </span>
          </div>

          {/* Fields */}
          {memoFields.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-4)' }}>
              {memoFields.map((f) => {
                const fv = memo.fields?.find((x: { fieldId: string; value: unknown }) => x.fieldId === f.id)
                return (
                  <div key={f.id} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-1)' }}>
                    <span
                      style={{
                        fontSize: 11,
                        fontWeight: 600,
                        textTransform: 'uppercase',
                        letterSpacing: '0.04em',
                        color: 'var(--fg-muted)',
                      }}
                    >
                      {f.name}
                    </span>
                    <FieldDisplay
                      field={{ ...f, options: f.options ?? undefined }}
                      value={fv?.value}
                    />
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}
    </Modal>
  )
}
