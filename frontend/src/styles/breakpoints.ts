// Breakpoint definitions
// Single source of truth for responsive design breakpoints
export const breakpoints = {
  mobile: 767,
  tablet: 1023,
  desktop: 1024,
} as const

// Media query helpers
export const mediaQueries = {
  mobile: `(max-width: ${breakpoints.mobile}px)`,
  tablet: `(min-width: ${breakpoints.mobile + 1}px) and (max-width: ${breakpoints.tablet}px)`,
  desktop: `(min-width: ${breakpoints.desktop}px)`,
  mobileOrTablet: `(max-width: ${breakpoints.tablet}px)`,
} as const
