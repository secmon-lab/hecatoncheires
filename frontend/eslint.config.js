// Minimal flat config. We intentionally do NOT enable the recommended
// ruleset — that would surface unrelated noise. The sole purpose of this
// config (for now) is to enforce the IME / Enter handling policy:
// direct keyboard introspection is forbidden outside `src/utils/keyboard.ts`.
//
// See `.claude/rules/frontend-keyboard-input.md` for the policy details.
import tsParser from '@typescript-eslint/parser'

// We use `operator=/^={2,3}$/` so both `===` and `==` are caught, and we
// cover both `right.value` and `left.value` so Yoda-style comparisons
// (`'Enter' === e.key`) cannot slip through.
const eqOp = "[operator=/^={2,3}$/]"

const restrictedKeyboard = [
  {
    selector: `BinaryExpression${eqOp}:matches([right.value='Enter'], [left.value='Enter'])`,
    message:
      "Direct 'Enter' key detection is forbidden. Use commitOnEnter / activateOnEnterOrSpace from src/utils/keyboard. Direct detection breaks IME (CJK) users.",
  },
  {
    selector: `BinaryExpression${eqOp}:matches([right.value=' '], [left.value=' '])`,
    message:
      "Direct Space-key detection is forbidden. Use activateOnEnterOrSpace from src/utils/keyboard.",
  },
  {
    selector: `BinaryExpression${eqOp}:matches([right.value=13], [left.value=13])`,
    message:
      'Direct keyCode === 13 detection is forbidden. Use commitOnEnter / activateOnEnterOrSpace from src/utils/keyboard.',
  },
  {
    selector: `BinaryExpression${eqOp}:matches([right.value=229], [left.value=229])`,
    message:
      'Direct keyCode === 229 detection is forbidden. IME composition checks must live inside src/utils/keyboard.ts only.',
  },
  {
    // NOTE: this matches *any* `.isComposing` property access. The `isComposing`
    // identifier is reserved for IME composition state across this codebase —
    // do not use it for other purposes (see frontend-keyboard-input.md).
    selector: "MemberExpression[property.name='isComposing']",
    message:
      'Direct isComposing access is forbidden. Use commitOnEnter / activateOnEnterOrSpace from src/utils/keyboard.',
  },
]

export default [
  {
    ignores: [
      'dist/**',
      'build/**',
      'node_modules/**',
      'coverage/**',
      'playwright-report/**',
      'test-results/**',
      'e2e/**',
    ],
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
        ecmaFeatures: { jsx: true },
      },
    },
    rules: {
      'no-restricted-syntax': ['error', ...restrictedKeyboard],
    },
  },
  {
    // The keyboard helper module is the *only* place allowed to inspect
    // raw keyboard / IME state. Disable the rule here so the helper itself
    // can implement the guard logic.
    files: ['src/utils/keyboard.ts', 'src/utils/keyboard.test.ts'],
    rules: {
      'no-restricted-syntax': 'off',
    },
  },
]
