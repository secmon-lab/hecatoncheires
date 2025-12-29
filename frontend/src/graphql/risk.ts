import { gql } from '@apollo/client'

// Fragment for full risk data
const RISK_FIELDS = gql`
  fragment RiskFields on Risk {
    id
    name
    description
    categoryIDs
    specificImpact
    likelihoodID
    impactID
    responseTeamIDs
    assigneeIDs
    detectionIndicators
    createdAt
    updatedAt
  }
`

export const GET_RISKS = gql`
  ${RISK_FIELDS}
  query GetRisks {
    risks {
      ...RiskFields
    }
  }
`

export const GET_RISK = gql`
  ${RISK_FIELDS}
  query GetRisk($id: Int!) {
    risk(id: $id) {
      ...RiskFields
      responses {
        id
        title
        status
        responders {
          id
          name
          realName
          imageUrl
        }
      }
    }
  }
`

export const GET_RISK_CONFIGURATION = gql`
  query GetRiskConfiguration {
    riskConfiguration {
      categories {
        id
        name
        description
      }
      likelihoodLevels {
        id
        name
        description
        score
      }
      impactLevels {
        id
        name
        description
        score
      }
      teams {
        id
        name
      }
    }
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

export const CREATE_RISK = gql`
  ${RISK_FIELDS}
  mutation CreateRisk($input: CreateRiskInput!) {
    createRisk(input: $input) {
      ...RiskFields
    }
  }
`

export const UPDATE_RISK = gql`
  ${RISK_FIELDS}
  mutation UpdateRisk($input: UpdateRiskInput!) {
    updateRisk(input: $input) {
      ...RiskFields
    }
  }
`

export const DELETE_RISK = gql`
  mutation DeleteRisk($id: Int!) {
    deleteRisk(id: $id)
  }
`
