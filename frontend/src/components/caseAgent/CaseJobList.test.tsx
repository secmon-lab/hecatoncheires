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
  const onRun = vi.fn()
  render(
    <I18nProvider>
      <CaseJobList
        jobs={[triageJob, dailyJob]}
        loading={false}
        error={false}
        onRetry={onRetry}
        onRun={onRun}
        runningJobIds={new Set()}
        pendingJobId={null}
        {...props}
      />
    </I18nProvider>,
  )
  return { onRetry, onRun }
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

  it('runs the job whose Run button was pressed', () => {
    const { onRun } = renderList()
    fireEvent.click(screen.getByTestId('job-run-button-daily'))
    expect(onRun).toHaveBeenCalledTimes(1)
    expect(onRun).toHaveBeenCalledWith('daily')
  })

  it('does not expand the row when Run is pressed', () => {
    renderList()
    fireEvent.click(screen.getByTestId('job-run-button-triage'))
    expect(screen.queryByText('TRIAGE PROMPT BODY')).toBeNull()
  })

  it('disables Run for a job that is already running', () => {
    renderList({ runningJobIds: new Set(['triage']) })
    const running = screen.getByTestId('job-run-button-triage')
    expect(running).toBeDisabled()
    expect(running).toHaveTextContent('Running')
    // Other rows stay available.
    expect(screen.getByTestId('job-run-button-daily')).toBeEnabled()
  })

  it('disables Run while its trigger request is in flight', () => {
    renderList({ pendingJobId: 'daily' })
    const pending = screen.getByTestId('job-run-button-daily')
    expect(pending).toBeDisabled()
    expect(pending).toHaveTextContent('Starting')
    expect(screen.getByTestId('job-run-button-triage')).toBeEnabled()
  })

  it('renders an error state with a working retry button', () => {
    const { onRetry } = renderList({ jobs: [], error: true })
    expect(screen.queryByText('Initial triage')).toBeNull()
    const retry = screen.getByRole('button')
    fireEvent.click(retry)
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
