import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import { useNavigate, useParams } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { EXECUTE_CASE_IMPORT, GET_IMPORT } from '../graphql/import'
import { GET_CASES } from '../graphql/case'
import Button from '../components/Button'
import { CaseRow, ItemBadge, IssueItem, SessionStatusBadge } from '../components/import'
import type { ImportSession } from '../components/import'

export default function ImportDetail() {
  const { currentWorkspace } = useWorkspace()
  const { importId } = useParams<{ importId: string }>()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [execError, setExecError] = useState<string | null>(null)

  const { data, loading, error } = useQuery<{ caseImport: ImportSession }>(GET_IMPORT, {
    variables: { workspaceId: currentWorkspace?.id ?? '', id: importId },
    skip: !currentWorkspace || !importId,
    fetchPolicy: 'cache-and-network',
  })
  const [executeImport, { loading: executing }] = useMutation(EXECUTE_CASE_IMPORT, {
    refetchQueries: [{ query: GET_CASES, variables: { workspaceId: currentWorkspace?.id ?? '' } }],
    awaitRefetchQueries: true,
  })

  if (loading && !data) {
    return (
      <div className="h-main-inner">
        <div className="soft" style={{ padding: 16 }}>{t('appName')} · loading…</div>
      </div>
    )
  }
  if (error || !data?.caseImport) {
    return (
      <div className="h-main-inner">
        <div style={{ padding: 16, color: 'var(--color-error)' }}>{t('importNotFound')}</div>
      </div>
    )
  }

  const session = data.caseImport
  const cases = session.snapshot.cases
  const sessionIssues = session.issues
  const caseIssues = cases.flatMap((c) =>
    c.issues.map((i) => ({ ...i, caseIdx: c.index })),
  )
  // Action issues no longer surface: Import does not create Actions
  // (DRAFT restriction), so per-Action issues are not produced.
  const allIssues = [...sessionIssues, ...caseIssues]
  const errorCount = allIssues.filter((i) => i.severity === 'ERROR').length
  const warnCount = allIssues.filter((i) => i.severity === 'WARNING').length

  const executeDisabled = !session.valid || executing
  const handleExecute = async () => {
    if (!currentWorkspace || !importId) return
    if (executeDisabled) return
    setExecError(null)
    try {
      await executeImport({ variables: { workspaceId: currentWorkspace.id, id: importId } })
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      setExecError(msg)
    }
  }

  return (
    <div className="h-main-inner" style={{ maxWidth: 920 }}>
      <div className="row" style={{ marginBottom: 6, alignItems: 'flex-start', gap: 12 }}>
        <div className="col" style={{ gap: 2, minWidth: 0 }}>
          <h1
            style={{
              margin: 0,
              fontSize: 18,
              fontWeight: 600,
              letterSpacing: -0.01,
              lineHeight: 1.25,
              wordBreak: 'break-all',
            }}
            className="mono"
          >
            {session.source.originalFileName || '(no filename)'}
          </h1>
          <div className="soft" style={{ fontSize: 11.5 }}>
            {(session.source.sizeBytes / 1024).toFixed(1)} KB
            {session.executedAt && ` · ${t('importExecutedAt', { when: new Date(session.executedAt).toLocaleString() })}`}
          </div>
        </div>
        <span style={{ flex: 1 }} />
        <SessionStatusBadge status={session.status} />
      </div>

      <div style={{ marginBottom: 24 }} />

      {/* Status-specific banner */}
      {session.status === 'APPLIED' && (
        <ResultBanner kind="ok">
          <div className="col" style={{ gap: 4 }}>
            <div style={{ fontSize: 15, fontWeight: 600 }}>
              {t('importAppliedHeadline', { n: session.createdCount })}
            </div>
            <ResultCountsLine
              created={session.createdCount}
              failed={session.failedCount}
              skipped={session.skippedCount}
            />
          </div>
          <Button
            variant="primary"
            onClick={() => navigate(`/ws/${currentWorkspace!.id}/cases`)}
          >
            {t('importBtnOpenCasesList')}
          </Button>
        </ResultBanner>
      )}

      {session.status === 'FAILED' && (
        <ResultBanner kind="err">
          <div className="col" style={{ gap: 6 }}>
            <div style={{ fontSize: 15, fontWeight: 600 }}>{t('importFailedHeadline')}</div>
            <ResultCountsLine
              created={session.createdCount}
              failed={session.failedCount}
              skipped={session.skippedCount}
            />
            <div className="soft" style={{ fontSize: 12, lineHeight: 1.55 }}>
              {t('importFailedHint')}
            </div>
          </div>
          <Button
            variant="secondary"
            onClick={() => navigate(`/ws/${currentWorkspace!.id}/cases`)}
          >
            {t('importBtnOpenCasesList')}
          </Button>
        </ResultBanner>
      )}

      {/* Summary stats (pending / applied / failed all show). The
          whole panel — heading included — is wrapped in one white
          card so it sits visibly above the muted page surface. */}
      <PanelCard style={{ marginBottom: 18 }}>
        <PanelHeading>{t('importSummaryHeader')}</PanelHeading>
        <div className="row" style={{ gap: 18 }}>
          <Stat n={cases.length} label={t('importStatCases')} />
          <Sep />
          <Stat
            n={errorCount}
            label={t('importStatErrors')}
            color={errorCount > 0 ? 'var(--color-error)' : undefined}
          />
          <Sep />
          <Stat
            n={warnCount}
            label={t('importStatWarnings')}
            color={warnCount > 0 ? 'var(--color-warning)' : undefined}
          />
        </div>
      </PanelCard>

      {allIssues.length > 0 && (
        <PanelCard style={{ marginBottom: 24 }}>
          <PanelHeading>
            {t('importSectionIssues')}{' '}
            <span className="soft mono" style={{ fontSize: 11 }}>{allIssues.length}</span>
          </PanelHeading>
          <div className="col" style={{ gap: 6 }}>
            {allIssues.map((i, idx) => (
              <IssueItem key={idx} issue={i} />
            ))}
          </div>
        </PanelCard>
      )}

      <PanelCard style={{ marginBottom: 24 }}>
        <PanelHeading>
          {session.status === 'APPLIED'
            ? t('importSectionCasesCreated')
            : session.status === 'FAILED'
              ? t('importSectionResults')
              : t('importSectionCasesToCreate')}
        </PanelHeading>
        <div className="col" style={{ gap: 8 }}>
          {cases.map((c) => (
            <CaseRow
              key={c.index}
              c={c}
              workspaceId={currentWorkspace!.id}
              defaultOpen={
                c.index === 0 ||
                c.issues.some((i) => i.severity === 'ERROR') ||
                c.result.status === 'FAILED'
              }
            />
          ))}
        </div>
      </PanelCard>

      {session.status === 'PENDING' && (
        <div
          style={{
            position: 'sticky',
            bottom: 0,
            marginTop: 28,
            padding: '14px 16px',
            background: 'var(--bg-paper)',
            borderTop: '1px solid var(--border-strong, var(--border-default))',
            borderRadius: '8px 8px 0 0',
            boxShadow: '0 -4px 16px rgba(0,0,0,0.04)',
            display: 'flex',
            alignItems: 'center',
            gap: 12,
          }}
        >
          <div className="col" style={{ gap: 2 }}>
            <div style={{ fontSize: 13, fontWeight: 500 }}>
              {errorCount > 0 ? (
                <span style={{ color: 'var(--color-error)' }}>
                  {t('importActionBarErrorMessage')}
                </span>
              ) : (
                t('importActionBarReadyMessage', { cases: cases.length })
              )}
            </div>
            <div className="soft" style={{ fontSize: 11.5 }}>
              {t('importActionBarHint')}
            </div>
          </div>
          <span style={{ flex: 1 }} />
          {execError && (
            <span style={{ color: 'var(--color-error)', fontSize: 12 }}>{execError}</span>
          )}
          <Button
            variant="primary"
            disabled={executeDisabled}
            aria-disabled={executeDisabled}
            onClick={() => void handleExecute()}
            style={{
              opacity: executeDisabled ? 0.5 : 1,
              cursor: executeDisabled ? 'not-allowed' : 'pointer',
              pointerEvents: executeDisabled ? 'none' : 'auto',
            }}
          >
            {executing ? t('importBtnExecuting') : t('importBtnExecute')}
          </Button>
        </div>
      )}
    </div>
  )
}

