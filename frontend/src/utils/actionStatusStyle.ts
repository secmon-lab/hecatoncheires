// actionStatusStyle resolves an Action status `color` value (preset name or
// `#RRGGBB` hex) into an inline style. Backend validation guarantees one of
// these two shapes; anything else is treated as the neutral "idle" preset.

import type { CSSProperties } from 'react'

const presetVar: Record<string, string> = {
  idle: '--action-status-idle',
  active: '--action-status-active',
  waiting: '--action-status-waiting',
  paused: '--action-status-paused',
  attention: '--action-status-attention',
  blocked: '--action-status-blocked',
  success: '--action-status-success',
  neutral_done: '--action-status-neutral-done',
  failure: '--action-status-failure',
}

const HEX_RE = /^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$/

export function actionStatusColor(value: string | null | undefined): string {
  if (!value) return `var(${presetVar.idle})`
  const v = value.trim()
  if (HEX_RE.test(v)) return v
  const variable = presetVar[v.toLowerCase()]
  if (variable) return `var(${variable})`
  return `var(${presetVar.idle})`
}

export function actionStatusColorStyle(value: string | null | undefined): CSSProperties {
  return { background: actionStatusColor(value) }
}

// actionStatusSlug derives a stable kebab-case test id from a status id or
// display label. Lowercases and collapses underscores / whitespace to hyphens
// so e2e selectors like "kanban-column-in-progress" / "kanban-column-to-do"
// keep working regardless of which form is passed in.
export function actionStatusSlug(value: string): string {
  return value.toLowerCase().trim().replace(/[\s_]+/g, '-')
}
