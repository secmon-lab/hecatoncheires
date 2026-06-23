import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client'

import { GET_JOB_RUN_EVENTS, GET_JOB_RUN_LOG } from '../graphql/caseAgent'
import { useTranslation } from '../i18n'
import {
  IconChevLeft,
  IconChevRight,
  IconDownload,
} from '../components/Icons'
import StageBadge, { type JobRunStage } from '../components/caseAgent/StageBadge'
import styles from './JobRunLogDetail.module.css'

type JobRunEventKind = 'LLM_REQUEST' | 'LLM_RESPONSE' | 'TOOL_CALL' | 'RUN_ERROR'

interface JobRunEvent {
  eventId: string
  runId: string
  sequence: number
  occurredAt: string
  kind: JobRunEventKind
  parentSequence: number
  phase: string
  agentLabel: string
  payload: string
}

interface JobRunLogDetailData {
  workspaceId: string
  caseId: number
  jobId: string
  jobName: string
  runId: string
  traceId: string
  stage: JobRunStage
  startedAt: string
  endedAt: string | null
  durationMs: number | null
  errorMessage: string
  systemPrompt: string
  eventType: string
  eventTriggerAt: string
}

// ExportedEvent mirrors JobRunEvent but with payload decoded from its
// transport string into a nested object when it parses as JSON. A payload
// that fails to parse (truncated / non-JSON on the server side) is kept
// verbatim so the exported record never loses data.
interface ExportedEvent extends Omit<JobRunEvent, 'payload'> {
  payload: unknown
}

export interface RunExport {
  exportedAt: string
  runLog: JobRunLogDetailData
  events: ExportedEvent[]
}

// buildRunExport assembles the complete, downloadable execution record from
// the run metadata and its events. Events are emitted in sequence order and
// each payload is decoded to nested JSON where possible. Kept as a pure
// function (no DOM access) so it can be unit-tested in isolation. The
// caller supplies exportedAt so the function stays deterministic/testable.
export function buildRunExport(
  log: JobRunLogDetailData,
  events: JobRunEvent[],
  exportedAt: string,
): RunExport {
  const ordered = [...events].sort((a, b) => a.sequence - b.sequence)
  return {
    exportedAt,
    runLog: log,
    events: ordered.map((ev) => {
      const { payload, ...rest } = ev
      let decoded: unknown = payload
      try {
        decoded = JSON.parse(payload)
      } catch {
        // Non-JSON payload: keep the original string rather than dropping it.
        decoded = payload
      }
      return { ...rest, payload: decoded }
    }),
  }
}

const EVENT_COLOR: Record<JobRunEventKind, string> = {
  LLM_REQUEST: 'var(--info)',
  LLM_RESPONSE: 'var(--accent)',
  TOOL_CALL: 'var(--warn)',
  RUN_ERROR: 'var(--danger)',
}

