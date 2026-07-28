import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MemoryRouter } from 'react-router'
import { I18nProvider } from '../i18n'
import WorkspaceSwitcher from './WorkspaceSwitcher'

function makeWorkspaces(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    id: `ws${i + 1}`,
    name: `Workspace ${i + 1}`,
    emoji: null,
    color: null,
  }))
}

function renderSwitcher(props: Partial<Parameters<typeof WorkspaceSwitcher>[0]> = {}) {
  const workspaces = props.workspaces ?? makeWorkspaces(3)
  return render(
    <MemoryRouter>
      <I18nProvider defaultLang="en">
        <WorkspaceSwitcher
          current={props.current ?? workspaces[0]}
          workspaces={workspaces}
          favoriteWorkspaceIds={props.favoriteWorkspaceIds ?? []}
        />
      </I18nProvider>
    </MemoryRouter>,
  )
}

function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: /Workspace 1/ }))
}

function itemIds(): string[] {
  return screen
    .getAllByTestId(/^workspace-switcher-item-/)
    .map((el) => el.getAttribute('data-testid')!.replace('workspace-switcher-item-', ''))
}

afterEach(() => cleanup())

describe('WorkspaceSwitcher', () => {
  it('lists favorited workspaces first and keeps the given order otherwise', () => {
    renderSwitcher({ favoriteWorkspaceIds: ['ws3'] })
    openMenu()
    expect(itemIds()).toEqual(['ws3', 'ws1', 'ws2'])
  })

  it('marks favorited rows with a star and leaves the others unmarked', () => {
    renderSwitcher({ favoriteWorkspaceIds: ['ws2'] })
    openMenu()
    const star = screen.getByTestId('workspace-switcher-star-ws2')
    expect(star).toHaveTextContent('★')
    expect(star).toHaveAttribute('aria-label', 'Favorite')
    expect(screen.queryByTestId('workspace-switcher-star-ws1')).not.toBeInTheDocument()
  })

  it('shows every workspace with no toggle when the list fits the limit', () => {
    renderSwitcher({ workspaces: makeWorkspaces(10) })
    openMenu()
    expect(itemIds()).toHaveLength(10)
    expect(screen.queryByTestId('workspace-switcher-more')).not.toBeInTheDocument()
  })

  it('collapses the overflow behind a toggle that reports how many are hidden', () => {
    renderSwitcher({ workspaces: makeWorkspaces(13) })
    openMenu()
    expect(itemIds()).toEqual([
      'ws1', 'ws2', 'ws3', 'ws4', 'ws5', 'ws6', 'ws7', 'ws8', 'ws9', 'ws10',
    ])
    const more = screen.getByTestId('workspace-switcher-more')
    expect(more).toHaveTextContent('Show 3 more')
    expect(more).toHaveAttribute('aria-expanded', 'false')
  })

  it('reveals the hidden workspaces when the toggle is clicked, and hides them again', () => {
    renderSwitcher({ workspaces: makeWorkspaces(13) })
    openMenu()

    fireEvent.click(screen.getByTestId('workspace-switcher-more'))
    expect(itemIds()).toHaveLength(13)
    const less = screen.getByTestId('workspace-switcher-more')
    expect(less).toHaveTextContent('Show less')
    expect(less).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(less)
    expect(itemIds()).toHaveLength(10)
  })

  it('keeps favorites beyond the limit visible while collapsing non-favorites', () => {
    // 12 favorites out of 15: every favorite stays on screen and all three
    // non-favorites go behind the toggle.
    const favorites = Array.from({ length: 12 }, (_, i) => `ws${i + 1}`)
    renderSwitcher({ workspaces: makeWorkspaces(15), favoriteWorkspaceIds: favorites })
    openMenu()

    expect(itemIds()).toEqual(favorites)
    expect(screen.getByTestId('workspace-switcher-more')).toHaveTextContent('Show 3 more')
  })

  it('fills the remaining slots with non-favorites up to the limit', () => {
    renderSwitcher({ workspaces: makeWorkspaces(13), favoriteWorkspaceIds: ['ws12', 'ws13'] })
    openMenu()

    expect(itemIds()).toEqual([
      'ws12', 'ws13', 'ws1', 'ws2', 'ws3', 'ws4', 'ws5', 'ws6', 'ws7', 'ws8',
    ])
    // 13 workspaces, 2 favorited: the 2 favorites plus 8 non-favorites fill the
    // 10 visible slots, leaving the remaining 3 non-favorites behind the toggle.
    expect(screen.getByTestId('workspace-switcher-more')).toHaveTextContent('Show 3 more')
  })

  it('reopens collapsed after the menu was closed while expanded', () => {
    renderSwitcher({ workspaces: makeWorkspaces(13) })
    openMenu()
    fireEvent.click(screen.getByTestId('workspace-switcher-more'))
    expect(itemIds()).toHaveLength(13)

    openMenu() // close
    expect(screen.queryByTestId('workspace-switcher-menu')).not.toBeInTheDocument()
    openMenu() // reopen
    expect(itemIds()).toHaveLength(10)
  })

  it('reports the popup state on the trigger button', () => {
    renderSwitcher()
    const trigger = screen.getByRole('button', { name: /Workspace 1/ })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    // The trigger names the element it controls so assistive tech can follow it.
    expect(trigger.getAttribute('aria-controls')).toBe(
      screen.getByTestId('workspace-switcher-menu').getAttribute('id'),
    )
  })

  it('closes on Escape and returns focus to the trigger', () => {
    renderSwitcher()
    const trigger = screen.getByRole('button', { name: /Workspace 1/ })
    openMenu()

    fireEvent.keyDown(screen.getByTestId('workspace-switcher-menu'), { key: 'Escape' })
    expect(screen.queryByTestId('workspace-switcher-menu')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  it('renders a plain label without a menu when only one workspace exists', () => {
    const workspaces = makeWorkspaces(1)
    renderSwitcher({ workspaces })
    fireEvent.click(screen.getByRole('button', { name: /Workspace 1/ }))
    expect(screen.queryByTestId('workspace-switcher-menu')).not.toBeInTheDocument()
  })
})
