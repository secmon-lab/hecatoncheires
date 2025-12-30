import { useState, useEffect } from 'react'
import { mediaQueries } from '../styles/breakpoints'

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => {
    if (typeof window !== 'undefined') {
      return window.matchMedia(query).matches
    }
    return false
  })

  useEffect(() => {
    const mediaQuery = window.matchMedia(query)

    const handler = (event: MediaQueryListEvent) => {
      setMatches(event.matches)
    }

    mediaQuery.addEventListener('change', handler)
    return () => mediaQuery.removeEventListener('change', handler)
  }, [query])

  return matches
}

// Predefined breakpoint hooks using centralized breakpoint definitions
export function useIsMobile(): boolean {
  return useMediaQuery(mediaQueries.mobile)
}

export function useIsTablet(): boolean {
  return useMediaQuery(mediaQueries.tablet)
}

export function useIsDesktop(): boolean {
  return useMediaQuery(mediaQueries.desktop)
}

export function useIsMobileOrTablet(): boolean {
  return useMediaQuery(mediaQueries.mobileOrTablet)
}
