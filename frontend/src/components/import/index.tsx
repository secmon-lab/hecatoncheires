import { Fragment, useState, type ReactNode } from 'react'
import { useTranslation } from '../../i18n'

// ---- Shared GraphQL-side types ---------------------------------------------

export type ImportItemStatus = 'PENDING' | 'CREATED' | 'FAILED' | 'SKIPPED'
export type ImportSessionStatus = 'PENDING' | 'APPLIED' | 'FAILED'
export type ImportIssueSeverity = 'ERROR' | 'WARNING'

export interface ImportIssue {
  path: string
  message: string
  severity: ImportIssueSeverity
  caseIdx?: number
}

export interface ImportSnapshotField {
  key: string
  display: string
}

export interface ImportSlackUser {
  id: string
  name?: string | null
  realName?: string | null
  imageUrl?: string | null
}

export interface ImportActionResult {
  status: ImportItemStatus
  createdActionID?: number | null
  error?: ImportIssue | null
}

export interface ImportCaseResult {
  status: ImportItemStatus
  createdCaseID?: number | null
  error?: ImportIssue | null
}

export interface ImportSnapshotAction {
  index: number
  title: string
  description?: string | null
  assigneeID?: string | null
  dueDate?: string | null
  issues: ImportIssue[]
  result: ImportActionResult
}

export interface ImportSnapshotCase {
  index: number
  title: string
  description?: string | null
  isPrivate: boolean
  assigneeIDs: string[]
  /** Slack users resolved on the server. IDs not present in the
   * workspace's SlackUser registry are omitted here (the raw `assigneeIDs`
   * still carries them so the UI can fall back to the literal ID). */
  assignees: ImportSlackUser[]
  fields: ImportSnapshotField[]
  actions: ImportSnapshotAction[]
  issues: ImportIssue[]
  result: ImportCaseResult
}

export interface ImportSnapshot {
  version: number
  cases: ImportSnapshotCase[]
}

export interface ImportSource {
  originalFileName: string
  sizeBytes: number
}

export interface ImportSession {
  id: string
  workspaceID: string
  creatorUserID: string
  status: ImportSessionStatus
  source: ImportSource
  snapshot: ImportSnapshot
  issues: ImportIssue[]
  valid: boolean
  fieldSchemaHash: string
  createdAt: string
  updatedAt: string
  executedAt?: string | null
  createdCount: number
  failedCount: number
  skippedCount: number
}

// ---- ItemBadge -------------------------------------------------------------

const ITEM_COLOR: Record<ImportItemStatus, string> = {
  CREATED: 'var(--color-success, #15803d)',
  FAILED: 'var(--color-error, #b91c1c)',
  SKIPPED: 'var(--text-muted, var(--text-body))',
  PENDING: 'var(--text-muted, var(--text-body))',
}

export function ItemBadge({
  status,
  small = false,
}: {
  status: ImportItemStatus
  small?: boolean
}) {
  const { t } = useTranslation()
  const label =
    status === 'CREATED'
      ? t('importBadgeCreated')
      : status === 'FAILED'
        ? t('importBadgeFailed')
        : status === 'SKIPPED'
          ? t('importBadgeSkipped')
          : t('importBadgePending')
  const color = ITEM_COLOR[status]
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: small ? '1px 6px' : '2px 8px',
        borderRadius: 4,
        fontSize: small ? 10.5 : 11.5,
        fontWeight: 500,
        lineHeight: 1.6,
        color,
        background: `color-mix(in oklch, ${color} 12%, transparent)`,
        border: `1px solid color-mix(in oklch, ${color} 26%, transparent)`,
        fontVariantNumeric: 'tabular-nums',
      }}
    >
      {label}
    </span>
  )
}

// ---- SessionStatusBadge ----------------------------------------------------

const SESSION_COLOR: Record<ImportSessionStatus, string> = {
  PENDING: 'var(--color-info, #2563eb)',
  APPLIED: 'var(--color-success, #15803d)',
  FAILED: 'var(--color-error, #b91c1c)',
}

