import type { CSSProperties, SVGProps } from 'react'

interface IconProps extends Omit<SVGProps<SVGSVGElement>, 'd'> {
  d: string
  size?: number
  sw?: number
  fill?: string
  style?: CSSProperties
}

export function Icon({ d, size = 16, sw = 1.6, fill = 'none', style, ...rest }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={fill}
      stroke="currentColor"
      strokeWidth={sw}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={style}
      {...rest}
    >
      <path d={d} />
    </svg>
  )
}

type P = Omit<IconProps, 'd'>
export const IconCases = (p: P) => <Icon {...p} d="M3 7h18M5 7v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7M9 7V5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2" />
export const IconActions = (p: P) => <Icon {...p} d="M3 6h13M3 12h13M3 18h9M19 5l2 2-3 3-2-2zM18 11l3 3-5 5h-3v-3z" />
export const IconKnowledge = (p: P) => <Icon {...p} d="M4 4h12a3 3 0 0 1 3 3v13H7a3 3 0 0 1-3-3V4zM4 17a3 3 0 0 1 3-3h12" />
export const IconSources = (p: P) => <Icon {...p} d="M3 6c0 1.7 4 3 9 3s9-1.3 9-3-4-3-9-3-9 1.3-9 3zM3 6v12c0 1.7 4 3 9 3s9-1.3 9-3V6M3 12c0 1.7 4 3 9 3s9-1.3 9-3" />
export const IconBell = (p: P) => <Icon {...p} d="M18 16v-5a6 6 0 1 0-12 0v5l-2 2v1h16v-1l-2-2zM10 21h4" />
export const IconChevDown = (p: P) => <Icon {...p} d="M6 9l6 6 6-6" />
export const IconChevRight = (p: P) => <Icon {...p} d="M9 6l6 6-6 6" />
export const IconChevLeft = (p: P) => <Icon {...p} d="M15 6l-6 6 6 6" />
export const IconPlus = (p: P) => <Icon {...p} d="M12 5v14M5 12h14" />
export const IconSearch = (p: P) => <Icon {...p} d="M11 19a8 8 0 1 1 0-16 8 8 0 0 1 0 16zM21 21l-4.3-4.3" />
export const IconLock = (p: P) => <Icon {...p} d="M5 11h14v10H5zM8 11V7a4 4 0 0 1 8 0v4" />
export const IconExt = (p: P) => <Icon {...p} d="M14 4h6v6M10 14L20 4M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" />
export const IconDots = (p: P) => <Icon {...p} d="M12 6h.01M12 12h.01M12 18h.01" sw={2.5} />
export const IconX = (p: P) => <Icon {...p} d="M18 6L6 18M6 6l12 12" />
export const IconCheck = (p: P) => <Icon {...p} d="M5 12l5 5L20 7" />
export const IconWarn = (p: P) => <Icon {...p} d="M12 2L1 21h22L12 2zM12 9v5M12 18h.01" />
export const IconCalendar = (p: P) => <Icon {...p} d="M3 6h18v15H3zM3 11h18M8 3v5M16 3v5" />
export const IconUser = (p: P) => <Icon {...p} d="M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0" />
export const IconRefresh = (p: P) => <Icon {...p} d="M3 12a9 9 0 1 0 9-9M3 4v5h5" />
export const IconFilter = (p: P) => <Icon {...p} d="M3 6h18M6 12h12M10 18h4" />
export const IconSettings = (p: P) => <Icon {...p} d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.6 1.6 0 0 0 .3 1.7l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.7-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.7.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.7 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.7l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.7.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.7-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.7V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z" />

export function IconSlack({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect x="3" y="10" width="6" height="2.4" rx="1.2" fill="#36C5F0" />
      <rect x="3" y="14" width="2.4" height="6" rx="1.2" fill="#2EB67D" />
      <rect x="14.6" y="11.6" width="6.4" height="2.4" rx="1.2" fill="#ECB22E" />
      <rect x="11.6" y="3" width="2.4" height="6.4" rx="1.2" fill="#E01E5A" />
      <rect x="14.6" y="14.6" width="2.4" height="6.4" rx="1.2" fill="#36C5F0" />
      <rect x="11.6" y="14.6" width="2.4" height="2.4" rx="1.2" fill="#36C5F0" />
      <rect x="6.6" y="11.6" width="2.4" height="2.4" rx="1.2" fill="#E01E5A" />
      <rect x="14.6" y="6.6" width="2.4" height="2.4" rx="1.2" fill="#ECB22E" />
    </svg>
  )
}

export function IconGitHub({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M12 2A10 10 0 0 0 8.84 21.5c.5.08.66-.23.66-.5v-1.7c-2.78.6-3.37-1.34-3.37-1.34-.45-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.89 1.52 2.34 1.08 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.55-1.11-4.55-4.94 0-1.1.39-1.99 1.03-2.69-.1-.25-.45-1.27.1-2.65 0 0 .84-.27 2.75 1.02a9.6 9.6 0 0 1 5 0c1.91-1.29 2.75-1.02 2.75-1.02.55 1.38.2 2.4.1 2.65.64.7 1.03 1.59 1.03 2.69 0 3.84-2.34 4.69-4.57 4.94.36.31.68.92.68 1.86V21c0 .27.16.59.67.5A10 10 0 0 0 12 2z" />
    </svg>
  )
}

export function IconNotion({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <rect x="4" y="3" width="16" height="18" rx="2" />
      <path d="M8 7l8 11M8 7v11M16 7v11" />
    </svg>
  )
}
