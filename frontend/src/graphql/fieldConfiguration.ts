import { gql } from '@apollo/client'

export const GET_FIELD_CONFIGURATION = gql`
  query GetFieldConfiguration {
    fieldConfiguration {
      fields {
        id
        name
        type
        required
        description
        options {
          id
          name
          description
          color
          metadata
        }
      }
      labels {
        case
      }
    }
  }
`