// PanelCard / PanelHeading — used by ImportDetail to wrap each section
// (Summary, Issues, Cases to create) in a white card that sits above
// the muted page background.

function PanelCard({
  children,
  style,
}: {
  children: React.ReactNode
  style?: React.CSSProperties
}) {
  return (
    <section
      style={{
        background: 'var(--bg-paper)',
        border: '1px solid var(--line-strong, var(--border-default))',
        borderRadius: 8,
        padding: 16,
        boxShadow: '0 1px 2px rgba(20,20,18,0.03)',
        ...style,
      }}
    >
      {children}
    </section>
  )
}

function PanelHeading({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="field-label"
      style={{
        marginBottom: 12,
        fontSize: 11,
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: 0.08,
        color: 'var(--text-muted)',
      }}
    >
      {children}
    </div>
  )
}

function Stat({ n, label, color }: { n: number; label: string; color?: string }) {
  return (
    <div className="col" style={{ gap: 2 }}>
      <div
        style={{
          fontSize: 22,
          fontWeight: 600,
          color: color || 'var(--text-heading, var(--text-body))',
          lineHeight: 1,
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        {n}
      </div>
      <div
        className="soft"
        style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 0.05, fontWeight: 500 }}
      >
        {label}
      </div>
    </div>
  )
}

