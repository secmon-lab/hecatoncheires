import { useTranslation, type MsgKey } from '../i18n'
import Button from './Button'
import Modal from './Modal'
import {
  DRAFT_ERROR_CODE,
  type BulkActionError,
} from '../graphql/draftErrorCodes'
import type { BulkActionResult, BulkActionKind } from '../hooks/useBulkDraftAction'

interface BulkResultDialogProps {
  open: boolean
  kind: BulkActionKind
  results: BulkActionResult[]
  onClose: () => void
}

// renderError converts a single BulkActionError into a human-readable
// sentence by picking the matching i18n key and interpolating code-specific
// detail. New codes must be added here AND in DRAFT_ERROR_CODE.
function useRenderError() {
  const { t } = useTranslation()
  return (e: BulkActionError): string => {
    switch (e.code) {
      case DRAFT_ERROR_CODE.MISSING_REQUIRED_FIELDS: {
        const fields = e.missingFieldNames.length > 0
          ? e.missingFieldNames.join(', ')
          : '—'
        return t('bulkErrorMissingRequired', { fields })
      }
      case DRAFT_ERROR_CODE.TITLE_REQUIRED:
        return t('bulkErrorTitleRequired')
      case DRAFT_ERROR_CODE.INVALID_STATUS_TRANSITION:
        return e.currentStatus
          ? t('bulkErrorInvalidStatusTransitionWithStatus', { status: e.currentStatus })
          : t('bulkErrorInvalidStatusTransition')
      case DRAFT_ERROR_CODE.FIELD_VALIDATION_FAILED:
        return t('bulkErrorFieldValidation', { message: e.message })
      case DRAFT_ERROR_CODE.FORBIDDEN:
        return t('bulkErrorForbidden')
      case DRAFT_ERROR_CODE.NOT_FOUND:
        return t('bulkErrorNotFound')
      case DRAFT_ERROR_CODE.ACTIVATION_FAILED:
        return t('bulkErrorActivationFailed')
      case DRAFT_ERROR_CODE.NETWORK:
        return t('bulkErrorNetwork', { message: e.message })
      case DRAFT_ERROR_CODE.INTERNAL:
      default:
        return t('bulkErrorGeneric', { message: e.message })
    }
  }
}

export default function BulkResultDialog({ open, kind, results, onClose }: BulkResultDialogProps) {
  const { t } = useTranslation()
  const renderError = useRenderError()

  const successes = results.filter((r) => r.ok)
  const failures = results.filter((r) => !r.ok)

  const titleKey: MsgKey = kind === 'submit' ? 'bulkResultTitleSubmit' : 'bulkResultTitleDelete'

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t(titleKey)}
      width={620}
      footer={
        <div className="row" style={{ justifyContent: 'flex-end' }}>
          <Button
            variant="primary"
            onClick={onClose}
            data-testid="bulk-result-close-button"
          >
            {t('bulkResultClose')}
          </Button>
        </div>
      }
    >
      <div data-testid="bulk-result-dialog-body">
        <p
          data-testid="bulk-result-summary"
          style={{ marginTop: 0, color: 'var(--text-body)', fontSize: 13 }}
        >
          {t('bulkResultSummary', {
            successCount: successes.length,
            failureCount: failures.length,
          })}
        </p>

        <section
          data-testid="bulk-result-failure-section"
          style={{ marginTop: 'var(--spacing-md)' }}
        >
          <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--text-heading)' }}>
            {t('bulkResultFailureSection', { count: failures.length })}
          </h3>
          {failures.length === 0 ? (
            <p style={{ margin: 'var(--spacing-xs) 0', color: 'var(--text-muted)', fontSize: 12 }}>
              {t('bulkResultEmptyFailure')}
            </p>
          ) : (
            <ul
              data-testid="bulk-result-failure-list"
              style={{
                margin: 'var(--spacing-sm) 0 0',
                padding: 0,
                listStyle: 'none',
                borderTop: '1px solid var(--border-default)',
              }}
            >
              {failures.map((r) => (
                <li
                  key={r.id}
                  data-testid={`bulk-result-failure-row-${r.id}`}
                  style={{
                    padding: 'var(--spacing-sm) 0',
                    borderBottom: '1px solid var(--border-default)',
                  }}
                >
                  <div
                    className="row"
                    style={{ gap: 'var(--spacing-sm)', fontSize: 13, alignItems: 'baseline' }}
                  >
                    <span className="mono soft" style={{ minWidth: 48 }}>#{r.id}</span>
                    <span style={{ fontWeight: 500, color: 'var(--text-heading)' }}>
                      {r.title || <span className="soft">—</span>}
                    </span>
                  </div>
                  <ul style={{ margin: 'var(--spacing-xs) 0 0 var(--spacing-lg)', padding: 0 }}>
                    {r.errors.map((err, idx) => (
                      <li
                        key={idx}
                        data-testid={`bulk-result-failure-reason-${r.id}-${idx}`}
                        style={{ fontSize: 12, color: 'var(--color-error)' }}
                      >
                        {renderError(err)}
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section
          data-testid="bulk-result-success-section"
          style={{ marginTop: 'var(--spacing-md)' }}
        >
          <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--text-heading)' }}>
            {t('bulkResultSuccessSection', { count: successes.length })}
          </h3>
          {successes.length === 0 ? (
            <p style={{ margin: 'var(--spacing-xs) 0', color: 'var(--text-muted)', fontSize: 12 }}>
              {t('bulkResultEmptySuccess')}
            </p>
          ) : (
            <ul
              data-testid="bulk-result-success-list"
              style={{
                margin: 'var(--spacing-sm) 0 0',
                padding: 0,
                listStyle: 'none',
              }}
            >
              {successes.map((r) => (
                <li
                  key={r.id}
                  data-testid={`bulk-result-success-row-${r.id}`}
                  style={{ fontSize: 12.5, padding: 'var(--spacing-xs) 0' }}
                >
                  <span className="mono soft" style={{ minWidth: 48, display: 'inline-block' }}>
                    #{r.id}
                  </span>{' '}
                  <span>{r.title || <span className="soft">—</span>}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </Modal>
  )
}
