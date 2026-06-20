import { useState, useEffect, useMemo } from 'react'
import Select from 'react-select'
import { useQuery } from '@apollo/client'
import { REFERENCEABLE_CASES, CASE_REFS_BY_IDS } from '../../graphql/caseRef'
import { buildSelectStyles, portalProps } from '../selectStyles'
import { useTranslation } from '../../i18n'
import styles from './FieldComponents.module.css'

interface CaseOption {
  value: string
  label: string
}

interface CaseRefFieldProps {
  fieldId: string
  label: string
  value: string | string[]
  onChange: (value: string | string[]) => void
  referenceWorkspaceId: string
  multi?: boolean
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

export default function CaseRefField({
  fieldId,
  label,
  value,
  onChange,
  referenceWorkspaceId,
  multi = false,
  required = false,
  description,
  error,
  disabled = false,
}: CaseRefFieldProps) {
  const { t } = useTranslation()
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(searchQuery)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  // Picker / search results (dropdown options)
  const { data, loading } = useQuery(REFERENCEABLE_CASES, {
    variables: {
      workspaceId: referenceWorkspaceId,
      query: debouncedQuery || undefined,
      limit: 50,
    },
    skip: !referenceWorkspaceId,
  })

  const cases = data?.referenceableCases ?? []
  const options: CaseOption[] = cases.map((c: { id: number; title: string }) => ({
    value: String(c.id),
    label: `${c.title} (#${c.id})`,
  }))

  // Resolve the currently stored value(s) so the selected chip always shows
  // a proper label, even when the stored case is not in the top-50 picker results.
  const currentIds = useMemo(() => {
    const raw = multi ? (Array.isArray(value) ? value : []) : (typeof value === 'string' && value !== '' ? [value] : [])
    return raw.map(Number).filter((n) => !Number.isNaN(n) && n > 0)
  }, [multi, value])

  const { data: resolvedData } = useQuery(CASE_REFS_BY_IDS, {
    variables: { workspaceId: referenceWorkspaceId, ids: currentIds },
    skip: currentIds.length === 0 || !referenceWorkspaceId,
  })

  const resolvedMap = useMemo(() => {
    const entries: Array<[string, string]> = (resolvedData?.caseRefsByIds ?? []).map(
      (c: { id: number; title: string }) => [String(c.id), `${c.title} (#${c.id})`],
    )
    return new Map(entries)
  }, [resolvedData])

  if (multi) {
    const selectedValues = Array.isArray(value) ? value : []
    // Build selected option objects from CASE_REFS_BY_IDS resolution, falling
    // back to the picker results and then to the unavailable fallback.
    const selectedOptions: CaseOption[] = selectedValues.map((id) => {
      const resolvedLabel = resolvedMap.get(id) ?? options.find((o) => o.value === id)?.label
      return {
        value: id,
        label: resolvedLabel ?? t('caseRefUnavailable', { id }),
      }
    })

    return (
      <div className={styles.field}>
        <label htmlFor={fieldId} className={styles.label}>
          {label}
          {required && <span className={styles.required}>*</span>}
        </label>
        {description && <p className={styles.description}>{description}</p>}
        <Select
          inputId={fieldId}
          aria-label={label}
          isMulti
          options={options}
          value={selectedOptions}
          isDisabled={disabled}
          isLoading={loading}
          placeholder={t('placeholderSelectCaseRef')}
          classNamePrefix="rs"
          {...portalProps}
          styles={buildSelectStyles({ error: !!error })}
          onInputChange={(inputValue) => setSearchQuery(inputValue)}
          onChange={(opts: readonly CaseOption[]) =>
            onChange(opts ? opts.map((o) => o.value) : [])
          }
          filterOption={() => true}
        />
        {error && <span className={styles.error}>{error}</span>}
      </div>
    )
  }

  const singleValue = typeof value === 'string' ? value : ''
  // Resolve the selected option from CASE_REFS_BY_IDS first, then fall back
  // to the picker results, so the selected label is always shown correctly.
  const selectedOption: CaseOption | null = singleValue !== ''
    ? {
        value: singleValue,
        label:
          resolvedMap.get(singleValue) ??
          options.find((o) => o.value === singleValue)?.label ??
          t('caseRefUnavailable', { id: singleValue }),
      }
    : null

  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <Select
        inputId={fieldId}
        aria-label={label}
        options={options}
        value={selectedOption}
        isDisabled={disabled}
        isClearable={!required}
        isLoading={loading}
        placeholder={t('placeholderSelectCaseRef')}
        classNamePrefix="rs"
        {...portalProps}
        styles={buildSelectStyles({ error: !!error })}
        onInputChange={(inputValue) => setSearchQuery(inputValue)}
        onChange={(opt: CaseOption | null) => onChange(opt ? opt.value : '')}
        filterOption={() => true}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
