export const colors = {
  // Primary purple gradient (from the image)
  primary: {
    main: '#9c27b0',      // Vibrant purple
    light: '#ba68c8',
    dark: '#7b1fa2',
    gradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
    contrastText: '#ffffff',
  },
  // Secondary colors matching the dashboard
  secondary: {
    main: '#00bcd4',      // Cyan
    light: '#4dd0e1',
    dark: '#0097a7',
    contrastText: '#ffffff',
  },
  // Card colors from the design
  cards: {
    orange: '#ff9800',
    orangeLight: '#ffb74d',
    orangeGradient: 'linear-gradient(135deg, #FFA726 0%, #FB8C00 100%)',

    green: '#4caf50',
    greenLight: '#81c784',
    greenGradient: 'linear-gradient(135deg, #66BB6A 0%, #43A047 100%)',

    red: '#f44336',
    redLight: '#e57373',
    redGradient: 'linear-gradient(135deg, #EF5350 0%, #E53935 100%)',

    cyan: '#00bcd4',
    cyanLight: '#4dd0e1',
    cyanGradient: 'linear-gradient(135deg, #26C6DA 0%, #00ACC1 100%)',

    purple: '#9c27b0',
    purpleLight: '#ba68c8',
    purpleGradient: 'linear-gradient(135deg, #AB47BC 0%, #8E24AA 100%)',
  },
  success: {
    main: '#4caf50',
    light: '#81c784',
    dark: '#388e3c',
  },
  warning: {
    main: '#ff9800',
    light: '#ffb74d',
    dark: '#f57c00',
  },
  error: {
    main: '#f44336',
    light: '#e57373',
    dark: '#d32f2f',
  },
  info: {
    main: '#2196f3',
    light: '#64b5f6',
    dark: '#1976d2',
  },
  // Background and surfaces
  background: {
    default: '#f5f6fa',   // Light grayish blue
    paper: '#ffffff',
    dark: '#2d3436',
  },
  // Text colors
  text: {
    primary: '#2c3e50',
    secondary: '#7f8c8d',
    disabled: '#bdc3c7',
    hint: '#95a5a6',
    white: '#ffffff',
  },
  // Sidebar specific colors (purple gradient)
  sidebar: {
    background: 'linear-gradient(180deg, #667eea 0%, #764ba2 100%)',
    backgroundSolid: '#667eea',
    text: '#ffffff',
    textSecondary: 'rgba(255, 255, 255, 0.7)',
    active: 'rgba(255, 255, 255, 0.2)',
    hover: 'rgba(255, 255, 255, 0.1)',
    border: 'rgba(255, 255, 255, 0.1)',
  },
  // Divider
  divider: '#e0e0e0',
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
  md: '8px',
  lg: '12px',
  xl: '16px',
  round: '50%',
}

export const shadows = {
  sm: '0 2px 4px rgba(0, 0, 0, 0.1)',
  md: '0 4px 8px rgba(0, 0, 0, 0.12)',
  lg: '0 8px 16px rgba(0, 0, 0, 0.15)',
  xl: '0 12px 24px rgba(0, 0, 0, 0.2)',
  card: '0 2px 8px rgba(0, 0, 0, 0.1)',
  cardHover: '0 4px 16px rgba(0, 0, 0, 0.15)',
}

export const transitions = {
  fast: '150ms cubic-bezier(0.4, 0, 0.2, 1)',
  normal: '300ms cubic-bezier(0.4, 0, 0.2, 1)',
  slow: '500ms cubic-bezier(0.4, 0, 0.2, 1)',
}

export const typography = {
  fontFamily: 'Roboto, -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif',
  fontSize: {
    xs: '12px',
    sm: '14px',
    md: '16px',
    lg: '18px',
    xl: '24px',
    xxl: '32px',
    xxxl: '48px',
  },
  fontWeight: {
    light: 300,
    regular: 400,
    medium: 500,
    semibold: 600,
    bold: 700,
  },
}
