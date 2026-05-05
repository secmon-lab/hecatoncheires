---
paths:
  - "frontend/**/*"
---

# Frontend Keyboard & IME Input Handling

This project serves Japanese, Chinese, and Korean users who rely on IME
(Input Method Editor) to compose characters. **Naïve `Enter` key handling
breaks their input experience** — when an IME user presses Enter to
*confirm a conversion* (e.g. kana → kanji), the keydown event fires with
`key === 'Enter'` even though the user is still composing. Treating that
as "submit" or "save" silently corrupts their input.

**Assume CJK / IME users always exist. Never write Enter handlers without
guarding for composition.**

## The Rule

Any `onKeyDown` / `onKeyPress` / `onKeyUp` handler that reacts to `Enter`
(or `Space`) and triggers a side effect — saving a field, submitting a
form, switching modes, navigating, dismissing a dialog — **MUST guard
against IME composition** before executing the side effect.

Use the shared helper:

```ts
import { isImeComposing } from '@/utils/keyboard'

onKeyDown={(e) => {
  if (e.key === 'Enter') {
    if (isImeComposing(e)) return  // ← required guard
    e.preventDefault()
    void commit()
  }
}}
```

The helper checks both `event.nativeEvent.isComposing` (modern browsers)
and `event.keyCode === 229` (legacy Safari fallback). Do not inline these
checks — always go through the helper so the guard is uniform and the
intent is searchable.

## When the Guard Is Required

- Inline-edit components (`Inline*`) — text, number, date, long text, etc.
- Search / filter inputs that fire on Enter
- Chat / comment inputs that send on Enter
- Tag / chip / multi-add inputs ("type and Enter to add")
- `role="button"` divs that activate on Enter / Space
- Modal confirm-on-Enter shortcuts
- Custom keyboard shortcut hooks

## When the Guard Is NOT Required

- Buttons (`<button>`) — the browser handles Enter as click and never
  fires composition events on a button
- Pure navigation / arrow-key handlers (`ArrowUp`, `ArrowDown`, etc.) —
  IME doesn't intercept these
- `Escape` — IME generally consumes its own cancel; let the browser
  handle it
- `<form onSubmit>` on forms with multiple inputs — browsers already
  suppress submit during composition. Still, **prefer explicit submit
  buttons** over relying on implicit Enter-submit behavior

## Forbidden Patterns

```tsx
// ❌ No IME guard — saves mid-composition
onKeyDown={(e) => {
  if (e.key === 'Enter') void commit()
}}

// ❌ Inlining the check — duplication causes drift
onKeyDown={(e) => {
  if (e.key === 'Enter' && !e.nativeEvent.isComposing) void commit()
}}

// ❌ keyCode-only — modern browsers also need isComposing
if (e.keyCode === 13 && e.keyCode !== 229) ...
```

## Required Patterns

```tsx
// ✅ Use the shared helper
import { isImeComposing } from '@/utils/keyboard'

onKeyDown={(e) => {
  if (e.key === 'Enter') {
    if (isImeComposing(e)) return
    e.preventDefault()
    void commit()
  }
  if (e.key === 'Escape') {
    e.preventDefault()
    cancel()
  }
}}
```

## Testing Requirement

Any component with Enter-triggered behavior MUST have a Vitest case
verifying the guard:

```tsx
it('does not save while IME is composing', async () => {
  const onSave = vi.fn()
  // ...render and enter edit mode...
  fireEvent.keyDown(input, {
    key: 'Enter',
    nativeEvent: { isComposing: true } as KeyboardEvent,
  })
  expect(onSave).not.toHaveBeenCalled()
})
```

Without this test, a future refactor can silently re-introduce the bug.

## Enforcement

This rule is enforced by ESLint (`no-restricted-syntax` in
`frontend/eslint.config.js`). The following patterns will fail
`pnpm lint` outside of `frontend/src/utils/keyboard.ts`:

- `e.key === 'Enter'` / `'Enter' === e.key` / `e.key == 'Enter'`
- `e.key === ' '` (and Yoda / `==` variants)
- `keyCode === 13` (and Yoda / `==` variants)
- `keyCode === 229` (and Yoda / `==` variants)
- `*.isComposing` member access (anywhere in the codebase)

If you genuinely need a new variant (e.g. multi-key activation,
modifier-aware shortcuts), **add a new helper to `keyboard.ts`** and use
it from the call site. Do not disable the lint rule.

## Reserved identifier: `isComposing`

The `isComposing` identifier is **reserved for IME composition state**.
The lint rule flags any `*.isComposing` member access codebase-wide,
which intentionally matches more than just `event.nativeEvent.isComposing`.

**Do not use `isComposing` as a property name on unrelated objects**
(e.g. domain models, component state, custom hooks). Pick a different
name. This avoids false-positive lint hits and keeps the identifier's
meaning unambiguous in code search.

## Why This Matters

CJK users press Enter dozens of times per sentence just to confirm IME
conversions. A single un-guarded handler turns the UI into a minefield
for them. This is one of the top-3 i18n bugs in Western-built web apps —
and we will not be one of those apps.
