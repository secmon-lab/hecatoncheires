import { gql } from '@apollo/client'

export const GET_WORKSPACES = gql`
  query GetWorkspaces {
    workspaces {
      id
      name
    }
  }
`
