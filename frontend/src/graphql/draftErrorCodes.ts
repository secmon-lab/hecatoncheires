// draftErrorCodes mirrors the const blocks in pkg/controller/graphql/errors.go.
// Any new error code must be added in BOTH places — the Go side as
// gqlctrl.ErrCodeXxx, the TS side here. Use the DRAFT_ERROR_CODE object
// instead of bare string literals so renames stay greppable.

export const DRAFT_ERROR_CODE = {
  MISSING_REQUIRED_FIELDS: 'MISSING_REQUIRED_FIELDS',
  TITLE_REQUIRED: 'TITLE_REQUIRED',
  INVALID_STATUS_TRANSITION: 'INVALID_STATUS_TRANSITION',
  FIELD_VALIDATION_FAILED: 'FIELD_VALIDATION_FAILED',
  FORBIDDEN: 'FORBIDDEN',
  NOT_FOUND: 'NOT_FOUND',
  ACTIVATION_FAILED: 'ACTIVATION_FAILED',
  NETWORK: 'NETWORK',
  INTERNAL: 'INTERNAL',
} as const

export type DraftErrorCode = (typeof DRAFT_ERROR_CODE)[keyof typeof DRAFT_ERROR_CODE]

export const DRAFT_EXT_KEY = {
  CODE: 'code',
  MISSING_FIELD_NAMES: 'missingFieldNames',
  CURRENT_STATUS: 'currentStatus',
} as const

// BulkActionError is the per-failure record produced by the bulk-action
// hook. The discriminated union makes code-specific detail fields
// compile-checked: e.g. MISSING_REQUIRED_FIELDS always carries
// missingFieldNames so the renderer can list them.
export type BulkActionError =
  | { code: typeof DRAFT_ERROR_CODE.MISSING_REQUIRED_FIELDS; message: string; missingFieldNames: string[] }
  | { code: typeof DRAFT_ERROR_CODE.TITLE_REQUIRED; message: string }
  | { code: typeof DRAFT_ERROR_CODE.INVALID_STATUS_TRANSITION; message: string; currentStatus?: string }
  | { code: typeof DRAFT_ERROR_CODE.FIELD_VALIDATION_FAILED; message: string }
  | { code: typeof DRAFT_ERROR_CODE.FORBIDDEN; message: string }
  | { code: typeof DRAFT_ERROR_CODE.NOT_FOUND; message: string }
  | { code: typeof DRAFT_ERROR_CODE.ACTIVATION_FAILED; message: string }
  | { code: typeof DRAFT_ERROR_CODE.NETWORK; message: string }
  | { code: typeof DRAFT_ERROR_CODE.INTERNAL; message: string }
