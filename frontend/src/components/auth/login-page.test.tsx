import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import { LoginPage } from './login-page'

// jsdom's window.location is read-only by default and assigning to
// window.location.href would attempt a navigation. Replace it with a
// plain object whose href setter is just a property write so we can
// observe the value the component would have navigated to.
const originalLocation = window.location

function stubLocation(pathname: string, search = '', hash = '') {
  Object.defineProperty(window, 'location', {
    writable: true,
    configurable: true,
    value: {
      ...originalLocation,
      pathname,
      search,
      hash,
      href: '',
      assign: () => undefined,
      replace: () => undefined,
    } as unknown as Location,
  })
}

describe('LoginPage', () => {
  beforeEach(() => {
    stubLocation('/')
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      writable: true,
      configurable: true,
      value: originalLocation,
    })
  })

  it('navigates to /api/auth/login without query when current path is /', () => {
    render(
      <I18nProvider>
        <LoginPage />
      </I18nProvider>,
    )

    fireEvent.click(screen.getByRole('button'))

    expect(window.location.href).toBe('/api/auth/login')
  })

  it('appends an URL-encoded return_to with pathname, search, and hash', () => {
    stubLocation('/ws/abc/cases/xyz', '?tab=actions', '#step-3')

    render(
      <I18nProvider>
        <LoginPage />
      </I18nProvider>,
    )

    fireEvent.click(screen.getByRole('button'))

    expect(window.location.href).toBe(
      '/api/auth/login?return_to=%2Fws%2Fabc%2Fcases%2Fxyz%3Ftab%3Dactions%23step-3',
    )
  })

  it('sends the path as return_to even without query and hash', () => {
    stubLocation('/ws/abc/cases/xyz')

    render(
      <I18nProvider>
        <LoginPage />
      </I18nProvider>,
    )

    fireEvent.click(screen.getByRole('button'))

    expect(window.location.href).toBe(
      '/api/auth/login?return_to=%2Fws%2Fabc%2Fcases%2Fxyz',
    )
  })
})
