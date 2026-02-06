import TextField from './TextField'
import NumberField from './NumberField'
import SelectField from './SelectField'
import MultiSelectField from './MultiSelectField'
import UserField from './UserField'
import MultiUserField from './MultiUserField'
import DateField from './DateField'
import URLField from './URLField'

interface FieldOption {
  id: string
  name: string
  description?: string
  color?: string
  metadata?: Record<string, any>
}

interface FieldDefinition {
  id: string
  name: string
  type: string
  required: boolean
  description?: string
  options?: FieldOption[]
}

interface User {
  id: string
  name: string
  realName: string
  imageUrl?: string
}

interface CustomFieldRendererProps {
  field: FieldDefinition
  value: any
  onChange: (fieldID: string, value: any) => void
  users?: User[]
  error?: string
  disabled?: boolean
  showMetadata?: boolean
}

export default function CustomFieldRenderer({
  field,
  value,
  onChange,
  users = [],
  error,
  disabled = false,
  showMetadata = false,
}: CustomFieldRendererProps) {
  const handleChange = (val: any) => {
    onChange(field.id, val)
  }

  switch (field.type) {
    case 'TEXT':
      return (
        <TextField
          fieldId={field.id}
          label={field.name}
          value={value || ''}
          onChange={handleChange}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'NUMBER':
      return (
        <NumberField
          fieldId={field.id}
          label={field.name}
          value={value}
          onChange={handleChange}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'SELECT':
      return (
        <SelectField
          fieldId={field.id}
          label={field.name}
          value={value || ''}
          onChange={handleChange}
          options={field.options || []}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
          showMetadata={showMetadata}
        />
      )

    case 'MULTI_SELECT':
      return (
        <MultiSelectField
          fieldId={field.id}
          label={field.name}
          value={value || []}
          onChange={handleChange}
          options={field.options || []}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'USER':
      return (
        <UserField
          fieldId={field.id}
          label={field.name}
          value={value || ''}
          onChange={handleChange}
          users={users}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'MULTI_USER':
      return (
        <MultiUserField
          fieldId={field.id}
          label={field.name}
          value={value || []}
          onChange={handleChange}
          users={users}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'DATE':
      return (
        <DateField
          fieldId={field.id}
          label={field.name}
          value={value || ''}
          onChange={handleChange}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    case 'URL':
      return (
        <URLField
          fieldId={field.id}
          label={field.name}
          value={value || ''}
          onChange={handleChange}
          required={field.required}
          description={field.description}
          error={error}
          disabled={disabled}
        />
      )

    default:
      return (
        <div>
          <p>Unsupported field type: {field.type}</p>
        </div>
      )
  }
}
