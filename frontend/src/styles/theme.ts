// RGB color values for dynamic rgba usage
const primaryColorRGB = '220, 38, 38'

export const colors = {
  // Primary — Crimson Red
  primary: {
    rgb: primaryColorRGB,
    main: '#DC2626',
    light: '#EF4444',
    dark: '#B91C1C',
    gradient: 'linear-gradient(135deg, #DC2626 0%, #EF4444 100%)',
    contrastText: '#ffffff',
  },
  // Secondary — Warm Orange
  secondary: {
    main: '#F59E0B',
    light: '#FBBF24',
    dark: '#D97706',
    contrastText: '#ffffff',
  },
  // Card colors
  cards: {
    orange: '#F59E0B',
    orangeLight: '#FEF3C7',
    orangeGradient: '#F59E0B',

    green: '#10B981',
    greenLight: '#D1FAE5',
    greenGradient: '#10B981',

    red: '#DC2626',
    redLight: '#FEE2E2',
    redGradient: '#DC2626',

    cyan: '#06B6D4',
    cyanLight: '#CFFAFE',
    cyanGradient: '#06B6D4',

    purple: '#8B5CF6',
    purpleLight: '#EDE9FE',
    purpleGradient: '#8B5CF6',

    blue: '#3B82F6',
    blueLight: '#DBEAFE',
    blueGradient: '#3B82F6',
  },
  success: {
    main: '#10B981',
    light: '#D1FAE5',
    dark: '#059669',
  },
  warning: {
    main: '#F59E0B',
    light: '#FEF3C7',
    dark: '#D97706',
  },
  error: {
    main: '#DC2626',
    light: '#FEE2E2',
    dark: '#B91C1C',
  },
  info: {
    main: '#3B82F6',
    light: '#DBEAFE',
    dark: '#2563EB',
  },
  background: {
    default: '#F1F3F5',
    paper: '#ffffff',
    dark: '#1E1E2D',
  },
  text: {
    primary: '#111827',
    secondary: '#6B7280',
    disabled: '#D1D5DB',
    hint: '#9CA3AF',
    white: '#ffffff',
    heading: '#111827',
    label: '#6B7280',
    body: '#374151',
    muted: '#9CA3AF',
  },
  sidebar: {
    background: '#1E1E2D',
    backgroundSolid: '#1E1E2D',
    text: 'rgba(255, 255, 255, 0.6)',
    textSecondary: 'rgba(255, 255, 255, 0.4)',
    active: 'rgba(220, 38, 38, 0.15)',
    hover: 'rgba(255, 255, 255, 0.05)',
    border: 'rgba(255, 255, 255, 0.08)',
  },
  divider: 'rgba(0, 0, 0, 0.06)',
}

export const spacing = {
  xs: '4px',
  sm: '8px',
  md: '16px',
  lg: '24px',
  xl: '32px',
  xxl: '48px',
}

export const borderRadius = {
  sm: '4px',
  md: '6px',
  lg: '10px',
  xl: '14px',
  round: '50%',
}

export const shadows = {
  sm: '0 1px 3px rgba(0, 0, 0, 0.06)',
  md: '0 4px 6px rgba(0, 0, 0, 0.07)',
  lg: '0 10px 25px rgba(0, 0, 0, 0.1)',
  xl: '0 20px 40px rgba(0, 0, 0, 0.15)',
  card: '0 1px 3px rgba(0, 0, 0, 0.06)',
  cardHover: '0 4px 12px rgba(0, 0, 0, 0.1)',
}

export const transitions = {
  fast: '150ms cubic-bezier(0.4, 0, 0.2, 1)',
  normal: '300ms cubic-bezier(0.4, 0, 0.2, 1)',
  slow: '500ms cubic-bezier(0.4, 0, 0.2, 1)',
}

export const typography = {
  fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  fontSize: {
    xs: '12px',
    sm: '13px',
    md: '14px',
    lg: '16px',
    xl: '20px',
    xxl: '24px',
    xxxl: '36px',
  },
  fontWeight: {
    light: 300,
    regular: 400,
    medium: 500,
    semibold: 600,
    bold: 700,
  },
  pageTitle: {
    fontSize: '24px',
    fontWeight: 600,
    lineHeight: 1.2,
  },
  sectionLabel: {
    fontSize: '11px',
    fontWeight: 600,
    lineHeight: 1.4,
  },
  bodyText: {
    fontSize: '14px',
    fontWeight: 400,
    lineHeight: 1.6,
  },
}

export const card = {
  background: '#ffffff',
  border: 'rgba(0, 0, 0, 0.06)',
  radius: '10px',
  padding: '20px 24px',
  shadow: '0 1px 3px rgba(0, 0, 0, 0.06)',
}

export const levelCard = {
  background: '#F8F9FA',
  border: 'rgba(0, 0, 0, 0.06)',
}

export const table = {
  headerBg: '#F9FAFB',
  headerBorder: 'rgba(0, 0, 0, 0.06)',
  rowBorder: 'rgba(0, 0, 0, 0.04)',
  rowHover: '#F9FAFB',
}
