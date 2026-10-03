import { useId, useMemo, useState } from 'react'
import { useQuery } from '@apollo/client'
import Select from 'react-select'
import { CASE_REFS_BY_IDS } from '../graphql/caseRef'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { useTranslation } from '../i18n'
import type { CaseFieldDefinition } from '../utils/caseFieldFilters'
import { commitOnEnter } from '../utils/keyboard'
import { displayName, type NameableUser } from '../utils/user'
import Button from './Button'
import { buildSelectStyles, portalProps } from './selectStyles'
import styles from './CaseFieldFilters.module.css'

interface Option { value: string; label: string }
interface Props {
  field: CaseFieldDefinition
  values: readonly string[]
  onChange: (values: readonly string[]) => void
  referenceValues?: readonly string[]
  summary?: boolean
  menuTarget?: HTMLDivElement | null
}
interface ChoiceProps extends Props {
  options: Option[]
  loading?: boolean
  error?: boolean
}

function ValueChips({ field, values, onChange, options = [] }: Props & { options?: Option[] }) {
  const { t } = useTranslation()
  return (
    <span className={styles.values}>
      {values.map((value) => {
        const label = options.find((o) => o.value === value)?.label ?? value
        return (
          <Button key={value} size="sm" variant="ghost" className={styles.valueChip}
            aria-label={t('filterRemoveValue', { field: field.name, value: label })}
            onClick={() => onChange(values.filter((v) => v !== value))}>
            {label} ×
          </Button>
        )
      })}
    </span>
  )
}

function ChoiceValues(props: ChoiceProps) {
  const { field, values, onChange, summary, options, loading, error, menuTarget } = props
  const { t } = useTranslation()
  const inputId = useId()
  if (summary) return <ValueChips {...props} />
  const selected = values.map((value) => options.find((o) => o.value === value) ?? { value, label: value })
  return (
    <>
      <Select<Option, true>
        inputId={inputId}
        aria-label={field.name}
        isMulti
        options={options}
        value={selected}
        onChange={(next) => onChange(next.map((o) => o.value))}
        closeMenuOnSelect
        isLoading={loading}
        placeholder={t('filterSelectValues')}
        noOptionsMessage={() => t('filterNoOptions')}
        styles={buildSelectStyles({ compact: true })}
        {...portalProps}
        menuPortalTarget={menuTarget}
      />
      {error && <span role="status" className={styles.hint}>{t('filterOptionsUnavailable')}</span>}
    </>
  )
}

function UserValues(props: Props) {
  const { data, loading, error } = useQuery<{ slackUsers: NameableUser[] }>(GET_SLACK_USERS)
  const options = (data?.slackUsers ?? []).map((u) => ({ value: u.id!, label: `${displayName(u)} (@${u.name || u.id})` }))
  return <ChoiceValues {...props} options={options} loading={loading} error={!!error} />
}

function CaseValues(props: Props) {
  const { field, values, summary, referenceValues } = props
  const ids = useMemo(() => [...new Set([
    ...values, ...(summary ? [] : referenceValues ?? []),
  ].map(Number).filter((id) => Number.isSafeInteger(id) && id > 0 && id <= 2147483647))], [values, summary, referenceValues])
  const { data, loading, error } = useQuery(CASE_REFS_BY_IDS, {
    variables: { workspaceId: field.referenceWorkspaceId, ids },
    skip: !field.referenceWorkspaceId || ids.length === 0,
  })
  const options: Option[] = (data?.caseRefsByIds ?? []).map((c: { id: number; title: string }) => ({
    value: String(c.id), label: `${c.title} (#${c.id})`,
  }))
  return <ChoiceValues {...props} options={options} loading={loading} error={!!error} />
}

function ScalarValues(props: Props) {
  const { field, values, onChange, summary } = props
  const { t } = useTranslation()
  const inputId = useId()
  const [draft, setDraft] = useState('')
  const valid = draft !== '' && (field.type !== 'NUMBER' || Number.isFinite(Number(draft)))
  const add = () => {
    if (!valid) return
    onChange([...new Set([...values, draft])])
    setDraft('')
  }
  if (summary) return <ValueChips {...props} />
  return (
    <>
      <ValueChips {...props} />
      <div className={styles.scalarInput}>
        <input
          id={inputId}
          aria-label={field.name}
          type={field.type === 'NUMBER' ? 'number' : field.type === 'DATE' ? 'date' : 'text'}
          step={field.type === 'NUMBER' ? 'any' : undefined}
          value={draft}
          placeholder={t('filterCaseFieldExact')}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={commitOnEnter({ onCommit: add })}
        />
        <Button size="sm" disabled={!valid} onClick={add}>{t('filterAddValue')}</Button>
      </div>
    </>
  )
}

export default function CaseFieldFilterValue(props: Props) {
  switch (props.field.type) {
    case 'SELECT': case 'MULTI_SELECT':
      return <ChoiceValues {...props} options={(props.field.options ?? []).map((o) => ({ value: o.id, label: o.name }))} />
    case 'USER': case 'MULTI_USER': return <UserValues {...props} />
    case 'CASE_REF': case 'MULTI_CASE_REF': return <CaseValues {...props} />
    default: return <ScalarValues {...props} />
  }
}
