import Select from 'react-select'
import styles from './FieldComponents.module.css'

interface SelectOption {
  id: string
  name: string
  description?: string
  color?: string
}

interface MultiSelectFieldProps {
  fieldId: string
  label: string
  value: string[]
  onChange: (value: string[]) => void
  options: SelectOption[]
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

export default function MultiSelectField({
  fieldId,
  label,
  value,
  onChange,
  options,
  required = false,
  description,
  error,
  disabled = false,
}: MultiSelectFieldProps) {
  const selectOptions = options.map((option) => ({
    value: option.id,
    label: option.name,
  }))

  const selectedOptions = selectOptions.filter((opt) =>
    value.includes(opt.value)
  )

  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <Select
        inputId={fieldId}
        options={selectOptions}
        value={selectedOptions}
        onChange={(options) => onChange(options?.map((opt) => opt.value) || [])}
        isMulti
        isClearable
        isDisabled={disabled}
        placeholder="Select..."
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
