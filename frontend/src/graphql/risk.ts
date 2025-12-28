import { gql } from '@apollo/client'

export const GET_RISKS = gql`
  query GetRisks {
    risks {
      id
      name
      description
      createdAt
      updatedAt
    }
  }
`

export const GET_RISK = gql`
  query GetRisk($id: Int!) {
    risk(id: $id) {
      id
      name
      description
      createdAt
      updatedAt
    }
  }
`

export const CREATE_RISK = gql`
  mutation CreateRisk($input: CreateRiskInput!) {
    createRisk(input: $input) {
      id
      name
      description
      createdAt
      updatedAt
    }
  }
`

export const UPDATE_RISK = gql`
  mutation UpdateRisk($input: UpdateRiskInput!) {
    updateRisk(input: $input) {
      id
      name
      description
      createdAt
      updatedAt
    }
  }
`

export const DELETE_RISK = gql`
  mutation DeleteRisk($id: Int!) {
    deleteRisk(id: $id)
  }
`
