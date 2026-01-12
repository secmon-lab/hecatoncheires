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
  query GetSources {
    sources {
      ...SourceFields
    }
  }
`

export const GET_SOURCE = gql`
  ${SOURCE_FIELDS}
  query GetSource($id: String!) {
    source(id: $id) {
      ...SourceFields
    }
  }
`

export const CREATE_NOTION_DB_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateNotionDBSource($input: CreateNotionDBSourceInput!) {
    createNotionDBSource(input: $input) {
      ...SourceFields
    }
  }
`

export const UPDATE_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation UpdateSource($input: UpdateSourceInput!) {
    updateSource(input: $input) {
      ...SourceFields
    }
  }
`

export const DELETE_SOURCE = gql`
  mutation DeleteSource($id: String!) {
    deleteSource(id: $id)
  }
`

export const VALIDATE_NOTION_DB = gql`
  mutation ValidateNotionDB($databaseID: String!) {
    validateNotionDB(databaseID: $databaseID) {
      valid
      databaseTitle
      databaseURL
      errorMessage
    }
  }
`

export const CREATE_SLACK_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation CreateSlackSource($input: CreateSlackSourceInput!) {
    createSlackSource(input: $input) {
      ...SourceFields
    }
  }
`

export const UPDATE_SLACK_SOURCE = gql`
  ${SOURCE_FIELDS}
  mutation UpdateSlackSource($input: UpdateSlackSourceInput!) {
    updateSlackSource(input: $input) {
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
