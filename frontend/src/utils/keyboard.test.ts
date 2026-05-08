import { describe, it, expect, vi } from 'vitest'
import type { KeyboardEvent } from 'react'
import { commitOnEnter, activateOnEnterOrSpace } from './keyboard'

// Build a minimal React.KeyboardEvent for handler testing. Only the fields
// the helpers actually read are populated.
function makeEvent(opts: {
  key: string
  isComposing?: boolean
  keyCode?: number
  metaKey?: boolean
  ctrlKey?: boolean
}): KeyboardEvent<HTMLElement> {
  const preventDefault = vi.fn()
  return {
    key: opts.key,
    keyCode: opts.keyCode ?? 0,
    metaKey: opts.metaKey ?? false,
    ctrlKey: opts.ctrlKey ?? false,
    nativeEvent: { isComposing: opts.isComposing ?? false } as unknown as globalThis.KeyboardEvent,
    preventDefault,
  } as unknown as KeyboardEvent<HTMLElement>
}

describe('commitOnEnter', () => {
  it('invokes onCommit on plain Enter', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit })
    const e = makeEvent({ key: 'Enter' })
    handler(e)
    expect(onCommit).toHaveBeenCalledOnce()
    expect(e.preventDefault).toHaveBeenCalledOnce()
  })

  it('does NOT invoke onCommit while IME is composing (isComposing=true)', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit })
    const e = makeEvent({ key: 'Enter', isComposing: true })
    handler(e)
    expect(onCommit).not.toHaveBeenCalled()
    expect(e.preventDefault).not.toHaveBeenCalled()
  })

  it('does NOT invoke onCommit when keyCode=229 (legacy Safari IME)', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit })
    const e = makeEvent({ key: 'Enter', keyCode: 229 })
    handler(e)
    expect(onCommit).not.toHaveBeenCalled()
  })

  it('invokes onCancel on Escape', () => {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const handler = commitOnEnter({ onCommit, onCancel })
    const e = makeEvent({ key: 'Escape' })
    handler(e)
    expect(onCancel).toHaveBeenCalledOnce()
    expect(onCommit).not.toHaveBeenCalled()
    expect(e.preventDefault).toHaveBeenCalledOnce()
  })

  it('ignores Escape when no onCancel is supplied (does not preventDefault)', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit })
    const e = makeEvent({ key: 'Escape' })
    handler(e)
    expect(e.preventDefault).not.toHaveBeenCalled()
  })

  it('ignores plain Enter when requireModifier=true', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit, requireModifier: true })
    handler(makeEvent({ key: 'Enter' }))
    expect(onCommit).not.toHaveBeenCalled()
  })

  it('invokes onCommit on Cmd+Enter when requireModifier=true', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit, requireModifier: true })
    handler(makeEvent({ key: 'Enter', metaKey: true }))
    expect(onCommit).toHaveBeenCalledOnce()
  })

  it('invokes onCommit on Ctrl+Enter when requireModifier=true', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit, requireModifier: true })
    handler(makeEvent({ key: 'Enter', ctrlKey: true }))
    expect(onCommit).toHaveBeenCalledOnce()
  })

  it('does NOT invoke onCommit on Cmd+Enter while IME is composing', () => {
    const onCommit = vi.fn()
    const handler = commitOnEnter({ onCommit, requireModifier: true })
    handler(makeEvent({ key: 'Enter', metaKey: true, isComposing: true }))
    expect(onCommit).not.toHaveBeenCalled()
  })

  it('ignores unrelated keys', () => {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const handler = commitOnEnter({ onCommit, onCancel })
    const e = makeEvent({ key: 'a' })
    handler(e)
    expect(onCommit).not.toHaveBeenCalled()
    expect(onCancel).not.toHaveBeenCalled()
    expect(e.preventDefault).not.toHaveBeenCalled()
  })
})

describe('activateOnEnterOrSpace', () => {
  it('activates on Enter', () => {
    const onActivate = vi.fn()
    const handler = activateOnEnterOrSpace(onActivate)
    const e = makeEvent({ key: 'Enter' })
    handler(e)
    expect(onActivate).toHaveBeenCalledOnce()
    expect(e.preventDefault).toHaveBeenCalledOnce()
  })

  it('activates on Space', () => {
    const onActivate = vi.fn()
    const handler = activateOnEnterOrSpace(onActivate)
    const e = makeEvent({ key: ' ' })
    handler(e)
    expect(onActivate).toHaveBeenCalledOnce()
    expect(e.preventDefault).toHaveBeenCalledOnce()
  })

  it('does NOT activate on Enter while IME is composing', () => {
    const onActivate = vi.fn()
    const handler = activateOnEnterOrSpace(onActivate)
    handler(makeEvent({ key: 'Enter', isComposing: true }))
    expect(onActivate).not.toHaveBeenCalled()
  })

  it('does NOT activate when keyCode=229', () => {
    const onActivate = vi.fn()
    const handler = activateOnEnterOrSpace(onActivate)
    handler(makeEvent({ key: 'Enter', keyCode: 229 }))
    expect(onActivate).not.toHaveBeenCalled()
  })

  it('ignores unrelated keys', () => {
    const onActivate = vi.fn()
    const handler = activateOnEnterOrSpace(onActivate)
    const e = makeEvent({ key: 'Tab' })
    handler(e)
    expect(onActivate).not.toHaveBeenCalled()
    expect(e.preventDefault).not.toHaveBeenCalled()
  })
})
