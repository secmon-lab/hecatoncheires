// Pure helpers used by the source Form components to derive their initial
// values from an existing Source in edit mode. Pulling these out of the React
// components keeps them trivially unit-testable without an Apollo / Router /
// Workspace context boilerplate.

export interface SourceForEdit {
  id: string
  name: string
  description: string | null
}

export interface GitHubRepoConfig {
  owner: string
  repo: string
}

export interface ValidatedRepo extends GitHubRepoConfig {
  fullName: string
}

export interface GitHubSourceForEdit extends SourceForEdit {
  config?: { repositories?: GitHubRepoConfig[] | null } | null
}

export interface NotionDBSourceForEdit extends SourceForEdit {
  config?: {
    databaseID?: string | null
    databaseTitle?: string | null
    databaseURL?: string | null
  } | null
}

export interface NotionPageSourceForEdit extends SourceForEdit {
  config?: {
    pageID?: string | null
    pageTitle?: string | null
    pageURL?: string | null
    recursive?: boolean | null
    maxDepth?: number | null
  } | null
}

export interface NotionDBValidationSnapshot {
  valid: true
  databaseTitle: string | null
  databaseURL: string | null
  errorMessage: null
}

export interface NotionPageValidationSnapshot {
  valid: true
  pageTitle: string | null
  pageURL: string | null
  errorMessage: null
}

export function reposFromSource(source?: GitHubSourceForEdit): ValidatedRepo[] {
  const list = source?.config?.repositories ?? []
  return list.map((r) => ({ owner: r.owner, repo: r.repo, fullName: `${r.owner}/${r.repo}` }))
}

export function initialNotionDBValidation(
  source?: NotionDBSourceForEdit,
): NotionDBValidationSnapshot | null {
  if (!source?.config?.databaseID) return null
  return {
    valid: true,
    databaseTitle: source.config.databaseTitle ?? null,
    databaseURL: source.config.databaseURL ?? null,
    errorMessage: null,
  }
}

export function initialNotionPageValidation(
  source?: NotionPageSourceForEdit,
): NotionPageValidationSnapshot | null {
  if (!source?.config?.pageID) return null
  return {
    valid: true,
    pageTitle: source.config.pageTitle ?? null,
    pageURL: source.config.pageURL ?? null,
    errorMessage: null,
  }
}