function eventLabelKey(kind: JobRunEventKind): string {
  switch (kind) {
    case 'LLM_REQUEST':
      return 'jobRunLogEventLlmRequest'
    case 'LLM_RESPONSE':
      return 'jobRunLogEventLlmResponse'
    case 'TOOL_CALL':
      return 'jobRunLogEventToolCall'
    case 'RUN_ERROR':
      return 'jobRunLogEventRunError'
  }
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  // HH:MM:SS.mmm to match the design's per-event timestamp granularity.
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${hh}:${mm}:${ss}.${ms}`
}

function formatDuration(ms: number | null, stage: JobRunStage, runningLabel: string): string {
  if (stage === 'RUNNING' || ms == null) return runningLabel
  if (ms < 1000) return `${ms}ms`
  const secs = Math.round(ms / 100) / 10
  if (secs < 60) return `${secs}s`
  const m = Math.floor(secs / 60)
  const rem = Math.round(secs - m * 60)
  return `${m}m ${rem}s`
}

function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso ?? '—'
  return d.toLocaleString()
}

// summariseEvent returns the short header that sits next to the type
// pill. The full request / response / argument body is rendered by
// eventContent() in its own wrapping block below — we never truncate
// payload text here.
function summariseEvent(ev: JobRunEvent): string {
  try {
    const payload = JSON.parse(ev.payload) as Record<string, unknown>
    switch (ev.kind) {
      case 'LLM_REQUEST': {
        const messages = (payload.Messages as unknown[]) ?? []
        return `${messages.length} message${messages.length === 1 ? '' : 's'}`
      }
      case 'LLM_RESPONSE': {
        const inT = (payload.InputTokens as number) ?? 0
        const outT = (payload.OutputTokens as number) ?? 0
        return `tokens in=${inT} out=${outT}`
      }
      case 'TOOL_CALL': {
        const name = (payload.ToolName as string) ?? '(unknown)'
        return name
      }
      case 'RUN_ERROR': {
        const stage = (payload.Stage as string) ?? ''
        return stage ? `[${stage}]` : ev.kind
      }
    }
  } catch {
    // Fall back to a generic label so the row still renders.
  }
  return ev.kind
}

// eventContent returns the long-form body text shown in a wrapping
// block under the summary line. For LLM events we extract the actual
// message / response text so operators see what the agent said; for
// tool calls we expose the full arguments and (when present) result.
// Returning null skips the body block entirely.
function eventContent(ev: JobRunEvent): { text: string; muted?: boolean } | null {
  try {
    const payload = JSON.parse(ev.payload) as Record<string, unknown>
    switch (ev.kind) {
      case 'LLM_REQUEST': {
        const messages = (payload.Messages as Array<Record<string, unknown>>) ?? []
        if (messages.length === 0) return { text: '(no messages)', muted: true }
        const rendered = messages
          .map((m) => {
            const role = (m.Role as string) ?? '?'
            const contents = (m.Contents as Array<Record<string, unknown>>) ?? []
            const text = contents
              .map((c) => {
                const ct = c.Type as string | undefined
                if (ct === 'text' || ct === 'reasoning') return (c.Text as string) ?? ''
                if (ct === 'tool_call') {
                  const n = (c.Name as string) ?? ''
                  const a = (c.ArgumentsJSON as string) ?? ''
                  return `[tool_call ${n}] ${a}`
                }
                if (ct === 'tool_response') {
                  const id = (c.ToolCallID as string) ?? ''
                  const r = (c.ResultJSON as string) ?? ''
                  return `[tool_response ${id}] ${r}`
                }
                if (ct === 'image' || ct === 'pdf') {
                  return `[${ct} ${(c.URL as string) ?? ''}]`
                }
                return ''
              })
              .filter(Boolean)
              .join('\n')
            return `# ${role}\n${text}`.trim()
          })
          .join('\n\n')
        return { text: rendered }
      }
      case 'LLM_RESPONSE': {
        const texts = (payload.Texts as string[]) ?? []
        const calls = (payload.FunctionCalls as Array<Record<string, unknown>>) ?? []
        const parts: string[] = []
        if (texts.length > 0) parts.push(texts.join('\n'))
        if (calls.length > 0) {
          const callList = calls
            .map((c) => `[function_call ${(c.Name as string) ?? ''}] ${(c.ArgumentsJSON as string) ?? ''}`)
            .join('\n')
          parts.push(callList)
        }
        if (parts.length === 0) return { text: '(no text content)', muted: true }
        return { text: parts.join('\n\n') }
      }
      case 'TOOL_CALL': {
        const args = (payload.ArgumentsJSON as string) ?? ''
        const result = (payload.ResultJSON as string) ?? ''
        const isError = Boolean(payload.IsError)
        const errMsg = (payload.ErrorMessage as string) ?? ''
        const parts: string[] = []
        if (args) parts.push(`args: ${formatJsonInline(args)}`)
        if (isError) parts.push(`error: ${errMsg}`)
        else if (result) parts.push(`result: ${formatJsonInline(result)}`)
        if (parts.length === 0) return null
        return { text: parts.join('\n') }
      }
      case 'RUN_ERROR': {
        const msg = (payload.Message as string) ?? ''
        if (!msg) return null
        return { text: msg }
      }
    }
  } catch {
    return null
  }
  return null
}

