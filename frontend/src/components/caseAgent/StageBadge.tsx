import { useTranslation } from '../../i18n'
import styles from '../../pages/CaseAgent.module.css'

export type JobRunStage = 'RUNNING' | 'SUCCESS' | 'FAILED' | 'AWAITING_INPUT'

interface Props {
  stage: JobRunStage
  size?: 'sm' | 'md'
}

const variantMap: Record<JobRunStage, string> = {
  RUNNING: styles.stageRunning,
  SUCCESS: styles.stageSuccess,
  FAILED: styles.stageFailed,
  AWAITING_INPUT: styles.stageAwaitingInput,
}

// StageBadge mirrors the design's StageBadge: a pill with a stage-coloured
// background, a 1px stroke, and a leading dot. The RUNNING dot pulses via
// the keyframe defined in CaseAgent.module.css.
export default function StageBadge({ stage, size = 'md' }: Props) {
  const { t } = useTranslation()
  const label = {
    RUNNING: t('caseAgentRunStageRunning'),
    SUCCESS: t('caseAgentRunStageSuccess'),
    FAILED: t('caseAgentRunStageFailed'),
    AWAITING_INPUT: t('caseAgentRunStageAwaitingInput'),
  }[stage]
  const cls = [styles.stageBadge, variantMap[stage]]
  if (size === 'sm') cls.push(styles.stageBadgeSm)
  return (
    <span className={cls.join(' ')}>
      <span className={[styles.stageDot, size === 'sm' ? styles.stageDotSm : ''].join(' ')} />
      {label}
    </span>
  )
}
