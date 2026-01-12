// Source type constants
// These must match the GraphQL enum SourceType values
export const SOURCE_TYPE = {
  NOTION_DB: 'NOTION_DB',
  SLACK: 'Slack',
} as const

export type SourceType = (typeof SOURCE_TYPE)[keyof typeof SOURCE_TYPE]

// Form step constants for source creation flow
export const FORM_STEP = {
  CLOSED: 'closed',
  SELECT_TYPE: 'select-type',
  NOTION_DB_FORM: 'notion-db-form',
  SLACK_FORM: 'slack-form',
} as const

export type FormStep = (typeof FORM_STEP)[keyof typeof FORM_STEP]
