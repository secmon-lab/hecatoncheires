import { useState, useEffect } from 'react'
import { useQuery } from '@apollo/client'
import InlineText from './InlineText'
import InlineLongText from './InlineLongText'
import InlineNumber from './InlineNumber'
import InlineSelect from './InlineSelect'
import InlineMultiSelect from './InlineMultiSelect'
import InlineUserSelect, { type UserItem } from './InlineUserSelect'
import InlineDate from './InlineDate'
import InlineURL from './InlineURL'
import InlineCaseSelect from './InlineCaseSelect'
import InlineMultiCaseSelect from './InlineMultiCaseSelect'
import { REFERENCEABLE_CASES } from '../../graphql/caseRef'

interface FieldOption {
  id: string
  name: string
  description?: string | null
}

interface FieldDefinition {
  id: string
  name: string
  type: string
  required?: boolean
  description?: string | null
  options?: FieldOption[] | null
  referenceWorkspaceId?: string | null
}

interface Props {
  field: FieldDefinition
  value: any
  users?: UserItem[]
  disabled?: boolean
  onSave: (next: any) => Promise<void> | void
  testId?: string
  /** Hint that this TEXT field is multi-line. */
  longText?: boolean
}

interface CaseRefLoaderProps {
  field: FieldDefinition
  value: any
  onSave: (next: any) => Promise<void> | void
  disabled?: boolean
  testId: string
  multi: boolean
}

function CaseRefInlineLoader({ field, value, onSave, disabled, testId, multi }: CaseRefLoaderProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(searchQuery), 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const { data, loading } = useQuery(REFERENCEABLE_CASES, {
    variables: {
      workspaceId: field.referenceWorkspaceId ?? '',
      query: debouncedQuery || undefined,
      limit: 50,
    },
    skip: !field.referenceWorkspaceId,
  })

  const cases = data?.referenceableCases ?? []

  if (multi) {
    return (
      <InlineMultiCaseSelect
        cases={cases}
        values={Array.isArray(value) ? value : []}
        onSave={(v) => onSave(v)}
        ariaLabel={field.name}
        placeholder="—"
        disabled={disabled}
        testId={testId}
        loading={loading}
        onSearchChange={setSearchQuery}
      />
    )
  }
  return (
    <InlineCaseSelect
      cases={cases}
      value={value ?? null}
      onSave={(v) => onSave(v)}
      ariaLabel={field.name}
      placeholder="—"
      disabled={disabled}
      testId={testId}
      loading={loading}
      onSearchChange={setSearchQuery}
    />
  )
}

// Inline edit renderer for custom fields. Maps field.type → the appropriate
// Inline* component; saves immediately (or via Save button for long text).
export default function InlineCustomField({
  field, value, users = [], disabled, onSave, testId, longText,
}: Props) {
  const tid = testId ?? `field-${field.id}`
  const placeholder = '—'
  const ariaLabel = field.name

  switch (field.type) {
    case 'TEXT':
      return longText ? (
        <InlineLongText
          value={value || ''}
          onSave={(s) => onSave(s)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      ) : (
        <InlineText
          value={value || ''}
          onSave={(s) => onSave(s)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
          allowEmpty
        />
      )

    case 'NUMBER':
      return (
        <InlineNumber
          value={value}
          onSave={(n) => onSave(n)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      )

    case 'SELECT': {
      const opts = (field.options || []).map((o) => ({
        value: o.id,
        label: o.name,
      }))
      return (
        <InlineSelect
          value={value || null}
          options={opts}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
          searchable={opts.length > 8}
        />
      )
    }

    case 'MULTI_SELECT': {
      const opts = (field.options || []).map((o) => ({
        value: o.id,
        label: o.name,
      }))
      return (
        <InlineMultiSelect
          values={Array.isArray(value) ? value : []}
          options={opts}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
          searchable={opts.length > 8}
        />
      )
    }

    case 'USER':
      return (
        <InlineUserSelect
          users={users}
          value={value || null}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      )

    case 'MULTI_USER':
      return (
        <InlineUserSelect
          users={users}
          isMulti
          value={Array.isArray(value) ? value : []}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      )

    case 'DATE':
      return (
        <InlineDate
          value={value || null}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      )

    case 'URL':
      return (
        <InlineURL
          value={value || ''}
          onSave={(v) => onSave(v)}
          ariaLabel={ariaLabel}
          placeholder={placeholder}
          disabled={disabled}
          testId={tid}
        />
      )

    case 'CASE_REF':
      return (
        <CaseRefInlineLoader
          field={field}
          value={value}
          onSave={onSave}
          disabled={disabled}
          testId={tid}
          multi={false}
        />
      )

    case 'MULTI_CASE_REF':
      return (
        <CaseRefInlineLoader
          field={field}
          value={value}
          onSave={onSave}
          disabled={disabled}
          testId={tid}
          multi={true}
        />
      )

    default:
      return <span style={{ fontSize: 13, color: 'var(--fg-muted)' }}>Unsupported: {field.type}</span>
  }
}
