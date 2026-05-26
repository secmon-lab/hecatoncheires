// JSON Schema for the YAML payload accepted by createCaseImport.
//
// The shape is mostly static (version / cases / actions), but the
// per-Case `fields` block is workspace-specific: its allowed keys, types,
// enum values, and required-ness all come from the workspace's
// FieldConfiguration. We therefore expose a builder that takes the
// resolved field definitions and produces a JSON Schema in which the
// `fields` object is fully describable on its own — copy-pasting it into
// a JSON Schema validator now actually validates the workspace contract.

import type { JSONSchema } from './jsonSchemaTypes'

export interface ImportFieldOption {
  id: string
  name?: string | null
}

export interface ImportFieldDefinition {
  id: string
  name?: string | null
  description?: string | null
  /** GraphQL `FieldType` enum value (TEXT / NUMBER / SELECT / ...) */
  type: string
  required: boolean
  options?: ImportFieldOption[] | null
}

/**
 * Build the per-field JSON Schema fragment for a single FieldDefinition.
 * The fragment is suitable for placement under
 * `properties.cases.items.properties.fields.properties.<id>`.
 */
function buildFieldSchema(fd: ImportFieldDefinition): JSONSchema {
  const title = fd.name && fd.name !== '' ? fd.name : fd.id
  const description = fd.description ?? undefined

  switch (fd.type) {
    case 'TEXT':
      return { title, description, type: 'string' }
    case 'NUMBER':
      return { title, description, type: 'number' }
    case 'SELECT': {
      const enums = (fd.options ?? []).map((o) => o.id)
      return enums.length > 0
        ? { title, description, type: 'string', enum: enums }
        : { title, description, type: 'string' }
    }
    case 'MULTI_SELECT': {
      const enums = (fd.options ?? []).map((o) => o.id)
      return {
        title,
        description,
        type: 'array',
        items: enums.length > 0 ? { type: 'string', enum: enums } : { type: 'string' },
      }
    }
    case 'USER':
      return {
        title,
        description: description ?? 'Slack user ID',
        type: 'string',
      }
    case 'MULTI_USER':
      return {
        title,
        description: description ?? 'Slack user IDs',
        type: 'array',
        items: { type: 'string' },
      }
    case 'DATE':
      return {
        title,
        description: description ?? 'YYYY-MM-DD or RFC3339',
        type: 'string',
      }
    case 'URL':
      return { title, description, type: 'string', format: 'uri' }
    default:
      // Unknown future type — fall back to "anything goes" rather than
      // failing schema generation.
      return { title, description }
  }
}

/**
 * Compose the full JSON Schema for the import YAML using the resolved
 * workspace field definitions. When `fields` is empty (no workspace
 * field schema configured), the resulting schema accepts any custom
 * field key.
 */
export function buildImportJsonSchema(fields: ImportFieldDefinition[]): JSONSchema {
  const properties: Record<string, JSONSchema> = {}
  const requiredFields: string[] = []
  for (const fd of fields) {
    properties[fd.id] = buildFieldSchema(fd)
    if (fd.required) requiredFields.push(fd.id)
  }
  const fieldsSchema: JSONSchema =
    fields.length === 0
      ? {
          type: 'object',
          description:
            'No workspace fields are configured for this workspace. Any keys you provide here will be accepted as custom field values.',
          additionalProperties: true,
        }
      : {
          type: 'object',
          description:
            'Custom field values keyed by FieldDefinition.ID. Validated against this workspace field schema at preview time.',
          properties,
          ...(requiredFields.length > 0 ? { required: requiredFields } : {}),
          additionalProperties: false,
        }

  return {
    $schema: 'https://json-schema.org/draft/2020-12/schema',
    $id: 'https://hecatoncheires.dev/import-case.schema.json',
    title: 'Hecatoncheires Case Import',
    description:
      'YAML payload accepted by createCaseImport. Cases are persisted as DRAFT; no Slack channel is created.',
    type: 'object',
    required: ['version', 'cases'],
    additionalProperties: false,
    properties: {
      version: { const: 1, description: 'Schema version. Only 1 is supported.' },
      cases: {
        type: 'array',
        minItems: 1,
        items: {
          type: 'object',
          required: ['title'],
          additionalProperties: false,
          properties: {
            title: { type: 'string', minLength: 1 },
            description: { type: 'string' },
            isPrivate: { type: 'boolean', default: false },
            assigneeIDs: {
              type: 'array',
              items: { type: 'string', description: 'Slack user ID (e.g. U12345678)' },
            },
            fields: fieldsSchema,
            // NOTE: `actions:` blocks are NOT part of the Import
            // contract — imported cases are saved as DRAFT, which
            // cannot hold editable Actions. If the YAML carries an
            // `actions:` key it is silently dropped at preview time
            // and surfaced as a WARNING, but the schema itself does
            // not advertise the field.
          },
        },
      },
    },
  }
}

// A workspace-agnostic skeleton for the YAML example block. The fields
// section in the example is best left empty / illustrative — concrete
// IDs depend on the workspace, and showing a half-true sample would be
// misleading. Callers can override by building their own example string
// from the resolved field definitions if desired.
export const IMPORT_YAML_EXAMPLE = `version: 1
cases:
  - title: "Suspicious login"
    description: |
      Multiple failed attempts from 10.0.0.1
    isPrivate: false
    assigneeIDs: [U12345678]
    fields:
      # The keys under "fields" must match this workspace's
      # FieldDefinition.ID values. See the JSON Schema panel above
      # for the exact allowed keys, types, enums, and which are
      # required.
      severity: high
      source: aws-cloudtrail
# NOTE: Actions are NOT imported (DRAFT cases cannot hold editable
# Actions). Add Actions after submitting the draft from the Case
# detail page.
`

/**
 * Build a YAML example whose `fields` block lists every configured
 * workspace field. Required ones use a sentinel placeholder so the
 * user immediately sees what they must fill in; optional ones are
 * commented out so the example stays valid as-is.
 */
export function buildYamlExample(fields: ImportFieldDefinition[]): string {
  if (fields.length === 0) return IMPORT_YAML_EXAMPLE
  const lines: string[] = [
    'version: 1',
    'cases:',
    '  - title: "Suspicious login"',
    '    description: |',
    '      Multiple failed attempts from 10.0.0.1',
    '    isPrivate: false',
    '    assigneeIDs: [U12345678]',
    '    fields:',
  ]
  for (const fd of fields) {
    const placeholder = examplePlaceholder(fd)
    const note = fd.required ? '  # required' : '  # optional'
    const prefix = fd.required ? '      ' : '      # '
    lines.push(`${prefix}${fd.id}: ${placeholder}${note}`)
  }
  // Actions are intentionally omitted — DRAFT cases cannot hold
  // editable Actions, so Import drops any actions: block in the YAML.
  return lines.join('\n') + '\n'
}

function examplePlaceholder(fd: ImportFieldDefinition): string {
  switch (fd.type) {
    case 'TEXT':
      return '"..."'
    case 'NUMBER':
      return '0'
    case 'SELECT':
      return (fd.options?.[0]?.id ?? '"<option-id>"') as string
    case 'MULTI_SELECT': {
      const ids = (fd.options ?? []).slice(0, 2).map((o) => o.id)
      return ids.length > 0 ? `[${ids.join(', ')}]` : '[]'
    }
    case 'USER':
      return 'U12345678'
    case 'MULTI_USER':
      return '[U12345678]'
    case 'DATE':
      return '2026-06-30'
    case 'URL':
      return '"https://example.com"'
    default:
      return '"..."'
  }
}