export function SessionStatusBadge({ status }: { status: ImportSessionStatus }) {
  const { t } = useTranslation()
  const label =
    status === 'PENDING'
      ? t('importStatusBadgePending')
      : status === 'APPLIED'
        ? t('importStatusBadgeApplied')
        : t('importStatusBadgeFailed')
  const color = SESSION_COLOR[status]
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        padding: '2px 9px',
        borderRadius: 4,
        fontSize: 11.5,
        fontWeight: 600,
        letterSpacing: 0.04,
        textTransform: 'uppercase',
        color,
        background: `color-mix(in oklch, ${color} 12%, transparent)`,
        border: `1px solid color-mix(in oklch, ${color} 28%, transparent)`,
      }}
    >
      <span
        style={{ width: 6, height: 6, borderRadius: 99, background: color }}
        aria-hidden
      />
      {label}
    </span>
  )
}

// ---- IssueItem -------------------------------------------------------------

export function IssueItem({ issue }: { issue: ImportIssue }) {
  const isError = issue.severity === 'ERROR'
  const color = isError ? 'var(--color-error, #b91c1c)' : 'var(--color-warning, #c2410c)'
  return (
    <div
      className="row"
      style={{
        gap: 10,
        padding: '8px 12px',
        alignItems: 'flex-start',
        background: `color-mix(in oklch, ${color} 5%, var(--bg-paper))`,
        borderLeft: `2px solid ${color}`,
        borderRadius: '0 4px 4px 0',
        fontSize: 12.5,
        lineHeight: 1.5,
      }}
    >
      <span style={{ color, marginTop: 1, flexShrink: 0, fontWeight: 600 }}>
        {isError ? '✗' : '!'}
      </span>
      {issue.path && (
        <code
          className="mono"
          style={{
            fontSize: 11.5,
            color: 'var(--text-muted, var(--text-body))',
            background: 'var(--bg-subtle)',
            padding: '1px 5px',
            borderRadius: 3,
            flexShrink: 0,
            marginTop: 1,
          }}
        >
          {issue.path}
        </code>
      )}
      <span
        style={{
          color: isError ? 'var(--text-heading, var(--text-body))' : 'var(--text-muted)',
        }}
      >
        {issue.message}
      </span>
    </div>
  )
}

// ---- FieldCell -----------------------------------------------------------
// A small "label + chipped value" cell used inside CaseRow's expanded
// body. The value is rendered as a chip (small colored pill) so it is
// visually distinct from the field label and from surrounding prose —
// this is the "make values look like values" requirement.

function ValueChip({ children }: { children: React.ReactNode }) {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        padding: '2px 8px',
        borderRadius: 4,
        fontSize: 12,
        background: 'var(--bg-subtle, var(--bg-sunken))',
        border: '1px solid var(--line-strong, var(--border-default))',
        color: 'var(--text-heading, var(--text-body))',
        fontVariantNumeric: 'tabular-nums',
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  )
}

function FieldCell({
  label,
  value,
  multi,
}: {
  label: string
  value?: string
  multi?: string[]
}) {
  // For MULTI_SELECT / MULTI_USER the backend may have already joined
  // the values with ", "; if a caller passes the raw display string
  // we still want each value to be an individual chip. Split on ", "
  // as the canonical separator the converter uses.
  const chips =
    multi !== undefined
      ? multi
      : value && value.includes(', ')
        ? value.split(', ')
        : value
          ? [value]
          : []
  return (
    <div className="col" style={{ gap: 4, minWidth: 0 }}>
      <div
        style={{
          fontSize: 10,
          color: 'var(--text-muted, var(--text-body))',
          textTransform: 'uppercase',
          letterSpacing: 0.08,
          fontWeight: 600,
        }}
      >
        {label}
      </div>
      <div className="row" style={{ gap: 5, flexWrap: 'wrap' }}>
        {chips.length === 0 ? (
          <span className="soft" style={{ fontSize: 12 }}>—</span>
        ) : (
          chips.map((c, i) => <ValueChip key={i}>{c}</ValueChip>)
        )}
      </div>
    </div>
  )
}

// ---- CaseChips ------------------------------------------------------------
// Tiny helper that renders 0..N inline chips ("private", "has errors",
// etc.) separated by bullets. Putting it behind a component prevents
// stray leading "·" separators when only some chips are present.

