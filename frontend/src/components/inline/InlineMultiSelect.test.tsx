import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineMultiSelect from './InlineMultiSelect'

const opts = [
  { value: 'data_breach', label: 'Data Breach' },
  { value: 'system_failure', label: 'System Failure' },
  { value: 'compliance', label: 'Compliance' },
]

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineMultiSelect', () => {
  it('renders selected option labels in the trigger', () => {
    renderWithI18n(
      <InlineMultiSelect
        values={['compliance']}
        options={opts}
        onSave={vi.fn()}
        ariaLabel="category"
        testId="m"
      />,
    )
    expect(screen.getByTestId('m')).toHaveTextContent('Compliance')
  })

  it('adds an option to the existing list on toggle', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiSelect
        values={['compliance']}
        options={opts}
        onSave={onSave}
        ariaLabel="category"
        testId="m"
      />,
    )
    fireEvent.click(screen.getByTestId('m'))
    fireEvent.click(screen.getByTestId('m-option-data_breach'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(['compliance', 'data_breach']))
  })

  it('removes an option when toggled off', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiSelect
        values={['compliance', 'data_breach']}
        options={opts}
        onSave={onSave}
        ariaLabel="category"
        testId="m"
      />,
    )
    fireEvent.click(screen.getByTestId('m'))
    fireEvent.click(screen.getByTestId('m-option-compliance'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(['data_breach']))
  })

  it('prunes stale option IDs (no longer in options) on toggle', async () => {
    // case 2 in the field has a legacy "service-outage" id that is not present
    // in the current TOML. Adding a valid option must drop the stale id.
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiSelect
        values={['service-outage', 'compliance']}
        options={opts}
        onSave={onSave}
        ariaLabel="category"
        testId="m"
      />,
    )
    fireEvent.click(screen.getByTestId('m'))
    fireEvent.click(screen.getByTestId('m-option-data_breach'))
    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith(['compliance', 'data_breach']),
    )
  })

  it('prunes stale ids even when unselecting a valid option', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiSelect
        values={['service-outage', 'compliance']}
        options={opts}
        onSave={onSave}
        ariaLabel="category"
        testId="m"
      />,
    )
    fireEvent.click(screen.getByTestId('m'))
    fireEvent.click(screen.getByTestId('m-option-compliance'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith([]))
  })
})
