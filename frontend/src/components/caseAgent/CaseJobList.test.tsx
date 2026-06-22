import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'

import { I18nProvider } from '../../i18n'
import CaseJobList, { type CaseJob } from './CaseJobList'

const triageJob: CaseJob = {
  id: 'triage',
  name: 'Initial triage',
  description: 'evaluate on create',
  strategy: 'PLANEXEC',
  quiet: false,
  prompt: 'TRIAGE PROMPT BODY',
  trigger: { caseEvents: ['CREATED'], schedule: null },
}

const dailyJob: CaseJob = {
  id: 'daily',
  name: 'Daily summary',
  description: 'report',
  strategy: 'SIMPLE',
  quiet: true,
  prompt: 'DAILY PROMPT BODY',
  trigger: { caseEvents: [], schedule: { everySeconds: null, cron: '0 9 * * *' } },
}

const renderList = (props: Partial<React.ComponentProps<typeof CaseJobList>> = {}) => {
  const onRetry = vi.fn()
  render(
    <I18nProvider>
      <CaseJobList
        jobs={[triageJob, dailyJob]}
        loading={false}
        error={false}
        onRetry={onRetry}
        {...props}
      />
    </I18nProvider>,
  )
  return { onRetry }
}

describe('CaseJobList', () => {
  it('renders each job name and description', () => {
    renderList()
    expect(screen.getByText('Initial triage')).toBeInTheDocument()
    expect(screen.getByText('evaluate on create')).toBeInTheDocument()
    expect(screen.getByText('Daily summary')).toBeInTheDocument()
  })

  it('surfaces a cron expression as visible text', () => {
    renderList()
    expect(screen.getByText('0 9 * * *')).toBeInTheDocument()
  })

  it('hides the prompt until a row is expanded, then toggles it', () => {
    renderList()
    expect(screen.queryByText('TRIAGE PROMPT BODY')).toBeNull()

    const row = screen.getByRole('button', { name: /Initial triage/ })
    fireEvent.click(row)
    expect(screen.getByText('TRIAGE PROMPT BODY')).toBeInTheDocument()
    expect(row).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(row)
    expect(screen.queryByText('TRIAGE PROMPT BODY')).toBeNull()
  })

  it('allows multiple rows open at once', () => {
    renderList()
    fireEvent.click(screen.getByRole('button', { name: /Initial triage/ }))
    fireEvent.click(screen.getByRole('button', { name: /Daily summary/ }))
    expect(screen.getByText('TRIAGE PROMPT BODY')).toBeInTheDocument()
    expect(screen.getByText('DAILY PROMPT BODY')).toBeInTheDocument()
  })

  it('renders the empty state when there are no jobs', () => {
    renderList({ jobs: [] })
    // Prompts of the sample jobs must be absent.
    expect(screen.queryByText('TRIAGE PROMPT BODY')).toBeNull()
    expect(screen.queryByText('Initial triage')).toBeNull()
  })

  it('renders an error state with a working retry button', () => {
    const { onRetry } = renderList({ jobs: [], error: true })
    expect(screen.queryByText('Initial triage')).toBeNull()
    const retry = screen.getByRole('button')
    fireEvent.click(retry)
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
