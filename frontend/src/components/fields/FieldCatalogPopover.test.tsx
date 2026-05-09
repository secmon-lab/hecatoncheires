import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import FieldCatalogPopover, { type FieldDefinitionForHelp } from './FieldCatalogPopover'

function renderWith(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

const SELECT_FIELD: FieldDefinitionForHelp = {
  id: 'severity',
  name: 'Severity',
  type: 'SELECT',
  description: 'Incident severity scoring (P1=highest).',
  options: [
    { id: 'p1', name: 'P1', description: 'Production-impacting, immediate response.' },
    { id: 'p2', name: 'P2', description: 'Partial degradation, 24h.' },
    { id: 'p3', name: 'P3', description: 'Minor, next sprint.' },
  ],
}

const MULTI_FIELD: FieldDefinitionForHelp = {
  id: 'systems',
  name: 'Affected systems',
  type: 'MULTI_SELECT',
  options: [
    { id: 'db', name: 'users-db', description: 'Production user DB.' },
    { id: 'api', name: 'api-gateway', description: 'Public edge.' },
    { id: 'auth', name: 'auth-svc', description: 'Authentication service.' },
  ],
}

const TEXT_FIELD: FieldDefinitionForHelp = {
  id: 'impact',
  name: 'Impact',
  type: 'TEXT',
  description: 'Brief impact summary.',
}

function makeAnchor(): HTMLElement {
  const el = document.createElement('div')
  document.body.appendChild(el)
  return el
}

describe('FieldCatalogPopover', () => {
  it('does not render when closed', () => {
    renderWith(
      <FieldCatalogPopover
        field={SELECT_FIELD}
        value="p1"
        anchor={makeAnchor()}
        open={false}
        onClose={vi.fn()}
        testId="cat"
      />,
    )
    expect(screen.queryByTestId('cat')).toBeNull()
  })

  it('renders field description when open', () => {
    renderWith(
      <FieldCatalogPopover
        field={SELECT_FIELD}
        value="p1"
        anchor={makeAnchor()}
        open={true}
        onClose={vi.fn()}
        testId="cat"
      />,
    )
    expect(screen.getByTestId('cat-desc')).toHaveTextContent('Incident severity scoring')
  })

  it('renders all SELECT options and marks the current value as Selected', () => {
    renderWith(
      <FieldCatalogPopover
        field={SELECT_FIELD}
        value="p1"
        anchor={makeAnchor()}
        open={true}
        onClose={vi.fn()}
        testId="cat"
      />,
    )
    expect(screen.getByTestId('cat-option-p1')).toBeInTheDocument()
    expect(screen.getByTestId('cat-option-p2')).toBeInTheDocument()
    expect(screen.getByTestId('cat-option-p3')).toBeInTheDocument()
    // P1 has the Selected badge, others do not.
    expect(screen.getByTestId('cat-option-p1')).toHaveTextContent('Selected')
    expect(screen.getByTestId('cat-option-p2')).not.toHaveTextContent('Selected')
  })

  it('marks every selected MULTI_SELECT option', () => {
    renderWith(
      <FieldCatalogPopover
        field={MULTI_FIELD}
        value={['db', 'auth']}
        anchor={makeAnchor()}
        open={true}
        onClose={vi.fn()}
        testId="cat"
      />,
    )
    expect(screen.getByTestId('cat-option-db')).toHaveTextContent('Selected')
    expect(screen.getByTestId('cat-option-auth')).toHaveTextContent('Selected')
    expect(screen.getByTestId('cat-option-api')).not.toHaveTextContent('Selected')
  })

  it('omits the option list for non-select field types', () => {
    renderWith(
      <FieldCatalogPopover
        field={TEXT_FIELD}
        value="anything"
        anchor={makeAnchor()}
        open={true}
        onClose={vi.fn()}
        testId="cat"
      />,
    )
    expect(screen.getByTestId('cat-desc')).toBeInTheDocument()
    expect(screen.queryByTestId('cat-option-p1')).toBeNull()
  })

  it('calls onClose when the close button is clicked', () => {
    const onClose = vi.fn()
    renderWith(
      <FieldCatalogPopover
        field={SELECT_FIELD}
        value="p1"
        anchor={makeAnchor()}
        open={true}
        onClose={onClose}
        testId="cat"
      />,
    )
    fireEvent.click(screen.getByTestId('cat-close'))
    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose on Escape', () => {
    const onClose = vi.fn()
    renderWith(
      <FieldCatalogPopover
        field={SELECT_FIELD}
        value="p1"
        anchor={makeAnchor()}
        open={true}
        onClose={onClose}
        testId="cat"
      />,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalled()
  })
})
