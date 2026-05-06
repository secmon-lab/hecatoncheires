import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import {
  ADD_ACTION_STEP,
  DELETE_ACTION_STEP,
  GET_ACTION_STEPS,
  RENAME_ACTION_STEP,
  SET_ACTION_STEP_DONE,
} from '../graphql/actionStep'
import { useTranslation } from '../i18n'
import { commitOnEnter } from '../utils/keyboard'
import styles from './StepList.module.css'

interface Step {
  id: string
  actionID: number
  title: string
  done: boolean
  doneAt?: string | null
  createdAt: string
  updatedAt: string
}

interface Props {
  workspaceId: string
  actionId: number
}

// StepList renders the binary-state checklist for an Action. By design the
// only data shown to the user is checkbox + title + (on hover) delete. All
// other backend metadata (creator, doneBy, timestamps) is intentionally
// hidden — the WebUI stays minimal; the LLM tool path can still see it.
export default function StepList({ workspaceId, actionId }: Props) {
  const { t } = useTranslation()
  const refetchVars = { workspaceId, id: actionId }

  const { data } = useQuery(GET_ACTION_STEPS, {
    variables: refetchVars,
  })

  const steps: Step[] = data?.action?.steps ?? []
  const progress = data?.action?.stepProgress ?? { done: 0, total: 0 }

  const refetchOpts = {
    refetchQueries: [{ query: GET_ACTION_STEPS, variables: refetchVars }],
  }

  const [addStep] = useMutation(ADD_ACTION_STEP, refetchOpts)
  const [setDone] = useMutation(SET_ACTION_STEP_DONE, refetchOpts)
  const [renameStep] = useMutation(RENAME_ACTION_STEP, refetchOpts)
  const [deleteStep] = useMutation(DELETE_ACTION_STEP, refetchOpts)

  const [draft, setDraft] = useState('')

  const submitNew = async () => {
    const title = draft.trim()
    if (title === '') return
    setDraft('')
    await addStep({
      variables: { workspaceId, input: { actionId, title } },
    })
  }

  const handleToggle = async (step: Step) => {
    await setDone({
      variables: { workspaceId, input: { actionId, stepId: step.id, done: !step.done } },
    })
  }

  const handleRename = async (step: Step, nextTitle: string) => {
    const title = nextTitle.trim()
    if (title === '' || title === step.title) return
    await renameStep({
      variables: { workspaceId, input: { actionId, stepId: step.id, title } },
    })
  }

  const handleDelete = async (step: Step) => {
    await deleteStep({
      variables: { workspaceId, input: { actionId, stepId: step.id } },
    })
  }

  return (
    <div data-testid="action-step-list">
      <div className={styles.header}>
        <span className={styles.headerTitle}>{t('sectionActionSteps')}</span>
        {progress.total > 0 && (
          <span className={styles.progressBadge} data-testid="action-step-progress">
            {progress.done}/{progress.total}
          </span>
        )}
      </div>

      <div className={styles.stepList}>
        {steps.length === 0 ? (
          <div className={styles.empty}>{t('emptyActionSteps')}</div>
        ) : (
          steps.map((step) => (
            <StepRow
              key={step.id}
              step={step}
              onToggle={() => handleToggle(step)}
              onRename={(next) => handleRename(step, next)}
              onDelete={() => handleDelete(step)}
              ariaCheckboxLabel={t('ariaActionStepCheckbox')}
              ariaDeleteLabel={t('ariaDeleteActionStep')}
            />
          ))
        )}

        <div className={styles.addRow}>
          <input
            className={styles.addInput}
            type="text"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={commitOnEnter({
              onCommit: () => {
                void submitNew()
              },
            })}
            placeholder={t('placeholderAddActionStep')}
            data-testid="action-step-add-input"
            aria-label={t('placeholderAddActionStep')}
          />
        </div>
      </div>
    </div>
  )
}

interface StepRowProps {
  step: Step
  onToggle: () => Promise<void> | void
  onRename: (next: string) => Promise<void> | void
  onDelete: () => Promise<void> | void
  ariaCheckboxLabel: string
  ariaDeleteLabel: string
}

export function StepRow({ step, onToggle, onRename, onDelete, ariaCheckboxLabel, ariaDeleteLabel }: StepRowProps) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(step.title)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!editing) setDraft(step.title)
  }, [step.title, editing])

  useEffect(() => {
    if (editing) {
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.select()
      })
    }
  }, [editing])

  const commit = async () => {
    const next = draft.trim()
    if (next === '' || next === step.title) {
      setEditing(false)
      setDraft(step.title)
      return
    }
    setEditing(false)
    await onRename(next)
  }

  const cancel = () => {
    setEditing(false)
    setDraft(step.title)
  }

  return (
    <div className={styles.row} data-testid={`action-step-row-${step.id}`}>
      <input
        type="checkbox"
        checked={step.done}
        onChange={() => {
          void onToggle()
        }}
        aria-label={ariaCheckboxLabel}
        data-testid={`action-step-checkbox-${step.id}`}
      />
      {editing ? (
        <input
          ref={inputRef}
          className={styles.titleEdit}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={commitOnEnter({
            onCommit: () => {
              void commit()
            },
            onCancel: cancel,
          })}
          onBlur={() => {
            void commit()
          }}
          data-testid={`action-step-edit-${step.id}`}
        />
      ) : (
        <button
          type="button"
          className={`${styles.titleButton} ${step.done ? styles.titleDone : ''}`}
          onClick={() => setEditing(true)}
          data-testid={`action-step-title-${step.id}`}
        >
          {step.title}
        </button>
      )}
      <button
        type="button"
        className={styles.deleteButton}
        onClick={() => {
          void onDelete()
        }}
        aria-label={ariaDeleteLabel}
        data-testid={`action-step-delete-${step.id}`}
      >
        ×
      </button>
    </div>
  )
}