// formatJsonInline pretty-prints when the value parses as JSON, otherwise
// passes the raw string through (it might already be a plain text /
// truncated chunk on the server side).
function formatJsonInline(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function eventMeta(ev: JobRunEvent): { key: string; value: string }[] {
  try {
    const payload = JSON.parse(ev.payload) as Record<string, unknown>
    switch (ev.kind) {
      case 'LLM_REQUEST': {
        return [
          { key: 'model', value: (payload.Model as string) ?? '—' },
        ]
      }
      case 'LLM_RESPONSE': {
        const meta: { key: string; value: string }[] = []
        if (payload.Model) meta.push({ key: 'model', value: payload.Model as string })
        if (payload.DurationMs != null)
          meta.push({ key: 'duration', value: `${payload.DurationMs as number}ms` })
        return meta
      }
      case 'TOOL_CALL': {
        const meta: { key: string; value: string }[] = []
        if (payload.StartedAt && payload.EndedAt) {
          const s = new Date(payload.StartedAt as string).getTime()
          const e = new Date(payload.EndedAt as string).getTime()
          if (!Number.isNaN(s) && !Number.isNaN(e)) {
            meta.push({ key: 'duration', value: `${e - s}ms` })
          }
        }
        if (payload.IsError) {
          meta.push({ key: 'error', value: (payload.ErrorMessage as string) ?? 'true' })
        }
        return meta
      }
      case 'RUN_ERROR':
        return []
    }
  } catch {
    return []
  }
  return []
}

export default function JobRunLogDetail() {
  const { workspaceId, id, runId } = useParams<{
    workspaceId: string
    id: string
    runId: string
  }>()
  const caseId = id ? parseInt(id, 10) : 0
  const { t } = useTranslation()

  const { data, loading, error } = useQuery<{ jobRunLog: JobRunLogDetailData | null }>(
    GET_JOB_RUN_LOG,
    {
      variables: { workspaceId, caseId, runId },
      skip: !workspaceId || !caseId || !runId,
      fetchPolicy: 'cache-and-network',
    },
  )
  const { data: eventsData } = useQuery<{ jobRunEvents: JobRunEvent[] }>(GET_JOB_RUN_EVENTS, {
    variables: { workspaceId, caseId, runId },
    skip: !workspaceId || !caseId || !runId,
    fetchPolicy: 'cache-and-network',
  })

  const [copied, setCopied] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  if (!workspaceId || !caseId || !runId) return null
  if (loading && !data?.jobRunLog) {
    return (
      <div className={styles.shell}>
        <div className={['card', styles.statusCard].join(' ')}>…</div>
      </div>
    )
  }
  if (error) {
    return (
      <div className={styles.shell}>
        <div className={['card', styles.statusCard].join(' ')}>{t('jobRunLogLoadError')}</div>
      </div>
    )
  }
  const log = data?.jobRunLog
  if (!log) {
    return (
      <div className={styles.shell}>
        <div className={['card', styles.statusCard].join(' ')}>{t('jobRunLogNotFound')}</div>
      </div>
    )
  }

  const events = eventsData?.jobRunEvents ?? []
  const promptLines = log.systemPrompt ? log.systemPrompt.split('\n').length : 0

  const copy = (label: string, text: string) => {
    void navigator.clipboard.writeText(text)
    setCopied(label)
    window.setTimeout(() => setCopied(null), 1500)
  }

  const downloadJson = () => {
    const record = buildRunExport(log, events, new Date().toISOString())
    const blob = new Blob([JSON.stringify(record, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `jobrun-${caseId}-${log.runId}.json`
    document.body.appendChild(a)
    try {
      a.click()
    } finally {
      document.body.removeChild(a)
      // Defer revoke: Firefox/Safari resolve the blob URL asynchronously
      // after click(), so a synchronous revoke can abort the download.
      window.setTimeout(() => URL.revokeObjectURL(url), 100)
    }
  }

  return (
    <div className={styles.shell}>
      {/* Breadcrumb */}
      <div className={styles.crumb}>
        <Link
          className={styles.crumbLink}
          to={`/ws/${workspaceId}/cases/${caseId}/agent`}
        >
          <IconChevLeft size={12} />
          {t('jobRunLogBack')}
        </Link>
        <span className={styles.crumbSep}>·</span>
        <span className={['truncate', styles.crumbTitle].join(' ')}>
          #{caseId}
        </span>
      </div>

      {/* Header */}
      <div className={styles.header}>
        <div className={styles.headerLead}>
          <div className={styles.headerCaption}>
            <span className={styles.captionLabel}>{t('jobRunLogTitleLabel')}</span>
            <span className={styles.captionRunId}>{log.runId}</span>
            <StageBadge stage={log.stage} />
          </div>
          <h1 className={styles.title}>{log.jobName}</h1>
        </div>
        <button
          type="button"
          data-testid="job-run-download-json"
          className={styles.downloadBtn}
          onClick={downloadJson}
          title={t('jobRunLogDownloadJsonTitle')}
        >
          <IconDownload size={14} />
          {t('jobRunLogDownloadJson')}
        </button>
      </div>

      {log.stage === 'FAILED' && log.errorMessage && (
        <div className={styles.errorBox}>⚠ {log.errorMessage}</div>
      )}

      {/* Meta card */}
      <div className={['card', styles.metaCard].join(' ')}>
        <MetaKpi label={t('jobRunLogMetaStarted')} value={formatDateTime(log.startedAt)} mono />
        <div className={styles.metaDivider} />
        <MetaKpi label={t('jobRunLogMetaEnded')} value={formatDateTime(log.endedAt)} mono />
        <div className={styles.metaDivider} />
        <MetaKpi
          label={t('jobRunLogMetaDuration')}
          value={formatDuration(log.durationMs, log.stage, t('caseAgentRunDurationRunning'))}
          mono
        />
        <div className={styles.metaDivider} />
        <MetaKpi label={t('jobRunLogMetaJobId')} value={log.jobId} mono />
        <div className={styles.metaDivider} />
        <MetaKpi label={t('jobRunLogMetaTrigger')} value={log.eventType || '—'} />
      </div>

      {/* ID row card */}
      <div className={['card', styles.idCard].join(' ')}>
        <IdField
          label={t('jobRunLogIdRunId')}
          value={log.runId}
          copied={copied === 'runId'}
          onCopy={() => copy('runId', log.runId)}
          copyLabel={t('jobRunLogIdCopy')}
          copiedLabel={t('jobRunLogIdCopied')}
        />
        <IdField
          label={t('jobRunLogIdTraceId')}
          value={log.traceId}
          copied={copied === 'traceId'}
          onCopy={() => copy('traceId', log.traceId)}
          copyLabel={t('jobRunLogIdCopy')}
          copiedLabel={t('jobRunLogIdCopied')}
        />
      </div>

      {/* System prompt */}
      <details className={['card', styles.promptDetails].join(' ')}>
        <summary className={styles.promptSummary}>
          <IconChevRight size={12} className={styles.promptSummaryChev} />
          <span className={styles.promptSummaryTitle}>{t('jobRunLogSystemPrompt')}</span>
          <span className={styles.promptSummaryLines}>
            {t('jobRunLogSystemPromptLines', { count: promptLines })}
          </span>
          <span className={styles.promptSummaryHint}>{t('jobRunLogSystemPromptHint')}</span>
        </summary>
        <pre className={styles.promptPre}>{log.systemPrompt}</pre>
      </details>

      {/* Timeline */}
      <div className={['card', styles.timelineCard].join(' ')}>
        <div className={styles.timelineHead}>
          <span className={styles.timelineTitle}>{t('jobRunLogTimeline')}</span>
          <span className={styles.timelineCount}>
            {t('jobRunLogTimelineEventCount', { count: events.length })}
          </span>
          <div className={styles.timelineLegend}>
            {(['LLM_REQUEST', 'LLM_RESPONSE', 'TOOL_CALL', 'RUN_ERROR'] as JobRunEventKind[]).map((k) => (
              <span key={k} className={styles.legendItem}>
                <span className={styles.legendDot} style={{ background: EVENT_COLOR[k] }} />
                {k}
              </span>
            ))}
          </div>
        </div>
        <div className={styles.timelineBody}>
          {events.length === 0 ? (
            <div className={styles.statusCard}>—</div>
          ) : (
            events.map((ev, i) => {
              const last = i === events.length - 1
              const color = EVENT_COLOR[ev.kind]
              const isOpen = expanded[ev.eventId] ?? false
              return (
                <div key={ev.eventId} className={styles.eventRow}>
                  <div className={styles.eventTimeCol}>
                    <span className={styles.eventTime}>{formatTime(ev.occurredAt)}</span>
                  </div>
                  <div className={styles.eventRail}>
                    <span
                      className={[
                        styles.eventRailLine,
                        last ? styles.eventRailLineLast : '',
                      ].join(' ')}
                    />
                    <span className={styles.eventRailDot} style={{ background: color }} />
                  </div>
                  <div className={styles.eventBody}>
                    <div className={styles.eventHeader}>
                      <span
                        className={styles.typePill}
                        style={{
                          color,
                          background: `color-mix(in oklch, ${color} 10%, transparent)`,
                        }}
                      >
                        {t(eventLabelKey(ev.kind) as Parameters<typeof t>[0])}
                      </span>
                      <span className={styles.eventSummary}>{summariseEvent(ev)}</span>
                    </div>
                    {(() => {
                      const body = eventContent(ev)
                      if (!body) return null
                      return (
                        <div
                          className={[
                            styles.eventContent,
                            body.muted ? styles.eventContentMuted : '',
                          ].join(' ')}
                        >
                          {body.text}
                        </div>
                      )
                    })()}
                    {eventMeta(ev).length > 0 && (
                      <div className={styles.eventMeta}>
                        {eventMeta(ev).map((m) => (
                          <span key={m.key}>
                            <span>{m.key}</span>{' '}
                            <span className="mono">{m.value}</span>
                          </span>
                        ))}
                      </div>
                    )}
                    {isOpen && (
                      <div className={styles.eventDetail}>
                        <pre>{formatJsonSafe(ev.payload)}</pre>
                      </div>
                    )}
                  </div>
                  <button
                    type="button"
                    className={styles.eventExpandBtn}
                    onClick={() => setExpanded((curr) => ({ ...curr, [ev.eventId]: !curr[ev.eventId] }))}
                  >
                    {isOpen ? t('jobRunLogEventCollapse') : t('jobRunLogEventExpand')}
                  </button>
                </div>
              )
            })
          )}
        </div>
      </div>
    </div>
  )
}

function formatJsonSafe(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

interface MetaKpiProps {
  label: string
  value: string
  mono?: boolean
}
function MetaKpi({ label, value, mono }: MetaKpiProps) {
  return (
    <div className={styles.metaKpi}>
      <div className={styles.metaKpiLabel}>{label}</div>
      <div className={[styles.metaKpiValue, mono ? 'mono' : ''].join(' ')}>{value}</div>
    </div>
  )
}

interface IdFieldProps {
  label: string
  value: string
  copied: boolean
  onCopy: () => void
  copyLabel: string
  copiedLabel: string
}
function IdField({ label, value, copied, onCopy, copyLabel, copiedLabel }: IdFieldProps) {
  return (
    <div className={styles.idRow}>
      <span className={styles.idLabel}>{label}</span>
      <code className={styles.idChip}>{value}</code>
      <button
        type="button"
        className={[styles.copyBtn, copied ? styles.copyBtnOk : ''].join(' ')}
        onClick={onCopy}
        title={copyLabel}
      >
        {copied ? copiedLabel : copyLabel}
      </button>
    </div>
  )
}
