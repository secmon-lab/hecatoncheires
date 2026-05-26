import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'

import { I18nProvider } from '../i18n'
import BulkResultDialog from './BulkResultDialog'
import { DRAFT_ERROR_CODE } from '../graphql/draftErrorCodes'
import type { BulkActionResult } from '../hooks/useBulkDraftAction'

const renderDialog = (
  props: Partial<React.ComponentProps<typeof BulkResultDialog>> = {},
) => {
  const onClose = vi.fn()
  render(
    <I18nProvider>
      <BulkResultDialog
        open
        kind="submit"
        results={[]}
        onClose={onClose}
        {...props}
      />
    </I18nProvider>,
  )
  return { onClose }
}

describe('BulkResultDialog', () => {
  it('returns nothing when open=false', () => {
    renderDialog({ open: false })
    expect(screen.queryByTestId('bulk-result-dialog-body')).toBeNull()
  })

  it('shows the summary counts', () => {
    const results: BulkActionResult[] = [
      { id: 1, title: 'a', ok: true, errors: [] },
      { id: 2, title: 'b', ok: false, errors: [{ code: DRAFT_ERROR_CODE.INTERNAL, message: 'x' }] },
      { id: 3, title: 'c', ok: true, errors: [] },
    ]
    renderDialog({ results })
    const summary = screen.getByTestId('bulk-result-summary')
    expect(summary.textContent).toMatch(/2/)
    expect(summary.textContent).toMatch(/1/)
  })

  it('lists success rows', () => {
    const results: BulkActionResult[] = [
      { id: 11, title: 'alpha', ok: true, errors: [] },
      { id: 12, title: 'beta', ok: true, errors: [] },
    ]
    renderDialog({ results })
    expect(screen.getByTestId('bulk-result-success-row-11')).toHaveTextContent('alpha')
    expect(screen.getByTestId('bulk-result-success-row-12')).toHaveTextContent('beta')
  })

  it('renders MISSING_REQUIRED_FIELDS with the field-name list', () => {
    const results: BulkActionResult[] = [
      {
        id: 7,
        title: 'incomplete',
        ok: false,
        errors: [
          {
            code: DRAFT_ERROR_CODE.MISSING_REQUIRED_FIELDS,
            message: 'missing',
            missingFieldNames: ['Severity', 'Reporter'],
          },
        ],
      },
    ]
    renderDialog({ results })
    const reason = screen.getByTestId('bulk-result-failure-reason-7-0')
    expect(reason.textContent).toContain('Severity')
    expect(reason.textContent).toContain('Reporter')
  })

  it('renders INVALID_STATUS_TRANSITION with the current status if available', () => {
    const results: BulkActionResult[] = [
      {
        id: 8,
        title: 'race',
        ok: false,
        errors: [
          {
            code: DRAFT_ERROR_CODE.INVALID_STATUS_TRANSITION,
            message: 'x',
            currentStatus: 'OPEN',
          },
        ],
      },
    ]
    renderDialog({ results })
    expect(screen.getByTestId('bulk-result-failure-reason-8-0').textContent).toContain('OPEN')
  })

  it('renders ACTIVATION_FAILED with the activation-specific message', () => {
    const results: BulkActionResult[] = [
      {
        id: 9,
        title: 'slack-busted',
        ok: false,
        errors: [{ code: DRAFT_ERROR_CODE.ACTIVATION_FAILED, message: 'slack 500' }],
      },
    ]
    renderDialog({ results })
    const reason = screen.getByTestId('bulk-result-failure-reason-9-0')
    expect(reason.textContent).toBeTruthy()
  })

  it('renders multiple error reasons per failure row', () => {
    const results: BulkActionResult[] = [
      {
        id: 10,
        title: 'multi',
        ok: false,
        errors: [
          { code: DRAFT_ERROR_CODE.TITLE_REQUIRED, message: 'title' },
          { code: DRAFT_ERROR_CODE.ACTIVATION_FAILED, message: 'slack' },
        ],
      },
    ]
    renderDialog({ results })
    expect(screen.getByTestId('bulk-result-failure-reason-10-0')).toBeInTheDocument()
    expect(screen.getByTestId('bulk-result-failure-reason-10-1')).toBeInTheDocument()
  })

  it('uses the submit title key when kind=submit and delete key when kind=discard', () => {
    const { unmount } = render(
      <I18nProvider>
        <BulkResultDialog open kind="submit" results={[]} onClose={() => undefined} />
      </I18nProvider>,
    )
    const submitTitle = screen.getAllByRole('heading')[0].textContent
    unmount()

    render(
      <I18nProvider>
        <BulkResultDialog open kind="discard" results={[]} onClose={() => undefined} />
      </I18nProvider>,
    )
    const discardTitle = screen.getAllByRole('heading')[0].textContent

    expect(submitTitle).not.toBe(discardTitle)
  })

  it('fires onClose when the Close button is clicked', () => {
    const { onClose } = renderDialog()
    fireEvent.click(screen.getByTestId('bulk-result-close-button'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
