import Select from 'react-select'
import styles from './FieldComponents.module.css'

interface User {
  id: string
  name: string
  realName: string
  imageUrl?: string
}

interface UserFieldProps {
  fieldId: string
  label: string
  value: string
  onChange: (value: string) => void
  users: User[]
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

export default function UserField({
  fieldId,
  label,
  value,
  onChange,
  users,
  required = false,
  description,
  error,
  disabled = false,
}: UserFieldProps) {
  const options = users.map((user) => ({
    value: user.id,
    label: user.realName || user.name,
    name: user.name,
    realName: user.realName,
    image: user.imageUrl,
  }))

  const selectedOption = options.find((opt) => opt.value === value) || null

  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <Select
        inputId={fieldId}
        options={options}
        value={selectedOption}
        onChange={(option) => onChange(option?.value || '')}
        isClearable
        isDisabled={disabled}
        placeholder="Select a user..."
        filterOption={(option, inputValue) => {
          const search = inputValue.toLowerCase()
          const data = option.data
          return (
            data.label.toLowerCase().includes(search) ||
            data.name.toLowerCase().includes(search) ||
            data.realName.toLowerCase().includes(search)
          )
        }}
        formatOptionLabel={(option) => (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {option.image && (
              <img
                src={option.image}
                alt={option.label}
                style={{ width: '24px', height: '24px', borderRadius: '50%' }}
              />
            )}
            <span>{option.label}</span>
          </div>
        )}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
