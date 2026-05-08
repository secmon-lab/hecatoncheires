import { gql } from '@apollo/client'

const ACTION_STEP_FIELDS = `
  id
  actionID
  title
  done
  doneAt
  createdAt
  updatedAt
`

export const GET_ACTION_STEPS = gql`
  query GetActionSteps($workspaceId: String!, $id: Int!) {
    action(workspaceId: $workspaceId, id: $id) {
      id
      steps {
        ${ACTION_STEP_FIELDS}
      }
      stepProgress {
        done
        total
      }
    }
  }
`

export const ADD_ACTION_STEP = gql`
  mutation AddActionStep($workspaceId: String!, $input: AddActionStepInput!) {
    addActionStep(workspaceId: $workspaceId, input: $input) {
      ${ACTION_STEP_FIELDS}
    }
  }
`

export const SET_ACTION_STEP_DONE = gql`
  mutation SetActionStepDone($workspaceId: String!, $input: SetActionStepDoneInput!) {
    setActionStepDone(workspaceId: $workspaceId, input: $input) {
      ${ACTION_STEP_FIELDS}
    }
  }
`

export const RENAME_ACTION_STEP = gql`
  mutation RenameActionStep($workspaceId: String!, $input: RenameActionStepInput!) {
    renameActionStep(workspaceId: $workspaceId, input: $input) {
      ${ACTION_STEP_FIELDS}
    }
  }
`

export const DELETE_ACTION_STEP = gql`
  mutation DeleteActionStep($workspaceId: String!, $input: DeleteActionStepInput!) {
    deleteActionStep(workspaceId: $workspaceId, input: $input)
  }
`
