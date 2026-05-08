import type { StylesConfig } from 'react-select'

// Shared, restrained style overrides used by every react-select instance in
// the app. Keeps the tactical look consistent and avoids react-select's
// blunt defaults (oversized control, neon-blue highlights, generic radii).
//
// Pass `error` to flip control border to var(--danger). Pass `compact` for
// tighter chrome (used by inline filter dropdowns where space is limited).
interface Opts {
  error?: boolean
  compact?: boolean
}

export function buildSelectStyles(opts: Opts = {}): StylesConfig<any, boolean> {
  const { error, compact } = opts
  const minH = compact ? 30 : 36
  const fontSize = compact ? 12 : 13
  const optPadX = compact ? 10 : 12
  const optPadY = compact ? 6 : 8
  return {
    menuPortal: (base) => ({ ...base, zIndex: 1100 }),
    control: (base, state) => ({
      ...base,
      minHeight: minH,
      fontSize,
      borderRadius: 6,
      borderColor: error
        ? 'var(--danger)'
        : state.isFocused
          ? 'var(--accent)'
          : 'var(--line)',
      boxShadow: state.isFocused
        ? '0 0 0 3px color-mix(in oklch, var(--accent) 18%, transparent)'
        : 'none',
      ':hover': { borderColor: error ? 'var(--danger)' : 'var(--accent)' },
      background: 'var(--bg-elev)',
      transition: 'border-color 120ms ease, box-shadow 120ms ease',
    }),
    valueContainer: (base) => ({ ...base, padding: compact ? '0 6px' : '2px 8px' }),
    indicatorSeparator: (base) => ({ ...base, background: 'var(--line)', margin: '6px 0' }),
    dropdownIndicator: (base, state) => ({
      ...base,
      padding: compact ? 4 : 6,
      color: state.isFocused ? 'var(--accent)' : 'var(--fg-soft)',
      ':hover': { color: 'var(--accent)' },
    }),
    clearIndicator: (base) => ({
      ...base,
      padding: compact ? 4 : 6,
      color: 'var(--fg-soft)',
      ':hover': { color: 'var(--danger)' },
    }),
    placeholder: (base) => ({ ...base, color: 'var(--fg-muted)', fontSize }),
    singleValue: (base) => ({ ...base, color: 'var(--fg)' }),
    input: (base) => ({ ...base, color: 'var(--fg)' }),
    menu: (base) => ({
      ...base,
      borderRadius: 8,
      border: '1px solid var(--line)',
      boxShadow: '0 8px 24px color-mix(in oklch, var(--fg) 8%, transparent)',
      overflow: 'hidden',
      background: 'var(--bg-elev)',
      marginTop: 4,
    }),
    menuList: (base) => ({ ...base, padding: 4 }),
    option: (base, state) => ({
      ...base,
      padding: `${optPadY}px ${optPadX}px`,
      borderRadius: 4,
      fontSize,
      cursor: 'pointer',
      color: state.isSelected ? 'var(--fg)' : 'var(--fg)',
      background: state.isSelected
        ? 'color-mix(in oklch, var(--accent) 14%, transparent)'
        : state.isFocused
          ? 'color-mix(in oklch, var(--accent) 7%, transparent)'
          : 'transparent',
      ':active': {
        background: 'color-mix(in oklch, var(--accent) 22%, transparent)',
      },
    }),
    multiValue: (base) => ({
      ...base,
      borderRadius: 4,
      background: 'color-mix(in oklch, var(--accent) 12%, var(--bg-sunken))',
      border: '1px solid color-mix(in oklch, var(--accent) 30%, var(--line))',
    }),
    multiValueLabel: (base) => ({ ...base, color: 'var(--fg)', fontSize: fontSize - 1, padding: '2px 4px 2px 6px' }),
    multiValueRemove: (base) => ({
      ...base,
      color: 'var(--fg-soft)',
      borderRadius: 4,
      ':hover': { background: 'color-mix(in oklch, var(--danger) 18%, transparent)', color: 'var(--danger)' },
    }),
    noOptionsMessage: (base) => ({ ...base, color: 'var(--fg-muted)', fontSize }),
  }
}

export const portalProps = {
  menuPortalTarget: typeof document !== 'undefined' ? document.body : undefined,
  menuPosition: 'fixed' as const,
  menuPlacement: 'auto' as const,
  menuShouldScrollIntoView: false,
  maxMenuHeight: 240,
}
