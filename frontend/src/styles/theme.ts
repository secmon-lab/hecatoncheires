// RGB color values for dynamic rgba usage
const primaryColorRGB = '59, 130, 246'

export const colors = {
  // Primary blue
  primary: {
    rgb: primaryColorRGB,
    main: '#3B82F6',
    light: '#60A5FA',
    dark: '#1D4ED8',
    gradient: 'linear-gradient(135deg, #3B82F6 0%, #60A5FA 100%)',
    contrastText: '#ffffff',
  },
  // Secondary colors
  secondary: {
    main: '#06B6D4',
    light: '#22D3EE',
    dark: '#0891B2',
    contrastText: '#ffffff',
  },
  // Card colors (flat)
  cards: {
    orange: '#F59E0B',
    orangeLight: '#FEF3C7',
    orangeGradient: '#F59E0B',

    green: '#10B981',
    greenLight: '#D1FAE5',
    greenGradient: '#10B981',

    red: '#EF4444',
    redLight: '#FEE2E2',
    redGradient: '#EF4444',

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
    main: '#EF4444',
    light: '#FEE2E2',
    dark: '#DC2626',
  },
  info: {
    main: '#3B82F6',
    light: '#DBEAFE',
    dark: '#2563EB',
  },
  // Background and surfaces
  background: {
    default: '#F8F9FA',
    paper: '#ffffff',
    dark: '#111827',
  },
  // Text colors
  text: {
    primary: '#111827',
    secondary: '#6B7280',
    disabled: '#D1D5DB',
    hint: '#9CA3AF',
    white: '#ffffff',
    // Semantic text colors
    heading: '#111827',
    label: '#6B7280',
    body: '#374151',
    muted: '#9CA3AF',
  },
  // Sidebar specific colors
  sidebar: {
    background: '#ffffff',
    backgroundSolid: '#ffffff',
    text: '#6B7280',
    textSecondary: `rgba(${primaryColorRGB}, 0.7)`,
    active: 'rgba(59, 130, 246, 0.06)',
    hover: 'rgba(0, 0, 0, 0.04)',
    border: 'rgba(0, 0, 0, 0.06)',
  },
  // Divider
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
  lg: '8px',
  xl: '12px',
  round: '50%',
}

export const shadows = {
  sm: '0 1px 2px rgba(0, 0, 0, 0.04)',
  md: '0 2px 4px rgba(0, 0, 0, 0.06)',
  lg: '0 4px 12px rgba(0, 0, 0, 0.08)',
  xl: '0 8px 24px rgba(0, 0, 0, 0.12)',
  card: '0 1px 2px rgba(0, 0, 0, 0.04)',
  cardHover: '0 2px 8px rgba(0, 0, 0, 0.08)',
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
  // Semantic typography scales
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

// Card and panel tokens
export const card = {
  background: '#ffffff',
  border: 'rgba(0, 0, 0, 0.08)',
  radius: '8px',
  padding: '16px 20px',
  shadow: '0 1px 2px rgba(0, 0, 0, 0.04)',
}

// Level card (LIKELIHOOD/IMPACT)
export const levelCard = {
  background: '#F8F9FA',
  border: 'rgba(0, 0, 0, 0.06)',
}

// Table tokens
export const table = {
  headerBg: '#F9FAFB',
  headerBorder: 'rgba(0, 0, 0, 0.08)',
  rowBorder: 'rgba(0, 0, 0, 0.04)',
  rowHover: '#F9FAFB',
}
