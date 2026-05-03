import { useQuery } from '@apollo/client'
import { useMemo } from 'react'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { useTranslation } from '../i18n'

export type ActionStatusDef = {
  id: string
  name: string
  nameJa?: string | null
  description?: string | null
  color?: string | null
  emoji?: string | null
}

export type ActionStatusConfig = {
  initial: string
  closed: string[]
  statuses: ActionStatusDef[]
}

const fallbackConfig: ActionStatusConfig = {
  initial: 'BACKLOG',
  closed: ['COMPLETED'],
  statuses: [
    { id: 'BACKLOG', name: 'Backlog', color: 'idle', emoji: '📋' },
    { id: 'TODO', name: 'To Do', color: 'idle', emoji: '📌' },
    { id: 'IN_PROGRESS', name: 'In Progress', color: 'active', emoji: '▶️' },
    { id: 'BLOCKED', name: 'Blocked', color: 'blocked', emoji: '🛑' },
    { id: 'COMPLETED', name: 'Completed', color: 'success', emoji: '✅' },
  ],
}

// useActionStatuses returns the workspace-scoped Action status configuration
// alongside helpers for the UI: localized label, color resolution, etc.
// While the query is loading, it returns the legacy default set so the UI
// can render synchronously without flashing empty columns.
export function useActionStatuses(workspaceId: string | undefined) {
  const { lang } = useTranslation()
  const { data, loading, error } = useQuery(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: workspaceId ?? '' },
    skip: !workspaceId,
  })

  const config: ActionStatusConfig = useMemo(() => {
    const ac = data?.fieldConfiguration?.actionConfig
    if (!ac || !ac.statuses?.length) return fallbackConfig
    return {
      initial: ac.initial,
      closed: ac.closed ?? [],
      statuses: ac.statuses,
    }
  }, [data])

  const byId = useMemo(() => {
    const m = new Map<string, ActionStatusDef>()
    for (const s of config.statuses) m.set(s.id, s)
    return m
  }, [config])

  const closedSet = useMemo(() => new Set(config.closed), [config])

  const label = (id: string): string => {
    const def = byId.get(id)
    if (!def) return id
    if (lang === 'ja' && def.nameJa) return def.nameJa
    return def.name || def.id
  }

  const isClosed = (id: string): boolean => closedSet.has(id)

  return {
    loading,
    error,
    config,
    statuses: config.statuses,
    initialId: config.initial,
    label,
    isClosed,
    get: (id: string) => byId.get(id),
  }
}
