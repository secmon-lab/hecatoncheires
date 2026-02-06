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
  const selectedOption = options.find((opt) => opt.id === value)

  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <select
        id={fieldId}
        className={`${styles.select} ${error ? styles.inputError : ''}`}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      >
        <option value="">-- Select --</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
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
