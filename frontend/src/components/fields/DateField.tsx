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
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
