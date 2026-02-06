import { gql } from '@apollo/client'

export const GET_SLACK_USERS = gql`
  query GetSlackUsers {
    slackUsers {
      id
      name
      realName
      imageUrl
    }
  }
`