function CaseChips({
  isPrivate,
  hasErr,
  hasWarn,
  actionFailedCount,
  privateLabel,
  hasErrLabel,
  hasWarnLabel,
  actionFailedLabel,
}: {
  isPrivate: boolean
  hasErr: boolean
  hasWarn: boolean
  actionFailedCount: number
  privateLabel: string
  hasErrLabel: string
  hasWarnLabel: string
  actionFailedLabel: string
}) {
  const chips: ReactNode[] = []
  if (isPrivate) {
    chips.push(
      <span key="p" style={{ color: 'var(--color-warning, #c2410c)' }}>
        {privateLabel}
      </span>,
    )
  }
  if (hasErr) {
    chips.push(
      <span key="e" style={{ color: 'var(--color-error, #b91c1c)' }}>
        {hasErrLabel}
      </span>,
    )
  } else if (hasWarn) {
    chips.push(
      <span key="w" style={{ color: 'var(--color-warning, #c2410c)' }}>
        {hasWarnLabel}
      </span>,
    )
  }
  if (actionFailedCount > 0) {
    chips.push(
      <span key="af" style={{ color: 'var(--color-error, #b91c1c)' }}>
        {actionFailedCount} {actionFailedLabel} failed
      </span>,
    )
  }
  if (chips.length === 0) return null
  return (
    <span
      className="soft"
      style={{ fontSize: 11.5, display: 'flex', gap: 8, alignItems: 'center' }}
    >
      {chips.map((node, i) => (
        <Fragment key={i}>
          {i > 0 && <span>·</span>}
          {node}
        </Fragment>
      ))}
    </span>
  )
}

// ---- CaseRow ---------------------------------------------------------------

