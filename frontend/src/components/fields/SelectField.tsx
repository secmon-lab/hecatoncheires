import Select from 'react-select'
import { useTranslation } from '../../i18n'
import { buildSelectStyles, portalProps } from '../selectStyles'
import styles from './FieldComponents.module.css'

interface SelectOption {
  id: string
  name: string
  description?: string
  color?: string
  metadata?: Record<string, any>
}

interface SelectFieldProps {
  fieldId: string
  label: string
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
  showMetadata?: boolean
}

export default function SelectField({
  fieldId,
  label,
  value,
  onChange,
  options,
  required = false,
  description,
  error,
  disabled = false,
  showMetadata = false,
}: SelectFieldProps) {
  const { t } = useTranslation()
  const selectedOption = options.find((opt) => opt.id === value)
  const rsOptions = options.map((o) => ({ value: o.id, label: o.name }))
  const rsValue = selectedOption ? { value: selectedOption.id, label: selectedOption.name } : null

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
        options={rsOptions}
        value={rsValue}
        isDisabled={disabled}
        isClearable={!required}
        placeholder={t('placeholderSelect')}
        classNamePrefix="rs"
        {...portalProps}
        styles={buildSelectStyles({ error: !!error })}
        onChange={(opt: any) => onChange(opt ? opt.value : '')}
      />
      {showMetadata && selectedOption?.metadata && (() => {
        const meta = typeof selectedOption.metadata === 'string'
          ? (() => { try { return JSON.parse(selectedOption.metadata) } catch { return null } })()
          : selectedOption.metadata
        if (!meta || typeof meta !== 'object') return null
        return (
          <div className={styles.metadata}>
            {Object.entries(meta).map(([key, val]) => (
              <div key={key} className={styles.metadataItem}>
                <span className={styles.metadataKey}>{key}:</span>
                <span className={styles.metadataValue}>{String(val)}</span>
              </div>
            ))}
          </div>
        )
      })()}
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
