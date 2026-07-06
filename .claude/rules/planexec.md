# planexec / Case Runtime — Project Knowledge

## Responsibility split (settled — do not re-litigate)
- Case status updates (close, etc.) are the **subagent's responsibility, invoked via a tool** inside planexec. `host` and `planexec` itself MUST NOT carry this as a side effect. If a design gives `host`/`planexec` a case side effect, that design is wrong.
- **materialize** (finalizing case content) is the `host`'s responsibility.
- `planexec` is a generic plan-execute framework and knows nothing about `case` or other domain concepts. Keep domain concepts out of it.

## How the split is wired (implemented)
- The status-change tool is exposed to sub-agents through the `case_status_write` toolset (`agent.ToolSetResolver`), built from `casewriter.NewStatusTool` (which returns ONLY `case__update_case_status` / `case__close_case`, never `case__update_case`). `threadcase.buildToolResolver` wires it for `ModeMention` turns (with `AllowSubAgentWrites=true` and `KnownToolSetIDsThreadWrite`); create turns stay read-only. So "close" is a sub-agent tool call inside the loop, never a host-applied decision. (The original bug was that `buildToolResolver` only wired read-only tools, so the planner routed close through a host `Decision` path that swallowed failures.)
- **Termination is an explicit `finalize` action** on a replan round (`ReplanResult.Finalize`). A replan must set exactly one of `tasks` / `question` / `finalize`; none is rejected and re-planned. The old implicit "empty tasks → done" is gone.
- **Final output is type-safe.** `planexec.Run[T Validatable]` decodes the terminal JSON, calls `T.Validate()`, and regenerates on failure (bounded by `finalOutputMaxRetry`). `RunText` / `ResumeText` are the plain-text variants. The old `RunRequest.OnFinalize` / `FinalOutputSchema` commit hooks are removed; `threadcase` materialize/create is applied by the host from the returned `*T` (no in-loop commit-retry). `Runner.Run` / `Runner.Resume` methods no longer exist — use the package functions.

## Where things live
- `.cckiro` and `.spec` are gitignored (not tracked). Put durable design docs in `docs/develop/` (next to `architecture.md`).

## Live LLM tests
- Regression tests hitting a live LLM are gated by `TEST_*` env vars plus `TEST_LLM_PROVIDER` / `TEST_LLM_MODEL` / `TEST_LLM_*_API_KEY`. Follow the existing patterns in `threadcase_test.go` (`TestThreadCase_MentionClose` / `MentionRespond` / `Creation`, `TestRealLLM_ThreadCaseCreate_VagueToCreate`).
- `zenv` resolves Slack/API tokens through GCP Secret Manager (ADC), so these tests fail inside the sandbox. Run them only with the sandbox disabled and explicit user instruction.

## gollem
- Structured output already has type-safe machinery: `Query[T]`, `ToSchema` (derives schema from a Go type), `queryWithRetry[T]`. Note it validates schema constraints only — it does NOT call a domain `Validate()`.
