import { InMemoryCache } from '@apollo/client'

// Apollo's default normalization keys every object by `__typename:id`, which
// assumes `id` is globally unique. In this API it is not: Case and Action ids
// come from a per-workspace counter, and Job ids are workspace-unique config
// keys. Left at the default, `Case:12` from two workspaces becomes ONE cache
// entry and the last write wins — the Home dashboard rendered another
// workspace's title and assignees on rows whose workspace badge was correct,
// and returning to a workspace's list with cache-first showed the other
// workspace's data.
//
// Two shapes of fix, picked by whether the type has an identity at all:
//
//   * Entities whose id is scoped by workspace get the composite key
//     (workspaceId, id) — the same uniqueness the server guarantees. Every
//     query selecting one of these MUST also select workspaceId, or Apollo
//     cannot compute the key and silently falls back to no normalization.
//   * Configuration value objects have no identity to key on. Their `id` is a
//     config key that is only meaningful inside the enclosing configuration
//     document, and the same id is reused both across workspaces AND across
//     the two independent documents that carry them (FieldConfiguration.fields
//     vs MemoConfiguration.fields; fieldConfiguration.actionConfig vs
//     caseStatusConfig). No field combination identifies one, so they are not
//     normalized at all.
//
// Types absent from this map are left on Apollo's default `__typename:id`
// because their id genuinely is globally unique. ActionStep and ActionComment
// are the cases worth naming: both live under one Action, but their ids are
// server-generated UUIDs (the mutation inputs carry no id), so the default key
// cannot collide across Actions or workspaces.
export function createCache(): InMemoryCache {
  return new InMemoryCache({
    typePolicies: {
      Case: { keyFields: ['workspaceId', 'id'] },
      Action: { keyFields: ['workspaceId', 'id'] },
      CaseRef: { keyFields: ['workspaceId', 'id'] },
      CaseJob: { keyFields: ['workspaceId', 'id'] },

      FieldDefinition: { keyFields: false },
      FieldOption: { keyFields: false },
      ActionStatusDefinition: { keyFields: false },
    },
  })
}
