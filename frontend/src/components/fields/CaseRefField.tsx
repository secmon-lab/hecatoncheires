import { useState, useEffect } from 'react'
import Select from 'react-select'
import { useQuery } from '@apollo/client'
import { REFERENCEABLE_CASES } from '../../graphql/caseRef'
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

  if (multi) {
    const selectedValues = Array.isArray(value) ? value : []
    const selectedOptions = options.filter((o) => selectedValues.includes(o.value))

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
  const selectedOption = options.find((o) => o.value === singleValue) ?? null

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
