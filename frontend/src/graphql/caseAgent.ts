import { gql } from '@apollo/client'

// GET_CASE_AGENT_SETTINGS pulls every field the CaseAgent page needs in
// a single round-trip: the prompt, the Source allowlist, and just enough
// Case metadata to render the page header.
export const GET_CASE_AGENT_SETTINGS = gql`
  query GetCaseAgentSettings($workspaceId: String!, $caseId: Int!) {
    case(workspaceId: $workspaceId, id: $caseId) {
      id
      title
      status
      isPrivate
      accessDenied
      slackChannelID
      slackChannelURL
      agentAdditionalPrompt
      agentSources {
        id
        name
        sourceType
        description
        enabled
      }
    }
    sources(workspaceId: $workspaceId) {
      id
      name
      sourceType
      description
      enabled
    }
  }
`

export const UPDATE_CASE_AGENT_SETTINGS = gql`
  mutation UpdateCaseAgentSettings(
    $workspaceId: String!
    $input: UpdateCaseAgentSettingsInput!
  ) {
    updateCaseAgentSettings(workspaceId: $workspaceId, input: $input) {
      id
      agentAdditionalPrompt
      agentSources {
        id
        name
        sourceType
        description
        enabled
      }
    }
  }
`

// GET_CASE_LATEST_JOB_RUN fetches just the most-recent run so the Case
// detail sidebar tile can show the "Last run · <stage> · <relative>"
// summary without pulling the full pagination payload.
export const GET_CASE_LATEST_JOB_RUN = gql`
  query GetCaseLatestJobRun($workspaceId: String!, $caseId: Int!) {
    caseJobRunLogs(workspaceId: $workspaceId, caseId: $caseId, first: 1) {
      items {
        runId
        stage
        startedAt
      }
    }
  }
`

export const GET_CASE_JOB_RUN_LOGS = gql`
  query GetCaseJobRunLogs(
    $workspaceId: String!
    $caseId: Int!
    $first: Int
    $after: String
  ) {
    caseJobRunLogs(
      workspaceId: $workspaceId
      caseId: $caseId
      first: $first
      after: $after
    ) {
      items {
        workspaceId
        caseId
        jobId
        jobName
        strategy
        runId
        traceId
        stage
        startedAt
        endedAt
        durationMs
        errorMessage
        eventType
        eventTriggerAt
      }
      nextCursor
    }
  }
`

export const GET_JOB_RUN_LOG = gql`
  query GetJobRunLog($workspaceId: String!, $caseId: Int!, $runId: String!) {
    jobRunLog(workspaceId: $workspaceId, caseId: $caseId, runId: $runId) {
      workspaceId
      caseId
      jobId
      jobName
      strategy
      runId
      traceId
      stage
      startedAt
      endedAt
      durationMs
      errorMessage
      systemPrompt
      eventType
      eventTriggerAt
    }
  }
`

export const GET_JOB_RUN_EVENTS = gql`
  query GetJobRunEvents($workspaceId: String!, $caseId: Int!, $runId: String!) {
    jobRunEvents(workspaceId: $workspaceId, caseId: $caseId, runId: $runId) {
      eventId
      runId
      sequence
      occurredAt
      kind
      parentSequence
      phase
      agentLabel
      payload
    }
  }
`
