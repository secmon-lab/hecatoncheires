import { describe, it, expect } from 'vitest'
import { diffAssignees, orderAssigneeCandidates } from './assignees'

const user = (id: string, realName = '', name = '') => ({ id, realName, name })

describe('orderAssigneeCandidates', () => {
  it('puts ranked users first, in ranking order', () => {
    const users = [user('U1', 'Alice'), user('U2', 'Bob'), user('U3', 'Carol')]
    const ordered = orderAssigneeCandidates(users, ['U3', 'U1'])
    expect(ordered.map((u) => u.id)).toEqual(['U3', 'U1', 'U2'])
  })

  it('orders unranked users by display name, ignoring case', () => {
    const users = [user('U1', 'charlie'), user('U2', 'Alice'), user('U3', 'bob')]
    const ordered = orderAssigneeCandidates(users, [])
    expect(ordered.map((u) => u.id)).toEqual(['U2', 'U3', 'U1'])
  })

  it('falls back to the handle when no display name is set', () => {
    const users = [user('U1', '', 'zoe'), user('U2', '', 'adam')]
    const ordered = orderAssigneeCandidates(users, [])
    expect(ordered.map((u) => u.id)).toEqual(['U2', 'U1'])
  })

  it('breaks display-name ties on the id', () => {
    const users = [user('U9', 'Same Name'), user('U2', 'Same Name')]
    const ordered = orderAssigneeCandidates(users, [])
    expect(ordered.map((u) => u.id)).toEqual(['U2', 'U9'])
  })

  it('drops ranked ids that match no user', () => {
    const users = [user('U1', 'Alice'), user('U2', 'Bob')]
    const ordered = orderAssigneeCandidates(users, ['U-gone', 'U2'])
    expect(ordered.map((u) => u.id)).toEqual(['U2', 'U1'])
  })

  it('ignores blank and duplicated ranked ids', () => {
    const users = [user('U1', 'Alice'), user('U2', 'Bob'), user('U3', 'Carol')]
    const ordered = orderAssigneeCandidates(users, ['', 'U3', 'U3', 'U2'])
    expect(ordered.map((u) => u.id)).toEqual(['U3', 'U2', 'U1'])
  })

  it('returns an empty array for no users', () => {
    expect(orderAssigneeCandidates([], ['U1'])).toEqual([])
  })

  it('does not mutate the input array', () => {
    const users = [user('U1', 'Charlie'), user('U2', 'Alice')]
    orderAssigneeCandidates(users, ['U1'])
    expect(users.map((u) => u.id)).toEqual(['U1', 'U2'])
  })
})

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
