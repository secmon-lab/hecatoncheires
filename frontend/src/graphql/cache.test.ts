import { describe, it, expect } from 'vitest'
import { gql } from '@apollo/client'
import { createCache } from './cache'

const CASE_FRAGMENT = gql`
  fragment CacheTestCase on Case {
    id
    workspaceId
    title
  }
`

describe('createCache', () => {
  it('keys workspace-scoped entities by (workspaceId, id) so two workspaces sharing an id stay distinct', () => {
    const cache = createCache()
    for (const typename of ['Case', 'Action', 'CaseRef', 'CaseJob']) {
      const risk = cache.identify({ __typename: typename, id: 12, workspaceId: 'risk' })
      const support = cache.identify({ __typename: typename, id: 12, workspaceId: 'support' })
      expect(risk).toBeDefined()
      expect(risk).not.toEqual(support)
    }
  })

  it('does not normalize configuration value objects, whose id is only meaningful inside its document', () => {
    const cache = createCache()
    for (const typename of ['FieldDefinition', 'FieldOption', 'ActionStatusDefinition']) {
      expect(cache.identify({ __typename: typename, id: 'severity' })).toBeUndefined()
    }
  })

  it('still normalizes types whose id is globally unique', () => {
    const cache = createCache()
    expect(cache.identify({ __typename: 'SlackUser', id: 'U1' })).toEqual('SlackUser:U1')
    expect(cache.identify({ __typename: 'Memo', id: 'a5f6' })).toEqual('Memo:a5f6')
  })

  it('writes two same-id Cases from different workspaces without either overwriting the other', () => {
    const cache = createCache()
    const write = (workspaceId: string, title: string) =>
      cache.writeFragment({
        id: cache.identify({ __typename: 'Case', id: 12, workspaceId }),
        fragment: CASE_FRAGMENT,
        data: { __typename: 'Case', id: 12, workspaceId, title },
      })
    const read = (workspaceId: string) =>
      cache.readFragment<{ title: string }>({
        id: cache.identify({ __typename: 'Case', id: 12, workspaceId }),
        fragment: CASE_FRAGMENT,
      })

    write('risk', 'Risk twelve')
    write('support', 'Support twelve')

    expect(read('risk')?.title).toEqual('Risk twelve')
    expect(read('support')?.title).toEqual('Support twelve')
  })
})
