import { useMemo, useRef, useState } from 'react'
import InlineCustomField from '../inline/InlineCustomField'
import FieldHelpButton from './FieldHelpButton'
import FieldCatalogPopover, { type FieldDefinitionForHelp } from './FieldCatalogPopover'
import ValueDescTooltip from './ValueDescTooltip'
import { useTranslation } from '../../i18n'

interface UserItem {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

interface FieldOption {
  id: string
  name: string
  description?: string | null
}

export interface CustomFieldDefinition {
  id: string
  name: string
  type: string
  required?: boolean
  description?: string | null
  options?: FieldOption[] | null
}

interface Props {
  field: CustomFieldDefinition
  value: unknown
  users: UserItem[]
  disabled?: boolean
  onSave: (next: unknown) => Promise<void> | void
  testId?: string
}

function fieldHasHelp(field: CustomFieldDefinition): boolean {
  if (field.description) return true
  return Boolean(field.options?.some((o) => o.description))
}

// Single row rendering for the case-detail sidebar custom-field list. Adds
// a (?) toggle that opens FieldCatalogPopover, and wraps SELECT /
// MULTI_SELECT display values with hover/focus tooltips for option
// descriptions.
export default function CustomFieldHelpRow({
  field, value, users, disabled, onSave, testId,
}: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const labelRef = useRef<HTMLSpanElement>(null)

  const hasHelp = useMemo(() => fieldHasHelp(field), [field])
  const helpField: FieldDefinitionForHelp = field

  return (
    <div className="kv-row" data-testid={testId}>
      <span className="kv-label" ref={labelRef} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <span>{field.name}</span>
        {hasHelp && (
          <FieldHelpButton
            ariaLabel={t('fieldHelpAriaToggle', { name: field.name })}
            expanded={open}
            onToggle={() => setOpen((v) => !v)}
            testId={testId ? `${testId}-help` : undefined}
          />
        )}
      </span>
      <span className="kv-value">
        {field.type === 'SELECT' || field.type === 'MULTI_SELECT'
          ? wrapWithTooltips(field, value, () => (
              <InlineCustomField
                field={field}
                value={value}
                users={users}
                disabled={disabled}
                onSave={onSave}
                testId={`field-${field.id}`}
              />
            ))
          : (
            <InlineCustomField
              field={field}
              value={value}
              users={users}
              disabled={disabled}
              onSave={onSave}
              testId={testId ? `${testId}-field` : undefined}
            />
          )}
      </span>
      {hasHelp && (
        <FieldCatalogPopover
          field={helpField}
          value={value}
          anchor={labelRef.current}
          open={open}
          onClose={() => setOpen(false)}
          testId={testId ? `${testId}-catalog` : undefined}
        />
      )}
    </div>
  )
}

// For SELECT/MULTI_SELECT we want the value display (read mode) to surface
// option descriptions on hover. The InlineCustomField shows a clickable
// trigger; we wrap it once with a tooltip resolved against the *current*
// value(s). When multiple values are selected, we expose the combined
// "name → description" mapping in a single tooltip surface, since each
// option's description is also reachable via the (?) catalog.
function wrapWithTooltips(
  field: CustomFieldDefinition,
  value: unknown,
  renderField: () => React.ReactNode,
): React.ReactNode {
  const options = field.options ?? []
  if (options.length === 0) return renderField()

  if (field.type === 'SELECT') {
    const id = typeof value === 'string' ? value : ''
    const opt = options.find((o) => o.id === id)
    if (!opt?.description) return renderField()
    return (
      <ValueDescTooltip name={opt.name} description={opt.description} decorate={false}>
        {renderField()}
      </ValueDescTooltip>
    )
  }

  // MULTI_SELECT — concatenate descriptions of selected options.
  const ids = Array.isArray(value) ? (value as string[]) : []
  const selected = options.filter((o) => ids.includes(o.id) && o.description)
  if (selected.length === 0) return renderField()
  const name = selected.map((o) => o.name).join(' · ')
  const description = selected
    .map((o) => `${o.name}: ${o.description ?? ''}`)
    .join('\n')
  return (
    <ValueDescTooltip name={name} description={description} decorate={false}>
      {renderField()}
    </ValueDescTooltip>
  )
}
