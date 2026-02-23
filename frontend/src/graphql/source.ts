import { gql } from '@apollo/client'

// Fragment for Notion DB config
const NOTION_DB_CONFIG_FIELDS = gql`
  fragment NotionDBConfigFields on NotionDBConfig {
    databaseID
    databaseTitle
    databaseURL
  }
`

// Fragment for Notion Page config
const NOTION_PAGE_CONFIG_FIELDS = gql`
  fragment NotionPageConfigFields on NotionPageConfig {
    pageID
    pageTitle
    pageURL
    recursive
    maxDepth
  }
`

// Fragment for Slack config
const SLACK_CONFIG_FIELDS = gql`
  fragment SlackConfigFields on SlackConfig {
    channels {
      id
      name
    }
  }
`

// Fragment for GitHub config
const GITHUB_CONFIG_FIELDS = gql`
  fragment GitHubConfigFields on GitHubConfig {
    repositories {
      owner
      repo
    }
  }
`

// Fragment for full source data
const SOURCE_FIELDS = gql`
  ${NOTION_DB_CONFIG_FIELDS}
  ${NOTION_PAGE_CONFIG_FIELDS}
  ${SLACK_CONFIG_FIELDS}
  ${GITHUB_CONFIG_FIELDS}
  fragment SourceFields on Source {
    id
    name
    sourceType
    description
    enabled
    config {
      ... on NotionDBConfig {
        ...NotionDBConfigFields
      }
      ... on NotionPageConfig {
        ...NotionPageConfigFields
      }
      ... on SlackConfig {
        ...SlackConfigFields
      }
      ... on GitHubConfig {
        ...GitHubConfigFields
      }
    }
    createdAt
    updatedAt
  }
`

export const GET_SOURCES = gql`
  ${SOURCE_FIELDS}
  query GetSources($workspaceId: String!) {
    sources(workspaceId: $workspaceId) {
      ...SourceFields
    }
  }
`

export const GET_SOURCE = gql`
  ${SOURCE_FIELDS}
  query GetSource($workspaceId: String!, $id: String!) {
    source(workspaceId: $workspaceId, id: $id) {
      ...SourceFields
    }
  }
`

export const CREATE_NOTION_DB_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateNotionDBSource($workspaceId: String!, $input: CreateNotionDBSourceInput!) {
    createNotionDBSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const CREATE_NOTION_PAGE_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateNotionPageSource($workspaceId: String!, $input: CreateNotionPageSourceInput!) {
    createNotionPageSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const VALIDATE_NOTION_PAGE = gql`
  mutation ValidateNotionPage($workspaceId: String!, $pageID: String!) {
    validateNotionPage(workspaceId: $workspaceId, pageID: $pageID) {
      valid
      pageTitle
      pageURL
      errorMessage
    }
  }
`

export const UPDATE_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation UpdateSource($workspaceId: String!, $input: UpdateSourceInput!) {
    updateSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const DELETE_SOURCE = gql`
  mutation DeleteSource($workspaceId: String!, $id: String!) {
    deleteSource(workspaceId: $workspaceId, id: $id)
  }
`

export const VALIDATE_NOTION_DB = gql`
  mutation ValidateNotionDB($workspaceId: String!, $databaseID: String!) {
    validateNotionDB(workspaceId: $workspaceId, databaseID: $databaseID) {
      valid
      databaseTitle
      databaseURL
      errorMessage
    }
  }
`

export const CREATE_SLACK_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateSlackSource($workspaceId: String!, $input: CreateSlackSourceInput!) {
    createSlackSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const UPDATE_SLACK_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation UpdateSlackSource($workspaceId: String!, $input: UpdateSlackSourceInput!) {
    updateSlackSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const GET_SLACK_JOINED_CHANNELS = gql`
  query GetSlackJoinedChannels {
    slackJoinedChannels {
      id
      name
    }
  }
`

export const UPDATE_GITHUB_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation UpdateGitHubSource($workspaceId: String!, $input: UpdateGitHubSourceInput!) {
    updateGitHubSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const CREATE_GITHUB_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateGitHubSource($workspaceId: String!, $input: CreateGitHubSourceInput!) {
    createGitHubSource(workspaceId: $workspaceId, input: $input) {
      ...SourceFields
    }
  }
`

export const VALIDATE_GITHUB_REPO = gql`
  query ValidateGitHubRepo($workspaceId: String!, $repository: String!) {
    validateGitHubRepo(workspaceId: $workspaceId, repository: $repository) {
      valid
      owner
      repo
      errorMessage
    }
  }
`
