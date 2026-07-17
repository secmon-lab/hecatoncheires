import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import WorkspaceChooser from './WorkspaceChooser'

const workspaces = [
  { id: 'risk', name: 'Risk', emoji: '🔥', color: null },
  { id: 'support', name: 'Support', emoji: null, color: '#2f6fed' },
]

function renderChooser(props: Partial<Parameters<typeof WorkspaceChooser>[0]> = {}) {
  const onToggleFavorite = props.onToggleFavorite ?? vi.fn()
  const utils = render(
    <MemoryRouter>
      <I18nProvider defaultLang="en">
        <WorkspaceChooser
          workspaces={props.workspaces ?? workspaces}
          isLoading={props.isLoading ?? false}
          favoriteWorkspaceIds={props.favoriteWorkspaceIds ?? []}
          onToggleFavorite={onToggleFavorite}
        />
      </I18nProvider>
    </MemoryRouter>,
  )
  return { ...utils, onToggleFavorite }
}

afterEach(() => cleanup())

describe('WorkspaceChooser', () => {
  it('shows a loading skeleton and nothing else while isLoading is true', () => {
    renderChooser({ isLoading: true })
    expect(screen.getByTestId('workspace-chooser-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('workspace-chooser')).not.toBeInTheDocument()
  })

  it('shows the empty state when there are no workspaces', () => {
    renderChooser({ workspaces: [], isLoading: false })
    expect(screen.getByTestId('workspace-chooser-empty')).toBeInTheDocument()
    expect(screen.getByText('No workspaces configured')).toBeInTheDocument()
  })

  it('renders each workspace as a card linking to its case list', () => {
    renderChooser()
    const riskCard = screen.getByTestId('workspace-card-risk')
    expect(riskCard).toHaveAttribute('href', '/ws/risk/cases')
    expect(screen.getByText('Risk')).toBeInTheDocument()
    expect(screen.getByText('Support')).toBeInTheDocument()
  })

  it('sorts favorited workspaces first, keeping relative order otherwise', () => {
    renderChooser({ favoriteWorkspaceIds: ['support'] })
    const cards = screen.getAllByTestId(/^workspace-card-/)
    expect(cards.map((c) => c.getAttribute('data-testid'))).toEqual([
      'workspace-card-support',
      'workspace-card-risk',
    ])
  })

  it('marks the favorite star active/inactive and labels it accessibly', () => {
    renderChooser({ favoriteWorkspaceIds: ['risk'] })
    const riskStar = screen.getByTestId('workspace-favorite-risk')
    const supportStar = screen.getByTestId('workspace-favorite-support')
    expect(riskStar).toHaveAttribute('aria-pressed', 'true')
    expect(riskStar).toHaveTextContent('★')
    expect(supportStar).toHaveAttribute('aria-pressed', 'false')
    expect(supportStar).toHaveTextContent('☆')
    expect(riskStar).toHaveAttribute('aria-label', 'Favorite')
  })

  it('toggles the favorite without navigating the card link', () => {
    const onToggleFavorite = vi.fn()
    renderChooser({ onToggleFavorite })
    const star = screen.getByTestId('workspace-favorite-risk')
    // fireEvent.click returns false when the event's preventDefault() was
    // called, which is how the star avoids triggering the parent card's
    // navigation.
    const notCancelled = fireEvent.click(star)
    expect(notCancelled).toBe(false)
    expect(onToggleFavorite).toHaveBeenCalledWith('risk')
    expect(onToggleFavorite).toHaveBeenCalledTimes(1)
  })
})
