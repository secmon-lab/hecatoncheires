import { describe, it, expect } from 'vitest'
import type { DocumentNode, FieldNode, FragmentDefinitionNode, SelectionSetNode } from 'graphql'
import { Kind } from 'graphql'
import { GET_CASES, GET_CASE, CREATE_CASE, UPDATE_CASE, CLOSE_CASE, REOPEN_CASE } from './case'

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
  'title',
  'description',
  'status',
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

const DETAIL_PATHS = [
  ...LIST_PATHS,
  'channelUserCount',
  'slackChannelURL',
  'actions.id',
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

  it('GET_CASE returns the list set plus detail-only fields and actions', () => {
    expect(collectLeafPaths(GET_CASE)).toEqual(DETAIL_PATHS)
  })
})
