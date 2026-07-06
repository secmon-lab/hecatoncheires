# planexec / Case Runtime — Project Knowledge

## Responsibility split (settled — do not re-litigate)
- Case status updates (close, etc.) are the **subagent's responsibility, invoked via a tool** inside planexec. `host` and `planexec` itself MUST NOT carry this as a side effect. If a design gives `host`/`planexec` a case side effect, that design is wrong.
- **materialize** (finalizing case content) is the `host`'s responsibility.
- `planexec` is a generic plan-execute framework and knows nothing about `case` or other domain concepts. Keep domain concepts out of it.
- Root cause of the "close reported but not applied" bug: `threadcase`'s `buildToolResolver` wires only read-only Knowledge and never connects the casewriter tool set. Casewriter tools: `case__update_case_status` (thread-mode) / `case__close_case` (channel-mode) / `case__update_case`. `ToolSetResolver` currently exposes only read-only sets like `core_ro` / `slack_ro`.
- Known design flaw: `planexec` termination is an implicit fallback at `plan.go:70-73` ("Tasks and Question both empty → done"). An explicit terminal action is the decided direction.

## Where things live
- `.cckiro` and `.spec` are gitignored (not tracked). Put durable design docs in `docs/develop/` (next to `architecture.md`).

## Live LLM tests
- Regression tests hitting a live LLM are gated by `TEST_*` env vars plus `TEST_LLM_PROVIDER` / `TEST_LLM_MODEL` / `TEST_LLM_*_API_KEY`. Follow the existing patterns in `threadcase_test.go` (`TestThreadCase_MentionClose` / `MentionRespond` / `Creation`, `TestRealLLM_ThreadCaseCreate_VagueToCreate`).
- `zenv` resolves Slack/API tokens through GCP Secret Manager (ADC), so these tests fail inside the sandbox. Run them only with the sandbox disabled and explicit user instruction.

## gollem
- Structured output already has type-safe machinery: `Query[T]`, `ToSchema` (derives schema from a Go type), `queryWithRetry[T]`. Note it validates schema constraints only — it does NOT call a domain `Validate()`.
