// Shared memo field-definition types and normalization.
//
// The GraphQL memoConfiguration.fields payload types its optional values as
// `| null`; UI components (MemoTab, MemoDetail, CustomFieldRenderer,
// sanitizeFieldValues) share the non-null FieldDef shape instead. This module
// owns the conversion so every memo surface normalizes identically.

export interface FieldOption {
  id: string
  name: string
  description?: string
  metadata?: Record<string, unknown>
}

export interface FieldDef {
  id: string
  name: string
  type: string
  required: boolean
  description?: string
  options?: FieldOption[]
}

export interface RawFieldOption {
  id: string
  name: string
  description?: string | null
  metadata?: Record<string, unknown> | null
}

export interface RawFieldDef {
  id: string
  name: string
  type: string
  required: boolean
  description?: string | null
  options?: RawFieldOption[] | null
}

export function normalizeMemoFields(fields: RawFieldDef[] | null | undefined): FieldDef[] {
  return (fields ?? []).map((f) => ({
    id: f.id,
    name: f.name,
    type: f.type,
    required: f.required,
    description: f.description ?? undefined,
    options: f.options
      ? f.options.map((o) => ({
          id: o.id,
          name: o.name,
          description: o.description ?? undefined,
          metadata: o.metadata ?? undefined,
        }))
      : undefined,
  }))
}
