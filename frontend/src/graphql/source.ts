import { gql } from '@apollo/client'

// Fragment for Notion DB config
const NOTION_DB_CONFIG_FIELDS = gql`
  fragment NotionDBConfigFields on NotionDBConfig {
    databaseID
    databaseTitle
    databaseURL
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

// Fragment for full source data
const SOURCE_FIELDS = gql`
  ${NOTION_DB_CONFIG_FIELDS}
  ${SLACK_CONFIG_FIELDS}
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
      ... on SlackConfig {
        ...SlackConfigFields
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
