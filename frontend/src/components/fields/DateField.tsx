import { calendarDate } from '../../utils/calendarDate'
import styles from './FieldComponents.module.css'

interface DateFieldProps {
  fieldId: string
  label: string
  value: string
  onChange: (value: string) => void
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

export default function DateField({
  fieldId,
  label,
  value,
  onChange,
  required = false,
  description,
  error,
  disabled = false,
}: DateFieldProps) {
  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <input
        id={fieldId}
        type="date"
        className={`${styles.input} ${error ? styles.inputError : ''}`}
        // The date control uses YYYY-MM-DD, while the field validator and
        // storage use RFC3339. Preserve the selected calendar day in UTC.
        value={calendarDate(value)}
        onChange={(e) => onChange(e.target.value ? `${e.target.value}T00:00:00Z` : '')}
        disabled={disabled}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
