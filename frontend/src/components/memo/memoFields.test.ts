import { describe, it, expect } from 'vitest'

import { normalizeMemoFields, type RawFieldDef } from './memoFields'

describe('normalizeMemoFields', () => {
  it('returns an empty array for null, undefined, and empty input', () => {
    expect(normalizeMemoFields(null)).toEqual([])
    expect(normalizeMemoFields(undefined)).toEqual([])
    expect(normalizeMemoFields([])).toEqual([])
  })

  it('preserves every field property and converts null description/options to undefined', () => {
    const raw: RawFieldDef[] = [
      {
        id: 'severity',
        name: 'Severity',
        type: 'SELECT',
        required: true,
        description: 'How bad it is',
        options: [
          { id: 'high', name: 'High', description: 'urgent', metadata: { color: 'red' } },
          { id: 'low', name: 'Low', description: null, metadata: null },
        ],
      },
      {
        id: 'body',
        name: 'Body',
        type: 'MARKDOWN',
        required: false,
        description: null,
        options: null,
      },
    ]

    const result = normalizeMemoFields(raw)

    expect(result).toEqual([
      {
        id: 'severity',
        name: 'Severity',
        type: 'SELECT',
        required: true,
        description: 'How bad it is',
        options: [
          { id: 'high', name: 'High', description: 'urgent', metadata: { color: 'red' } },
          { id: 'low', name: 'Low', description: undefined, metadata: undefined },
        ],
      },
      {
        id: 'body',
        name: 'Body',
        type: 'MARKDOWN',
        required: false,
        description: undefined,
        options: undefined,
      },
    ])
  })

  it('keeps an explicitly empty options array as an empty array (not undefined)', () => {
    const result = normalizeMemoFields([
      { id: 'f1', name: 'F1', type: 'TEXT', required: false, options: [] },
    ])
    expect(result[0].options).toEqual([])
  })
})
