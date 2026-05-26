import { gql } from '@apollo/client'

// Fragment selecting every visible field on an ImportSession. Keeping it
// in one place means every Import-related Query / Mutation returns the
// same shape, simplifying the React-side typing.
export const IMPORT_SESSION_FIELDS = gql`
  fragment ImportSessionFields on ImportSession {
    id
    workspaceID
    creatorUserID
    status
    source {
      originalFileName
      sizeBytes
    }
    issues {
      path
      message
      severity
    }
    valid
    fieldSchemaHash
    createdAt
    updatedAt
    executedAt
    createdCount
    failedCount
    skippedCount
    snapshot {
      version
      cases {
        index
        title
        description
        isPrivate
        assigneeIDs
        assignees {
          id
          name
          realName
          imageUrl
        }
        fields {
          key
          display
        }
        issues {
          path
          message
          severity
        }
        result {
          status
          createdCaseID
          error {
            path
            message
            severity
          }
        }
        actions {
          index
          title
          description
          assigneeID
          dueDate
          issues {
            path
            message
            severity
          }
          result {
            status
            createdActionID
            error {
              path
              message
              severity
            }
          }
        }
      }
    }
  }
`

export const GET_IMPORT = gql`
  ${IMPORT_SESSION_FIELDS}
  query GetCaseImport($workspaceId: String!, $id: ID!) {
    caseImport(workspaceId: $workspaceId, id: $id) {
      ...ImportSessionFields
    }
  }
`

export const CREATE_CASE_IMPORT = gql`
  ${IMPORT_SESSION_FIELDS}
  mutation CreateCaseImport($workspaceId: String!, $input: CreateCaseImportInput!) {
    createCaseImport(workspaceId: $workspaceId, input: $input) {
      ...ImportSessionFields
    }
  }
`

export const EXECUTE_CASE_IMPORT = gql`
  ${IMPORT_SESSION_FIELDS}
  mutation ExecuteCaseImport($workspaceId: String!, $id: ID!) {
    executeCaseImport(workspaceId: $workspaceId, id: $id) {
      ...ImportSessionFields
    }
  }
`
