import type { KeyboardEvent } from 'react'

// IME (Input Method Editor) composition guard.
//
// CJK users press Enter to confirm IME conversions (e.g. kana → kanji).
// During that confirmation, keydown fires with key="Enter" while the user
// is still composing. Treating that as a "commit" silently corrupts input.
//
// `nativeEvent.isComposing` covers modern browsers; `keyCode === 229`
// covers legacy Safari fallback. Both are required for full coverage.
//
// Intentionally NOT exported: callers must use the higher-level helpers
// (`commitOnEnter`, `activateOnEnterOrSpace`) so the guard cannot be
// forgotten or accidentally bypassed.
function isImeComposing(e: KeyboardEvent<HTMLElement>): boolean {
  if (e.nativeEvent.isComposing) return true
  if (e.keyCode === 229) return true
  return false
}

interface CommitHandlers {
  /** Called when the user confirms with Enter (or Cmd/Ctrl+Enter). */
  onCommit: () => void
  /** Called when the user cancels with Escape. */
  onCancel?: () => void
  /**
   * Require Cmd (mac) or Ctrl (win/linux) to be held with Enter to commit.
   * Use for `<textarea>` where bare Enter must insert a newline.
   */
  requireModifier?: boolean
}

/**
 * Build an `onKeyDown` handler for input / textarea elements that confirms
 * on Enter and cancels on Escape, with IME composition guards applied
 * automatically.
 *
 * **Always use this helper instead of writing `if (e.key === 'Enter')`
 * directly.** Direct Enter detection in component code is prohibited —
 * see `.claude/rules/frontend-keyboard-input.md`.
 */
export function commitOnEnter<E extends HTMLElement = HTMLElement>(
  handlers: CommitHandlers,
): (e: KeyboardEvent<E>) => void {
  return (e) => {
    if (e.key === 'Enter') {
      if (isImeComposing(e)) return
      if (handlers.requireModifier && !(e.metaKey || e.ctrlKey)) return
      e.preventDefault()
      handlers.onCommit()
      return
    }
    if (e.key === 'Escape') {
      if (!handlers.onCancel) return
      e.preventDefault()
      handlers.onCancel()
    }
  }
}

/**
 * Build an `onKeyDown` handler for `role="button"` style pseudo-buttons
 * that activate on Enter or Space, with IME composition guards applied
 * automatically.
 *
 * **Always use this helper for keyboard-activated div / span elements.**
 */
export function activateOnEnterOrSpace<E extends HTMLElement = HTMLElement>(
  onActivate: () => void,
): (e: KeyboardEvent<E>) => void {
  return (e) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    if (isImeComposing(e)) return
    e.preventDefault()
    onActivate()
  }
}
