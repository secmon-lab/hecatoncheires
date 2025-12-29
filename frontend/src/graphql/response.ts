import { gql } from '@apollo/client'

// Fragment for full response data
const RESPONSE_FIELDS = gql`
  fragment ResponseFields on Response {
    id
    title
    description
    responders {
      id
      name
      realName
      imageUrl
    }
    url
    status
    createdAt
    updatedAt
  }
`

export const GET_RESPONSES = gql`
  ${RESPONSE_FIELDS}
  query GetResponses {
    responses {
      ...ResponseFields
    }
  }
`

export const GET_RESPONSE = gql`
  ${RESPONSE_FIELDS}
  query GetResponse($id: Int!) {
    response(id: $id) {
      ...ResponseFields
      risks {
        id
        name
        description
      }
    }
  }
`

export const GET_RESPONSES_BY_RISK = gql`
  ${RESPONSE_FIELDS}
  query GetResponsesByRisk($riskID: Int!) {
    responsesByRisk(riskID: $riskID) {
      ...ResponseFields
    }
  }
`

export const CREATE_RESPONSE = gql`
  ${RESPONSE_FIELDS}
  mutation CreateResponse($input: CreateResponseInput!) {
    createResponse(input: $input) {
      ...ResponseFields
    }
  }
`

export const UPDATE_RESPONSE = gql`
  ${RESPONSE_FIELDS}
  mutation UpdateResponse($input: UpdateResponseInput!) {
    updateResponse(input: $input) {
      ...ResponseFields
    }
  }
`

export const DELETE_RESPONSE = gql`
  mutation DeleteResponse($id: Int!) {
    deleteResponse(id: $id)
  }
`

export const LINK_RESPONSE_TO_RISK = gql`
  mutation LinkResponseToRisk($responseID: Int!, $riskID: Int!) {
    linkResponseToRisk(responseID: $responseID, riskID: $riskID)
  }
`

export const UNLINK_RESPONSE_FROM_RISK = gql`
  mutation UnlinkResponseFromRisk($responseID: Int!, $riskID: Int!) {
    unlinkResponseFromRisk(responseID: $responseID, riskID: $riskID)
  }
`

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
