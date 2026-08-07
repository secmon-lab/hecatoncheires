import { displayName } from './user'

export interface OrderableUser {
  id: string
  name?: string | null
  realName?: string | null
}

// orderAssigneeCandidates puts the people who actually take work in this
// workspace at the top of an assignee picker. `frequentIDs` is the server's
// ranking (most assigned first, see the frequentAssigneeIDs query); everyone it
// does not name follows in display-name order, which is the useful default the
// picker never had — the raw slackUsers list arrives in Slack user ID order.
//
// Ranking ids with no matching user are dropped (someone removed from Slack
// lingers in the cached ranking until it is recomputed). Returns a new array;
// the input is never mutated.
export function orderAssigneeCandidates<T extends OrderableUser>(
  users: readonly T[],
  frequentIDs: readonly string[],
): T[] {
  const rank = new Map<string, number>()
  frequentIDs.forEach((id, index) => {
    if (id && !rank.has(id)) rank.set(id, index)
  })

  const ranked: T[] = []
  const rest: T[] = []
  for (const user of users) {
    if (rank.has(user.id)) ranked.push(user)
    else rest.push(user)
  }

  ranked.sort((a, b) => rank.get(a.id)! - rank.get(b.id)!)
  rest.sort((a, b) => {
    const byName = displayName(a).localeCompare(displayName(b), undefined, { sensitivity: 'base' })
    // Fall back to the id so users sharing a display name keep a stable order.
    return byName !== 0 ? byName : a.id.localeCompare(b.id)
  })

  return [...ranked, ...rest]
}

// diffAssignees computes the delta needed to turn a current assignee set into
// a desired one, so a full-selection UI (e.g. a multi-user picker) can persist
// the change through the delta assignCase / unassignCase mutations rather than
// a full-list replace. `toAdd` are ids in `desired` but not `current`;
// `toRemove` are ids in `current` but not `desired`. Blank ids are ignored and
// duplicates are collapsed.
export function diffAssignees(
  current: readonly string[],
  desired: readonly string[],
): { toAdd: string[]; toRemove: string[] } {
  const currentSet = new Set(current.filter((id) => id !== ''))
  const desiredSet = new Set(desired.filter((id) => id !== ''))

  const toAdd: string[] = []
  for (const id of desiredSet) {
    if (!currentSet.has(id)) toAdd.push(id)
  }

  const toRemove: string[] = []
  for (const id of currentSet) {
    if (!desiredSet.has(id)) toRemove.push(id)
  }

  return { toAdd, toRemove }
}
