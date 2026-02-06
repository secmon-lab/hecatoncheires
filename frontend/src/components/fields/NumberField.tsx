import styles from './FieldComponents.module.css'

interface NumberFieldProps {
  fieldId: string
  label: string
  value: number | null
  onChange: (value: number | null) => void
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

export default function NumberField({
  fieldId,
  label,
  value,
  onChange,
  required = false,
  description,
  error,
  disabled = false,
}: NumberFieldProps) {
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value
    if (val === '') {
      onChange(null)
    } else {
      const num = parseFloat(val)
      if (!isNaN(num)) {
        onChange(num)
      }
    }
  }

  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <input
        id={fieldId}
        type="number"
        step="any"
        className={`${styles.input} ${error ? styles.inputError : ''}`}
        value={value ?? ''}
        onChange={handleChange}
        disabled={disabled}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
