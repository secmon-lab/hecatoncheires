import { useState } from 'react'

import { useTranslation } from '../../i18n'
import {
  IconCalendar,
  IconCheck,
  IconChevRight,
  IconPlus,
  IconRefresh,
  IconRobot,
  IconWarn,
} from '../Icons'
import { intervalLabel } from '../../utils/jobTrigger'
import styles from './CaseJobList.module.css'

export type CaseLifecycleEvent = 'CREATED' | 'CLOSED'

export interface JobSchedule {
  everySeconds: number | null
  cron: string | null
}

export interface JobTrigger {
  caseEvents: CaseLifecycleEvent[]
  schedule: JobSchedule | null
}

export interface CaseJob {
  id: string
  name: string
  description: string
  strategy: 'SIMPLE' | 'PLANEXEC'
  quiet: boolean
  prompt: string
  trigger: JobTrigger
}

interface CaseJobListProps {
  jobs: CaseJob[]
  loading: boolean
  error: boolean
  onRetry: () => void
}

export default function CaseJobList({ jobs, loading, error, onRetry }: CaseJobListProps) {
  const { t } = useTranslation()
  // Multiple rows may be expanded at once; the design lets operators
  // compare two prompts side by side without one collapsing the other.
  const [openIds, setOpenIds] = useState<Set<string>>(new Set())

  const toggle = (id: string) =>
    setOpenIds((curr) => {
      const next = new Set(curr)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  return (
    <>
      <p className={styles.subtitle}>{t('caseAgentJobsSubtitle')}</p>
      <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
        {error ? (
          <div className={styles.state}>
            <span className={[styles.stateIcon, styles.stateIconError].join(' ')}>
              <IconWarn size={22} />
            </span>
            <div className={styles.stateTitle}>{t('caseAgentJobsError')}</div>
            <div className={styles.stateDesc}>{t('caseAgentJobsErrorDesc')}</div>
            <button type="button" className={styles.retryBtn} onClick={onRetry}>
              <IconRefresh size={14} />
              {t('caseAgentJobsRetry')}
            </button>
          </div>
        ) : loading && jobs.length === 0 ? (
          <div className={styles.skeletonWrap}>
            {[0, 1, 2].map((i) => (
              <div key={i} className={styles.skeletonRow}>
                <div className={styles.skeletonBody}>
                  <span className={styles.skeletonLineWide} />
                  <span className={styles.skeletonLineNarrow} />
                </div>
                <span className={styles.skeletonBadge} />
              </div>
            ))}
          </div>
        ) : jobs.length === 0 ? (
          <div className={styles.state}>
            <span className={styles.stateIcon}>
              <IconRobot size={22} sw={1.7} />
            </span>
            <div className={styles.stateTitle}>{t('caseAgentJobsEmpty')}</div>
            <div className={styles.stateDesc}>{t('caseAgentJobsEmptyDesc')}</div>
          </div>
        ) : (
          jobs.map((job) => (
            <JobRow
              key={job.id}
              job={job}
              open={openIds.has(job.id)}
              onToggle={() => toggle(job.id)}
            />
          ))
        )}
      </div>
    </>
  )
}

function JobRow({
  job,
  open,
  onToggle,
}: {
  job: CaseJob
  open: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.row}>
      {/* A native <button> handles Enter/Space activation itself, so no
          IME-composition guard is required (see frontend-keyboard rule). */}
      <button
        type="button"
        className={styles.rowHeader}
        onClick={onToggle}
        aria-expanded={open}
      >
        <span className={styles.rowMain}>
          <span className={styles.rowName}>{job.name}</span>
          {job.description && <span className={styles.rowDesc}>{job.description}</span>}
        </span>

        <span className={styles.rowMeta}>
          <TriggerBadges trigger={job.trigger} />
          {job.strategy === 'PLANEXEC' && (
            <span className={[styles.chip, styles.chipStrategy].join(' ')}>
              {t('jobStrategyPlanexec')}
            </span>
          )}
          {job.quiet && (
            <span
              className={[styles.chip, styles.chipQuiet].join(' ')}
              title={t('caseAgentJobQuietTitle')}
            >
              {t('caseAgentJobQuiet')}
            </span>
          )}
        </span>

        <span className={[styles.chevron, open ? styles.chevronOpen : ''].join(' ')}>
          <IconChevRight size={16} />
        </span>
      </button>

      {open && (
        <div className={styles.promptWrap}>
          <div className={styles.promptLabelRow}>
            <span className={styles.promptLabel}>{t('caseAgentJobPromptLabel')}</span>
            <span className={styles.promptRule} />
          </div>
          <pre className={styles.promptBody}>{job.prompt}</pre>
        </div>
      )}
    </div>
  )
}

function TriggerBadges({ trigger }: { trigger: JobTrigger }) {
  const { t } = useTranslation()
  const badges: React.ReactNode[] = []

  for (const ev of trigger.caseEvents) {
    if (ev === 'CREATED') {
      badges.push(
        <span key="created" className={[styles.badge, styles.badgeCase].join(' ')}>
          <IconPlus size={13} />
          {t('caseAgentJobTriggerCreated')}
        </span>,
      )
    } else if (ev === 'CLOSED') {
      badges.push(
        <span key="closed" className={[styles.badge, styles.badgeCase].join(' ')}>
          <IconCheck size={13} />
          {t('caseAgentJobTriggerClosed')}
        </span>,
      )
    }
  }

  const sched = trigger.schedule
  if (sched?.everySeconds != null) {
    const { key, count } = intervalLabel(sched.everySeconds)
    badges.push(
      <span key="interval" className={[styles.badge, styles.badgeSchedule].join(' ')}>
        <IconRefresh size={13} />
        {t(key, { count })}
      </span>,
    )
  }
  if (sched?.cron != null) {
    badges.push(
      <span key="cron" className={[styles.badge, styles.badgeSchedule].join(' ')}>
        <IconCalendar size={13} />
        <span>{t('caseAgentJobCronLabel')}</span>
        <code className={styles.cronExpr}>{sched.cron}</code>
      </span>,
    )
  }

  return <>{badges}</>
}
