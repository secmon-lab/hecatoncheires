// Workspace badge visual resolution shared by the workspace selector page and
// the breadcrumb workspace switcher, so both render an identical badge for the
// same workspace.
//
// Resolution rules (mutually exclusive, in priority order):
//   1. emoji set    → render the emoji on a neutral background
//   2. color set    → render the initials on a gradient derived from the color
//   3. neither       → render the initials on an auto color picked
//                      deterministically from the workspace id (stable across
//                      reordering / count changes)

// Auto-color palette. Index is chosen by a deterministic hash of the workspace
// id so the same workspace always gets the same color regardless of list order.
export const WORKSPACE_GRADIENTS = [
  'linear-gradient(135deg, #5b6cff, #8b3fb5)',
  'linear-gradient(135deg, #ff9b3f, #c8501c)',
  'linear-gradient(135deg, #2cb38d, #126b56)',
  'linear-gradient(135deg, #3fb6e5, #1d6f9e)',
  'linear-gradient(135deg, #e25b8e, #872551)',
]

// Neutral background for emoji badges. Uses a design token so it follows the
// light/dark theme rather than hardcoding a color.
export const NEUTRAL_BADGE_BG = 'var(--bg-subtle)'

// hexColorPattern matches the 6-digit #RRGGBB form accepted by the backend.
const hexColorPattern = /^#[0-9a-fA-F]{6}$/

export interface WorkspaceVisualInput {
  id: string
  name: string
  emoji?: string | null
  color?: string | null
}

export type WorkspaceVisual =
  | { kind: 'emoji'; emoji: string; background: string }
  | { kind: 'mark'; mark: string; background: string }

// workspaceMark builds a short initials badge label from the workspace name.
// Two words → first letter of each; otherwise the first two characters.
export function workspaceMark(name: string): string {
  const trimmed = (name || '').trim()
  if (!trimmed) return '?'
  const parts = trimmed.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return trimmed.slice(0, 2).toUpperCase()
}

// hashIndex computes a deterministic, order-independent index in [0, mod).
// djb2 over the UTF-16 code units of the key.
function hashIndex(key: string, mod: number): number {
  let hash = 5381
  for (let i = 0; i < key.length; i++) {
    hash = ((hash << 5) + hash + key.charCodeAt(i)) >>> 0
  }
  return hash % mod
}

// darkenHex multiplies each RGB channel of a #RRGGBB color by factor (0..1)
// and returns the resulting #RRGGBB string. Used to derive the darker end stop
// of the badge gradient.
function darkenHex(hex: string, factor: number): string {
  const r = Math.round(parseInt(hex.slice(1, 3), 16) * factor)
  const g = Math.round(parseInt(hex.slice(3, 5), 16) * factor)
  const b = Math.round(parseInt(hex.slice(5, 7), 16) * factor)
  const clamp = (v: number) => Math.max(0, Math.min(255, v))
  const toHex = (v: number) => clamp(v).toString(16).padStart(2, '0')
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`
}

// colorGradient builds a 135° gradient from a base #RRGGBB color to a darker
// shade, matching the look of the auto-color palette.
function colorGradient(hex: string): string {
  return `linear-gradient(135deg, ${hex}, ${darkenHex(hex, 0.7)})`
}

// workspaceVisual resolves how a workspace badge should be rendered.
export function workspaceVisual(ws: WorkspaceVisualInput): WorkspaceVisual {
  const emoji = (ws.emoji || '').trim()
  if (emoji) {
    return { kind: 'emoji', emoji, background: NEUTRAL_BADGE_BG }
  }

  const mark = workspaceMark(ws.name)
  const color = (ws.color || '').trim()
  if (hexColorPattern.test(color)) {
    return { kind: 'mark', mark, background: colorGradient(color) }
  }

  const idx = hashIndex(ws.id || ws.name, WORKSPACE_GRADIENTS.length)
  return { kind: 'mark', mark, background: WORKSPACE_GRADIENTS[idx] }
}
