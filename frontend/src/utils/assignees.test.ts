import { describe, it, expect } from 'vitest'
import { diffAssignees } from './assignees'

describe('diffAssignees', () => {
  it('adds ids present in desired but not current', () => {
    expect(diffAssignees(['U1'], ['U1', 'U2', 'U3'])).toEqual({
      toAdd: ['U2', 'U3'],
      toRemove: [],
    })
  })

  it('removes ids present in current but not desired', () => {
    expect(diffAssignees(['U1', 'U2', 'U3'], ['U1'])).toEqual({
      toAdd: [],
      toRemove: ['U2', 'U3'],
    })
  })

  it('computes add and remove together', () => {
    expect(diffAssignees(['U1', 'U2'], ['U2', 'U3'])).toEqual({
      toAdd: ['U3'],
      toRemove: ['U1'],
    })
  })

  it('returns empty deltas when sets are equal regardless of order', () => {
    expect(diffAssignees(['U1', 'U2'], ['U2', 'U1'])).toEqual({
      toAdd: [],
      toRemove: [],
    })
  })

  it('treats clearing all as removals', () => {
    expect(diffAssignees(['U1', 'U2'], [])).toEqual({
      toAdd: [],
      toRemove: ['U1', 'U2'],
    })
  })

  it('ignores blank ids and collapses duplicates', () => {
    expect(diffAssignees(['', 'U1'], ['U1', 'U1', '', 'U2'])).toEqual({
      toAdd: ['U2'],
      toRemove: [],
    })
  })
})
