import { describe, it, expect } from 'vitest'
import { buildImportJsonSchema, buildYamlExample, type ImportFieldDefinition } from './yamlSchema'

describe('buildImportJsonSchema', () => {
  it('expands workspace fields into properties / required / enums', () => {
    const fields: ImportFieldDefinition[] = [
      {
        id: 'severity',
        name: 'Severity',
        type: 'SELECT',
        required: true,
        options: [{ id: 'low' }, { id: 'medium' }, { id: 'high' }],
      },
      { id: 'source', name: 'Source', type: 'TEXT', required: false },
      { id: 'tags', name: 'Tags', type: 'MULTI_SELECT', required: false, options: [{ id: 'a' }, { id: 'b' }] },
      { id: 'dueDate', name: 'Due', type: 'DATE', required: true },
    ]
    const schema = buildImportJsonSchema(fields)

    const fieldsSchema = (schema as any).properties.cases.items.properties.fields
    expect(fieldsSchema.additionalProperties).toBe(false)
    expect(fieldsSchema.required).toEqual(['severity', 'dueDate'])
    expect(fieldsSchema.properties.severity.enum).toEqual(['low', 'medium', 'high'])
    expect(fieldsSchema.properties.severity.type).toBe('string')
    expect(fieldsSchema.properties.source.type).toBe('string')
    expect(fieldsSchema.properties.tags.type).toBe('array')
    expect(fieldsSchema.properties.tags.items.enum).toEqual(['a', 'b'])
    expect(fieldsSchema.properties.dueDate.type).toBe('string')
  })

  it('keeps additionalProperties: true when no workspace fields are configured', () => {
    const schema = buildImportJsonSchema([])
    const fieldsSchema = (schema as any).properties.cases.items.properties.fields
    expect(fieldsSchema.additionalProperties).toBe(true)
    expect(fieldsSchema.properties).toBeUndefined()
    expect(fieldsSchema.required).toBeUndefined()
  })

  it('does not emit a required[] array when no field is marked required', () => {
    const fields: ImportFieldDefinition[] = [
      { id: 'note', name: 'Note', type: 'TEXT', required: false },
    ]
    const schema = buildImportJsonSchema(fields)
    const fieldsSchema = (schema as any).properties.cases.items.properties.fields
    expect(fieldsSchema.required).toBeUndefined()
    expect(Object.keys(fieldsSchema.properties)).toEqual(['note'])
  })
})

describe('buildYamlExample', () => {
  it('lists every configured field, marking required ones uncommented', () => {
    const fields: ImportFieldDefinition[] = [
      {
        id: 'severity',
        name: 'Severity',
        type: 'SELECT',
        required: true,
        options: [{ id: 'low' }, { id: 'high' }],
      },
      { id: 'source', name: 'Source', type: 'TEXT', required: false },
    ]
    const example = buildYamlExample(fields)
    // required field: uncommented line ending with "# required"
    expect(example).toMatch(/^\s{6}severity: low\s+# required$/m)
    // optional field: commented (`#` after indent), ending with "# optional"
    expect(example).toMatch(/^\s{6}# source: "\.\.\."\s+# optional$/m)
  })
})
