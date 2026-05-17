import { describe, it, expect } from 'vitest'
import {
  reposFromSource,
  initialNotionDBValidation,
  initialNotionPageValidation,
} from './sourceFormHelpers'

describe('reposFromSource', () => {
  it('returns empty array when source is undefined', () => {
    expect(reposFromSource(undefined)).toEqual([])
  })

  it('returns empty array when config is missing', () => {
    expect(reposFromSource({ id: 'a', name: 'n', description: null })).toEqual([])
  })

  it('returns empty array when repositories is null', () => {
    expect(
      reposFromSource({
        id: 'a',
        name: 'n',
        description: null,
        config: { repositories: null },
      }),
    ).toEqual([])
  })

  it('maps owner/repo into fullName', () => {
    const got = reposFromSource({
      id: 'a',
      name: 'n',
      description: null,
      config: {
        repositories: [
          { owner: 'foo', repo: 'bar' },
          { owner: 'baz', repo: 'qux' },
        ],
      },
    })
    expect(got).toEqual([
      { owner: 'foo', repo: 'bar', fullName: 'foo/bar' },
      { owner: 'baz', repo: 'qux', fullName: 'baz/qux' },
    ])
  })
})

describe('initialNotionDBValidation', () => {
  it('returns null when source is undefined', () => {
    expect(initialNotionDBValidation(undefined)).toBeNull()
  })

  it('returns null when databaseID is empty', () => {
    expect(
      initialNotionDBValidation({
        id: 'a',
        name: 'n',
        description: null,
        config: { databaseID: '' },
      }),
    ).toBeNull()
  })

  it('returns a valid snapshot using existing title/URL', () => {
    const got = initialNotionDBValidation({
      id: 'a',
      name: 'n',
      description: null,
      config: {
        databaseID: 'db-1',
        databaseTitle: 'My DB',
        databaseURL: 'https://notion.so/db-1',
      },
    })
    expect(got).toEqual({
      valid: true,
      databaseTitle: 'My DB',
      databaseURL: 'https://notion.so/db-1',
      errorMessage: null,
    })
  })

  it('treats missing title/URL as null in the snapshot', () => {
    const got = initialNotionDBValidation({
      id: 'a',
      name: 'n',
      description: null,
      config: { databaseID: 'db-1' },
    })
    expect(got).toEqual({
      valid: true,
      databaseTitle: null,
      databaseURL: null,
      errorMessage: null,
    })
  })
})

describe('initialNotionPageValidation', () => {
  it('returns null when source is undefined', () => {
    expect(initialNotionPageValidation(undefined)).toBeNull()
  })

  it('returns null when pageID is empty', () => {
    expect(
      initialNotionPageValidation({
        id: 'a',
        name: 'n',
        description: null,
        config: { pageID: '' },
      }),
    ).toBeNull()
  })

  it('returns a valid snapshot using existing title/URL', () => {
    const got = initialNotionPageValidation({
      id: 'a',
      name: 'n',
      description: null,
      config: {
        pageID: 'p-1',
        pageTitle: 'My Page',
        pageURL: 'https://notion.so/p-1',
        recursive: true,
        maxDepth: 3,
      },
    })
    expect(got).toEqual({
      valid: true,
      pageTitle: 'My Page',
      pageURL: 'https://notion.so/p-1',
      errorMessage: null,
    })
  })
})
