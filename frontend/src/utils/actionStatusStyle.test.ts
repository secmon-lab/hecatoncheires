import { describe, it, expect } from 'vitest'
import { actionStatusColor, actionStatusColorStyle, actionStatusSlug } from './actionStatusStyle'

describe('actionStatusColor', () => {
  it('returns the idle preset for empty / unknown values', () => {
    expect(actionStatusColor(undefined)).toBe('var(--action-status-idle)')
    expect(actionStatusColor(null)).toBe('var(--action-status-idle)')
    expect(actionStatusColor('')).toBe('var(--action-status-idle)')
    expect(actionStatusColor('not-a-preset')).toBe('var(--action-status-idle)')
  })

  it('maps preset names to CSS variables (case-insensitive)', () => {
    expect(actionStatusColor('active')).toBe('var(--action-status-active)')
    expect(actionStatusColor('SUCCESS')).toBe('var(--action-status-success)')
    expect(actionStatusColor('neutral_done')).toBe('var(--action-status-neutral-done)')
  })

  it('passes through hex colors verbatim', () => {
    expect(actionStatusColor('#5EAEDC')).toBe('#5EAEDC')
    expect(actionStatusColor('#abc')).toBe('#abc')
  })

  it('rejects css var or rgb expressions and falls back', () => {
    expect(actionStatusColor('var(--ok)')).toBe('var(--action-status-idle)')
    expect(actionStatusColor('rgb(0,0,0)')).toBe('var(--action-status-idle)')
  })
})

describe('actionStatusColorStyle', () => {
  it('wraps the resolved color in a CSS background style', () => {
    expect(actionStatusColorStyle('blocked')).toEqual({ background: 'var(--action-status-blocked)' })
    expect(actionStatusColorStyle('#fff')).toEqual({ background: '#fff' })
  })
})

describe('actionStatusSlug', () => {
  it('lowercases and converts underscores to hyphens', () => {
    expect(actionStatusSlug('IN_PROGRESS')).toBe('in-progress')
    expect(actionStatusSlug('backlog')).toBe('backlog')
    expect(actionStatusSlug('Waiting_User')).toBe('waiting-user')
  })

  it('collapses whitespace in display labels', () => {
    expect(actionStatusSlug('To Do')).toBe('to-do')
    expect(actionStatusSlug('In Progress')).toBe('in-progress')
    expect(actionStatusSlug('  Quick   Triage ')).toBe('quick-triage')
  })
})
