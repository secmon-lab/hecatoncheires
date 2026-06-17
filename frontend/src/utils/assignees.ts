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