export function CaseRow({
  c,
  workspaceId,
  defaultOpen = false,
}: {
  c: ImportSnapshotCase
  workspaceId: string
  defaultOpen?: boolean
}) {
  const { t } = useTranslation()
  // Auto-expand when there's a failure to surface so the user does not
  // have to click each row to discover what went wrong.
  const actionFailed = c.actions.some((a) => a.result.status === 'FAILED')
  const initiallyOpen = defaultOpen || c.result.status === 'FAILED' || actionFailed
  const [open, setOpen] = useState(initiallyOpen)
  const status = c.result.status
  const hasErr = c.issues.some((i) => i.severity === 'ERROR')
  const hasWarn = c.issues.some((i) => i.severity === 'WARNING')
  const actionFailedCount = c.actions.filter((a) => a.result.status === 'FAILED').length

  // Left border colour — encodes the per-Case status at a glance. For
  // PENDING rows without errors we still want a visible "neutral" tone,
  // so we fall back to --line-strong (the strongest line token in the
  // palette) instead of the near-invisible --border-default.
  const borderL =
    status === 'FAILED'
      ? 'var(--color-error, #b91c1c)'
      : status === 'CREATED'
        ? 'var(--color-success, #15803d)'
        : status === 'SKIPPED'
          ? 'var(--line-strong)'
          : hasErr
            ? 'var(--color-error, #b91c1c)'
            : 'var(--line-strong)'

  const bg =
    status === 'SKIPPED'
      ? 'color-mix(in oklch, var(--text-muted) 4%, var(--bg-paper))'
      : status === 'PENDING' && hasErr
        ? 'color-mix(in oklch, var(--color-error, #b91c1c) 4%, var(--bg-paper))'
        : 'var(--bg-paper)'

  return (
    <div
      style={{
        // --line-strong gives the rounded card a visible outline; the
        // earlier --border-default (8% alpha) was too faint to read.
        border: '1px solid var(--line-strong, var(--border-default))',
        borderLeft: `3px solid ${borderL}`,
        borderRadius: 6,
        background: bg,
        overflow: 'hidden',
        opacity: status === 'SKIPPED' ? 0.72 : 1,
        boxShadow: '0 1px 2px rgba(20,20,18,0.03)',
      }}
    >
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          width: '100%',
          background: 'transparent',
          border: 'none',
          textAlign: 'left',
          padding: '10px 14px',
          cursor: 'pointer',
          color: 'inherit',
          fontFamily: 'inherit',
          display: 'grid',
          gridTemplateColumns: 'auto auto 1fr auto auto',
          alignItems: 'center',
          gap: 12,
        }}
      >
        <span
          style={{
            display: 'inline-block',
            width: 10,
            color: 'var(--text-muted, var(--text-body))',
            transform: open ? 'rotate(90deg)' : 'none',
            transition: 'transform .12s',
          }}
          aria-hidden
        >
          ▸
        </span>
        <span
          className="mono soft"
          style={{ fontSize: 12, minWidth: 24, textAlign: 'right' }}
        >
          [{c.index + 1}]
        </span>
        <span className="col" style={{ gap: 3, minWidth: 0 }}>
          <span
            style={{
              fontSize: 13.5,
              fontWeight: 500,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {c.title || <em className="soft">(no title)</em>}
          </span>
          <CaseChips
            isPrivate={c.isPrivate}
            hasErr={hasErr}
            hasWarn={hasWarn}
            actionFailedCount={actionFailedCount}
            privateLabel="private"
            hasErrLabel={t('importCaseHasErrors')}
            hasWarnLabel={t('importCaseHasWarnings')}
            actionFailedLabel={t('importActionsLabel').toLowerCase()}
          />
        </span>

        {status === 'CREATED' && c.result.createdCaseID && (
          <a
            href={`/ws/${workspaceId}/cases/${c.result.createdCaseID}`}
            onClick={(e) => e.stopPropagation()}
            className="row"
            style={{
              gap: 4,
              fontSize: 12,
              color: 'var(--color-primary, var(--accent))',
              textDecoration: 'none',
              fontFamily: 'inherit',
              fontWeight: 500,
            }}
          >
            <span>Case #{c.result.createdCaseID}</span>
            <span aria-hidden>→</span>
          </a>
        )}
        {status === 'FAILED' && c.result.error && (
          <span
            style={{
              fontSize: 11,
              color: 'var(--color-error, #b91c1c)',
              fontFamily: 'inherit',
              maxWidth: 280,
              textAlign: 'right',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {c.result.error.message}
          </span>
        )}
        {status === 'SKIPPED' && (
          <span className="soft" style={{ fontSize: 11, fontStyle: 'italic' }}>
            {t('importCaseSkippedHint')}
          </span>
        )}
        {(status === 'PENDING' || (status === 'CREATED' && !c.result.createdCaseID)) && (
          <span />
        )}
        <ItemBadge status={status} small />
      </button>

      {open && (
        <div
          style={{
            padding: '0 14px 14px',
            borderTop: '1px dashed var(--border-default)',
          }}
        >
          {c.description && (
            <div
              style={{
                padding: '12px 0 0',
                fontSize: 12.5,
                lineHeight: 1.6,
                color: 'var(--text-muted, var(--text-body))',
                whiteSpace: 'pre-wrap',
              }}
            >
              {c.description}
            </div>
          )}
          {(c.fields.length > 0 || c.assigneeIDs.length > 0) && (
            <div
              style={{
                paddingTop: 12,
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))',
                gap: 12,
              }}
            >
              {c.fields.map((f) => (
                <FieldCell key={f.key} label={f.key.toUpperCase()} value={f.display} />
              ))}
              {c.assigneeIDs.length > 0 && (
                <FieldCell
                  label={t('importAssigneesLabel').toUpperCase()}
                  multi={c.assigneeIDs.map((id) => {
                    const u = c.assignees.find((u) => u.id === id)
                    return u?.realName || u?.name || id
                  })}
                />
              )}
            </div>
          )}
          {c.issues.length > 0 && (
            <div className="col" style={{ gap: 6, paddingTop: 12 }}>
              {c.issues.map((i, idx) => (
                <IssueItem key={idx} issue={i} />
              ))}
            </div>
          )}
          {/* Import does not create Actions (DRAFT restriction). The
              Actions list is therefore never rendered inside the
              expanded preview — the YAML's `actions:` blocks are
              already surfaced as a per-Case WARNING above. */}
        </div>
      )}
    </div>
  )
}
