// Minimal JSON Schema TS shape covering the subset we generate in
// `yamlSchema.ts`. Kept intentionally permissive (no const enum on
// `type`) so the schema builder does not have to fight the type system
// for every constant — the canonical reference for what is valid lives
// in the JSON Schema spec, not in this file.
export interface JSONSchema {
  $schema?: string
  $id?: string
  title?: string
  description?: string
  type?: string | string[]
  required?: string[]
  additionalProperties?: boolean | JSONSchema
  properties?: Record<string, JSONSchema>
  items?: JSONSchema
  enum?: unknown[]
  const?: unknown
  default?: unknown
  minLength?: number
  minItems?: number
  format?: string
}
