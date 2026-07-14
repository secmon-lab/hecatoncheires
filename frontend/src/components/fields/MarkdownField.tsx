import MarkdownEditor from '../markdown/MarkdownEditor'
import styles from './FieldComponents.module.css'

interface MarkdownFieldProps {
  fieldId: string
  label: string
  value: string
  onChange: (value: string) => void
  required?: boolean
  description?: string
  error?: string
  disabled?: boolean
}

// Form field for a Markdown custom field: the standard label / description /
// error chrome (shared with the other *Field components) wrapping the Write /
// Preview MarkdownEditor. Used in the Memo form modal and the Case create form,
// both of which have the full width for an inline preview.
export default function MarkdownField({
  fieldId,
  label,
  value,
  onChange,
  required = false,
  description,
  error,
  disabled = false,
}: MarkdownFieldProps) {
  return (
    <div className={styles.field}>
      <label htmlFor={fieldId} className={styles.label}>
        {label}
        {required && <span className={styles.required}>*</span>}
      </label>
      {description && <p className={styles.description}>{description}</p>}
      <MarkdownEditor
        value={value}
        onChange={onChange}
        disabled={disabled}
        testId={fieldId}
      />
      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
