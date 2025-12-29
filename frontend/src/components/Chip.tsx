import { ReactNode } from 'react'
import styles from './Chip.module.css'

interface ChipProps {
  variant?: 'default' | 'category' | 'team' | 'user' | 'primary' | 'secondary' | 'status'
  size?: 'sm' | 'md'
  onRemove?: () => void
  icon?: ReactNode
  colorIndex?: number
  children: ReactNode
}

const CHIP_COLORS = [
  { bg: '#DBEAFE', text: '#1E3A8A', border: '#93C5FD' }, // blue
  { bg: '#D1FAE5', text: '#065F46', border: '#6EE7B7' }, // green
  { bg: '#E9D5FF', text: '#6B21A8', border: '#D8B4FE' }, // purple
  { bg: '#FED7AA', text: '#7C2D12', border: '#FDBA74' }, // orange
  { bg: '#FEF3C7', text: '#78350F', border: '#FDE68A' }, // yellow
  { bg: '#FECDD3', text: '#881337', border: '#FDA4AF' }, // rose
  { bg: '#E0E7FF', text: '#3730A3', border: '#C7D2FE' }, // indigo
  { bg: '#FCE7F3', text: '#9F1239', border: '#FBCFE8' }, // pink
  { bg: '#A7F3D0', text: '#064E3B', border: '#6EE7B7' }, // emerald
  { bg: '#FEF3C7', text: '#92400E', border: '#FDE68A' }, // amber
  { bg: '#F3E8FF', text: '#581C87', border: '#E9D5FF' }, // violet
  { bg: '#CCFBF1', text: '#115E59', border: '#5EEAD4' }, // teal
  { bg: '#FFEDD5', text: '#9A3412', border: '#FED7AA' }, // orange-light
  { bg: '#BAE6FD', text: '#075985', border: '#7DD3FC' }, // sky
  { bg: '#F5D0FE', text: '#86198F', border: '#F0ABFC' }, // fuchsia
  { bg: '#FFE4E6', text: '#9F1239', border: '#FECDD3' }, // rose-light
  { bg: '#BBF7D0', text: '#14532D', border: '#86EFAC' }, // green-light
  { bg: '#C7D2FE', text: '#312E81', border: '#A5B4FC' }, // indigo-light
  { bg: '#EDE9FE', text: '#5B21B6', border: '#DDD6FE' }, // purple-lighter
  { bg: '#FEF9C3', text: '#713F12', border: '#FEF08A' }, // yellow-lighter
  { bg: '#E0F2FE', text: '#0C4A6E', border: '#BAE6FD' }, // sky-lighter
  { bg: '#F0FDFA', text: '#134E4A', border: '#CCFBF1' }, // teal-lighter
  { bg: '#FDF2F8', text: '#831843', border: '#FBCFE8' }, // pink-lighter
  { bg: '#FEF2F2', text: '#7F1D1D', border: '#FECACA' }, // red-lighter
  { bg: '#F7FEE7', text: '#3F6212', border: '#D9F99D' }, // lime
  { bg: '#FFFBEB', text: '#78350F', border: '#FDE68A' }, // amber-lighter
  { bg: '#ECFEFF', text: '#155E75', border: '#A5F3FC' }, // cyan
  { bg: '#FDF4FF', text: '#701A75', border: '#F5D0FE' }, // fuchsia-lighter
  { bg: '#FFF1F2', text: '#9F1239', border: '#FFE4E6' }, // rose-lighter
  { bg: '#F0FDF4', text: '#14532D', border: '#BBF7D0' }, // green-lightest
]

export default function Chip({
  variant = 'default',
  size = 'md',
  onRemove,
  icon,
  colorIndex,
  children,
}: ChipProps) {
  const getStyle = () => {
    if (variant === 'category' && colorIndex !== undefined) {
      const color = CHIP_COLORS[colorIndex % CHIP_COLORS.length]
      return {
        backgroundColor: color.bg,
        color: color.text,
        border: `1px solid ${color.border}`,
      }
    }
    if (variant === 'team' && colorIndex !== undefined) {
      const color = CHIP_COLORS[colorIndex % CHIP_COLORS.length]
      return {
        backgroundColor: color.bg,
        color: color.text,
        border: `1px solid ${color.border}`,
      }
    }
    if (variant === 'status' && colorIndex !== undefined) {
      const color = CHIP_COLORS[colorIndex % CHIP_COLORS.length]
      return {
        backgroundColor: color.bg,
        color: color.text,
        border: `1px solid ${color.border}`,
      }
    }
    return {}
  }

  const classNames = [
    styles.chip,
    colorIndex === undefined ? styles[variant] : '',
    styles[size],
  ].filter(Boolean).join(' ')

  return (
    <span className={classNames} style={getStyle()}>
      {icon && <span className={styles.icon}>{icon}</span>}
      <span className={styles.label}>{children}</span>
      {onRemove && (
        <button
          type="button"
          className={styles.removeButton}
          onClick={onRemove}
          aria-label="Remove"
        >
          ×
        </button>
      )}
    </span>
  )
}
