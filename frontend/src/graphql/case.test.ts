import { describe, it, expect } from 'vitest'
import type { DocumentNode, FieldNode, FragmentDefinitionNode, SelectionSetNode } from 'graphql'
import { Kind } from 'graphql'
import {
  GET_CASES,
  GET_CASES_WITH_SLACK_LINK,
  GET_CASE,
  CREATE_CASE,
  UPDATE_CASE,
  CLOSE_CASE,
  REOPEN_CASE,
  ARCHIVE_CASE,
  UNARCHIVE_CASE,
  BULK_ARCHIVE_CASES,
  BULK_UNARCHIVE_CASES,
} from './case'

// The fragment refactor must not change any operation's effective field
// set: these tests flatten each document (resolving fragment spreads
// against the definitions carried in the same document) into sorted
// dotted leaf paths and compare them against the exact pre-refactor
// selection sets.

function collectLeafPaths(doc: DocumentNode): string[] {
  const fragments = new Map<string, FragmentDefinitionNode>()
  for (const def of doc.definitions) {
    if (def.kind === Kind.FRAGMENT_DEFINITION) {
      fragments.set(def.name.value, def)
    }
  }

  const operation = doc.definitions.find((def) => def.kind === Kind.OPERATION_DEFINITION)
  if (!operation || operation.kind !== Kind.OPERATION_DEFINITION) {
    throw new Error('document has no operation definition')
  }
  const rootField = operation.selectionSet.selections[0]
  if (!rootField || rootField.kind !== Kind.FIELD || !rootField.selectionSet) {
    throw new Error('operation root field has no selection set')
  }

  const paths: string[] = []
  const walk = (set: SelectionSetNode, prefix: string): void => {
    for (const selection of set.selections) {
      if (selection.kind === Kind.FIELD) {
        const field: FieldNode = selection
        const path = prefix === '' ? field.name.value : `${prefix}.${field.name.value}`
        if (field.selectionSet) {
          walk(field.selectionSet, path)
        } else {
          paths.push(path)
        }
      } else if (selection.kind === Kind.FRAGMENT_SPREAD) {
        const fragment = fragments.get(selection.name.value)
        if (!fragment) {
          throw new Error(`fragment ${selection.name.value} is not defined in the document`)
        }
        walk(fragment.selectionSet, prefix)
      } else if (selection.kind === Kind.INLINE_FRAGMENT) {
        walk(selection.selectionSet, prefix)
      }
    }
  }
  walk(rootField.selectionSet, '')
  return [...paths].sort()
}

const USER_PATHS = (prefix: string): string[] => [
  `${prefix}.id`,
  `${prefix}.name`,
  `${prefix}.realName`,
  `${prefix}.imageUrl`,
]

// The exact Case selection set case mutations returned before the
// fragment refactor.
const MUTATION_PATHS = [
  'id',
  // workspaceId is part of the Case cache key (see graphql/cache.ts), so every
  // Case selection set must carry it or Apollo silently stops normalizing.
  'workspaceId',
  'title',
  'description',
  'status',
  // archivedAt rides in the mutation set, not only the list set, so
  // archiveCase / unarchiveCase refresh the detail page's badge and menu from
  // their own response. There is deliberately no derived `archived` boolean.
  'archivedAt',
  'isPrivate',
  'isTest',
  'accessDenied',
  'reporterID',
  ...USER_PATHS('reporter'),
  'assigneeIDs',
  ...USER_PATHS('assignees'),
  'slackChannelID',
  'createdAt',
  'updatedAt',
  'fields.fieldId',
  'fields.value',
].sort()

const LIST_PATHS = [...MUTATION_PATHS, 'slackThreadTS', 'isThreadBound', 'boardStatus'].sort()

// The Case list page's own operation is the list set plus slackChannelURL.
// The extra field is deliberately kept out of LIST_PATHS — see the comment on
// GET_CASES_WITH_SLACK_LINK in case.ts.
const LIST_WITH_SLACK_LINK_PATHS = [...LIST_PATHS, 'slackChannelURL'].sort()

const DETAIL_PATHS = [
  ...LIST_PATHS,
  'channelUserCount',
  'slackChannelURL',
  'actions.id',
  'actions.workspaceId',
  'actions.title',
  'actions.status',
  'actions.assigneeID',
  ...USER_PATHS('actions.assignee'),
  'actions.dueDate',
  'actions.archived',
  'actions.archivedAt',
  'actions.createdAt',
  'actions.updatedAt',
].sort()

describe('case.ts selection sets', () => {
  it('all four case mutations return the identical historical field set', () => {
    expect(collectLeafPaths(CREATE_CASE)).toEqual(MUTATION_PATHS)
    expect(collectLeafPaths(UPDATE_CASE)).toEqual(MUTATION_PATHS)
    expect(collectLeafPaths(CLOSE_CASE)).toEqual(MUTATION_PATHS)
    expect(collectLeafPaths(REOPEN_CASE)).toEqual(MUTATION_PATHS)
  })

  it('GET_CASES returns the mutation set plus the list-only fields', () => {
    expect(collectLeafPaths(GET_CASES)).toEqual(LIST_PATHS)
  })

  it('GET_CASES does not carry slackChannelURL, so a broken Slack cannot break its consumers', () => {
    expect(collectLeafPaths(GET_CASES)).not.toContain('slackChannelURL')
  })

  it('GET_CASES_WITH_SLACK_LINK is GET_CASES plus slackChannelURL', () => {
    expect(collectLeafPaths(GET_CASES_WITH_SLACK_LINK)).toEqual(LIST_WITH_SLACK_LINK_PATHS)
  })

  it('GET_CASE returns the list set plus detail-only fields and actions', () => {
    expect(collectLeafPaths(GET_CASE)).toEqual(DETAIL_PATHS)
  })

  it('the single-case archive mutations return the same field set as the other case mutations', () => {
    expect(collectLeafPaths(ARCHIVE_CASE)).toEqual(MUTATION_PATHS)
    expect(collectLeafPaths(UNARCHIVE_CASE)).toEqual(MUTATION_PATHS)
  })

  it('every case operation carries archivedAt so the UI can tell archived from active', () => {
    for (const doc of [
      CREATE_CASE,
      UPDATE_CASE,
      CLOSE_CASE,
      REOPEN_CASE,
      ARCHIVE_CASE,
      UNARCHIVE_CASE,
      GET_CASES,
      GET_CASES_WITH_SLACK_LINK,
      GET_CASE,
    ]) {
      expect(collectLeafPaths(doc)).toContain('archivedAt')
    }
  })

  // The bulk mutations return [Int!]! — the ids ACCEPTED for the change, not
  // Case objects. Pinning the absence of a selection set keeps a future edit
  // from quietly turning them into object-returning mutations, which would
  // imply the change had already been applied when it is still asynchronous.
  it('the bulk archive mutations select a scalar id list, not Case objects', () => {
    for (const doc of [BULK_ARCHIVE_CASES, BULK_UNARCHIVE_CASES]) {
      const operation = doc.definitions.find((def) => def.kind === Kind.OPERATION_DEFINITION)
      expect(operation).toBeDefined()
      if (!operation || operation.kind !== Kind.OPERATION_DEFINITION) return
      const rootField = operation.selectionSet.selections[0]
      expect(rootField.kind).toBe(Kind.FIELD)
      if (rootField.kind !== Kind.FIELD) return
      expect(rootField.selectionSet).toBeUndefined()
    }
  })
})
