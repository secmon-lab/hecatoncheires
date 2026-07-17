import { gql } from '@apollo/client'

// Shared selection sets for the Home page. Kept deliberately narrower than
// the fragments in graphql/case.ts and graphql/action.ts — Home only ever
// renders a handful of fields per row, so it does not reuse those larger
// fragments (which would pull in fields Home never displays).

const HOME_USER_FIELDS = gql`
  fragment HomeUserFields on SlackUser {
    id
    name
    realName
    imageUrl
  }
`

const HOME_CASE_FIELDS = gql`
  ${HOME_USER_FIELDS}
  fragment HomeCaseFields on Case {
    id
    title
    status
    assigneeIDs
    assignees {
      ...HomeUserFields
    }
    updatedAt
  }
`

const HOME_ACTION_FIELDS = gql`
  fragment HomeActionFields on Action {
    id
    title
    status
    dueDate
  }
`

export const GET_MY_OPEN_CASES = gql`
  ${HOME_CASE_FIELDS}
  query GetMyOpenCases {
    myOpenCases {
      workspaceId
      workspaceName
      stalled
      case {
        ...HomeCaseFields
      }
    }
  }
`

export const GET_MY_DUE_ACTIONS = gql`
  ${HOME_ACTION_FIELDS}
  query GetMyDueActions {
    myDueActions {
      workspaceId
      workspaceName
      caseId
      caseTitle
      action {
        ...HomeActionFields
      }
    }
  }
`

export const GET_FAVORITE_WORKSPACE_IDS = gql`
  query GetFavoriteWorkspaceIds {
    favoriteWorkspaceIds
  }
`

// Favorites are replaced wholesale rather than delta add/remove (unlike
// case assignees — see ASSIGN_CASE/UNASSIGN_CASE in graphql/case.ts) because
// the set is small (bounded by the user's workspace count) and always fully
// known client-side, so there is no concurrent-edit risk a delta API would
// need to guard against.
export const SET_FAVORITE_WORKSPACES = gql`
  mutation SetFavoriteWorkspaces($workspaceIds: [String!]!) {
    setFavoriteWorkspaces(workspaceIds: $workspaceIds)
  }
`

export const GET_HOME_MESSAGE = gql`
  query GetHomeMessage($clientTime: Time!, $lang: String!) {
    homeMessage(clientTime: $clientTime, lang: $lang) {
      message
    }
  }
`