function Sep() {
  return (
    <div style={{ width: 1, alignSelf: 'stretch', background: 'var(--border-default)' }} />
  )
}

function ResultBanner({
  kind,
  children,
}: {
  kind: 'ok' | 'err'
  children: React.ReactNode
}) {
  const color =
    kind === 'ok' ? 'var(--color-success, #15803d)' : 'var(--color-error, #b91c1c)'
  return (
    <div
      style={{
        padding: 18,
        background: `color-mix(in oklch, ${color} 6%, var(--bg-paper))`,
        border: `1px solid color-mix(in oklch, ${color} 28%, var(--border-default))`,
        borderRadius: 8,
        marginBottom: 24,
        display: 'grid',
        gridTemplateColumns: 'auto 1fr auto',
        alignItems: 'center',
        gap: 16,
      }}
    >
      <span
        style={{
          width: 36,
          height: 36,
          borderRadius: 99,
          background: color,
          color: 'white',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontWeight: 600,
        }}
      >
        {kind === 'ok' ? '✓' : '!'}
      </span>
      {children}
    </div>
  )
}

function ResultCountsLine({
  created,
  failed,
  skipped,
}: {
  created: number
  failed: number
  skipped: number
}) {
  return (
    <div
      className="row"
      style={{
        gap: 14,
        fontSize: 13,
        fontVariantNumeric: 'tabular-nums',
        alignItems: 'center',
      }}
    >
      <span className="row" style={{ gap: 5 }}>
        <ItemBadge status="CREATED" small />
        <b>{created}</b>
      </span>
      <span className="row" style={{ gap: 5 }}>
        <ItemBadge status="FAILED" small />
        <b>{failed}</b>
      </span>
      <span className="row" style={{ gap: 5 }}>
        <ItemBadge status="SKIPPED" small />
        <b>{skipped}</b>
      </span>
    </div>
  )
}
