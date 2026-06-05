import { useQuery } from '@apollo/client'
import { useMemo } from 'react'
import { GET_CASE_STATUS_CONFIG } from '../graphql/caseStatus'
import type { ActionStatusDef } from './useActionStatuses'

export type CaseStatusConfig = {
  initial: string
  closed: string[]
  statuses: ActionStatusDef[]
}

// useCaseStatuses returns the workspace-scoped Case status configuration for
// thread-mode workspaces. `config` is null for channel-mode workspaces (the
// `caseStatusConfig` query returns null), which the Kanban uses to decide
// whether to render the Case board or the Action board. Helpers mirror
// useActionStatuses so the rendering code can be shared.
export function useCaseStatuses(workspaceId: string | undefined) {
  const { data, loading, error } = useQuery(GET_CASE_STATUS_CONFIG, {
    variables: { workspaceId: workspaceId ?? '' },
    skip: !workspaceId,
  })

  const config: CaseStatusConfig | null = useMemo(() => {
    const cc = data?.caseStatusConfig
    if (!cc || !cc.statuses?.length) return null
    return {
      initial: cc.initial,
      closed: cc.closed ?? [],
      statuses: cc.statuses,
    }
  }, [data])

  const byId = useMemo(() => {
    const m = new Map<string, ActionStatusDef>()
    for (const s of config?.statuses ?? []) m.set(s.id, s)
    return m
  }, [config])

  const closedSet = useMemo(() => new Set(config?.closed ?? []), [config])

  const label = (id: string): string => {
    const def = byId.get(id)
    if (!def) return id
    return def.name || def.id
  }

  const isClosed = (id: string): boolean => closedSet.has(id)

  return {
    loading,
    error,
    // isThreadMode is true when the workspace exposes a Case status set.
    isThreadMode: config != null,
    config,
    statuses: config?.statuses ?? [],
    initialId: config?.initial ?? '',
    label,
    isClosed,
    get: (id: string) => byId.get(id),
  }
}
