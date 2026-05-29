import { describe, it, expect } from 'vitest'
import {
  workspaceMark,
  workspaceVisual,
  WORKSPACE_GRADIENTS,
  NEUTRAL_BADGE_BG,
} from './workspace'

describe('workspaceMark', () => {
  it('returns ? for empty / whitespace names', () => {
    expect(workspaceMark('')).toBe('?')
    expect(workspaceMark('   ')).toBe('?')
  })

  it('uses the first letter of each of the first two words', () => {
    expect(workspaceMark('Risk Management')).toBe('RM')
    expect(workspaceMark('incident response team')).toBe('IR')
  })

  it('uses the first two characters for single-word names', () => {
    expect(workspaceMark('risk')).toBe('RI')
    expect(workspaceMark('リスク管理')).toBe('リス')
  })

  it('slices on Unicode code points so surrogate pairs are not split', () => {
    // A name starting with a surrogate-pair emoji must not produce a broken
    // half-character. Two words → first code point of each.
    expect(workspaceMark('🎉 Party')).toBe('🎉P')
    // Single word starting with surrogate pairs → first two code points intact.
    expect(workspaceMark('🎉🎊fest')).toBe('🎉🎊')
  })
})

describe('workspaceVisual', () => {
  it('renders an emoji on a neutral background when emoji is set', () => {
    const v = workspaceVisual({ id: 'risk', name: 'Risk Management', emoji: '🛡️' })
    expect(v.kind).toBe('emoji')
    if (v.kind === 'emoji') {
      expect(v.emoji).toBe('🛡️')
      expect(v.background).toBe(NEUTRAL_BADGE_BG)
    }
  })

  it('renders initials on a gradient derived from a valid color', () => {
    const v = workspaceVisual({ id: 'incident', name: 'Incident Response', color: '#c8501c' })
    expect(v.kind).toBe('mark')
    if (v.kind === 'mark') {
      expect(v.mark).toBe('IR')
      // Base color is the first gradient stop; a darker shade is the second.
      expect(v.background).toContain('#c8501c')
      expect(v.background.startsWith('linear-gradient(135deg, #c8501c,')).toBe(true)
    }
  })

  it('emoji takes precedence over color (mutual exclusion is enforced at config, but defend in UI)', () => {
    const v = workspaceVisual({ id: 'risk', name: 'Risk', emoji: '🛡️', color: '#c8501c' })
    expect(v.kind).toBe('emoji')
  })

  it('falls back to an auto-color initials badge when neither emoji nor color is set', () => {
    const v = workspaceVisual({ id: 'risk', name: 'Risk Management' })
    expect(v.kind).toBe('mark')
    if (v.kind === 'mark') {
      expect(v.mark).toBe('RM')
      expect(WORKSPACE_GRADIENTS).toContain(v.background)
    }
  })

  it('falls back to auto color when color is malformed', () => {
    const v = workspaceVisual({ id: 'risk', name: 'Risk', color: 'blue' })
    expect(v.kind).toBe('mark')
    if (v.kind === 'mark') {
      expect(WORKSPACE_GRADIENTS).toContain(v.background)
    }
  })

  it('falls back to auto color when color is a 3-digit hex (only 6-digit accepted)', () => {
    const v = workspaceVisual({ id: 'risk', name: 'Risk', color: '#fff' })
    expect(v.kind).toBe('mark')
    if (v.kind === 'mark') {
      expect(WORKSPACE_GRADIENTS).toContain(v.background)
    }
  })

  it('auto color is deterministic for the same id', () => {
    const a = workspaceVisual({ id: 'risk', name: 'Risk Management' })
    const b = workspaceVisual({ id: 'risk', name: 'Totally Different Name' })
    expect(a.kind).toBe('mark')
    expect(b.kind).toBe('mark')
    if (a.kind === 'mark' && b.kind === 'mark') {
      // Same id → same color, independent of name and list order.
      expect(a.background).toBe(b.background)
    }
  })

  it('auto color is independent of position in any list', () => {
    const ids = ['buddy', 'incident', 'risk']
    const first = ids.map((id) => workspaceVisual({ id, name: id }))
    const reversed = [...ids].reverse().map((id) => workspaceVisual({ id, name: id }))
    // Each id maps to the same color regardless of where it appears.
    for (const id of ids) {
      const fromFirst = first.find((_, i) => ids[i] === id)
      const fromReversed = reversed.find((_, i) => [...ids].reverse()[i] === id)
      expect(fromFirst && fromFirst.kind === 'mark' && fromFirst.background).toBe(
        fromReversed && fromReversed.kind === 'mark' && fromReversed.background,
      )
    }
  })
})
