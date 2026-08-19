# planexec / Case Runtime — Project Knowledge

## Responsibility split (settled — do not re-litigate)
- Case mutations requested through a `case__*` tool call (status change, assignee change, content edit) are the **subagent's responsibility** inside planexec. `host` and `planexec` itself MUST NOT carry them as a side effect. If a design gives `host`/`planexec` a case side effect on that path, that design is wrong.
- The terminal `materialize` decision is the one **explicit exception**: it also writes case content, and the `host` applies it after the turn. So "who writes the case" is decided by the path, not by the field: tool call → subagent, terminal decision → host. See the note on picking one content path per turn below.
- `planexec` is a generic plan-execute framework and knows nothing about `case` or other domain concepts. Keep domain concepts out of it.

## How the split is wired (implemented)
- Case writes are exposed to sub-agents through the `case_write` toolset (`agent.ToolSetResolver`), built from `casewriter.New` — the **full** set: `case__update_case`, `case__assign`, `case__unassign`, and the mode-appropriate `case__update_case_status` / `case__close_case`. `threadcase` advertises it on `ModeMention` turns only (`KnownToolSetIDsThreadWrite` + `AllowSubAgentWrites=true`); `ModeMaterialize` stays read-only and `ModeCreate` has no case yet, so `buildToolResolver` leaves `CaseWrite` zero and builds nothing. So closing / assigning / editing are all sub-agent tool calls inside the loop, never host-applied decisions. (The original bug was that `buildToolResolver` only wired read-only tools, so the planner routed close through a host `Decision` path that swallowed failures. A later one was that only the status tool was wired, so a mention asking the agent to fill the empty assignee field ended with the agent reporting it had no such capability.)
- **There is deliberately no status-only or assignee-only subset.** Any host granting case writes grants all of them. Restricting a mention agent's write palette below the full set was found to produce exactly one outcome: the agent tells the user it cannot do what it was asked.
- **Two content paths coexist, and the prompt picks between them.** `case__update_case` (in-loop) and the terminal `materialize` decision both write title / description, and `materialize` is a **full replacement** — so a sub-agent edit followed by a `materialize` in the same turn loses the edit. The threadcase `ModeMention` system prompt instructs the planner to pick one path per turn. That is a prompt-only guardrail, not a code lock.
- **Termination is an explicit `finalize` action** on a replan round (`ReplanResult.Finalize`). A replan must set exactly one of `tasks` / `question` / `finalize`; none is rejected and re-planned. The old implicit "empty tasks → done" is gone.
- **Final output is type-safe.** The strategy decodes the terminal JSON into the host's `Config[T]` type, calls `T.Validate()`, and regenerates on failure (bounded by `finalOutputMaxRetry`). Plain-text hosts use `Config[T]{TextOnly: true}` with `T = TextResult`. The in-process `Runner` and its `Run` / `RunText` / `ResumeText` entry functions no longer exist — `planexec.Register` is the only way in.
- **Host finalizers validate the final output against host context inside the regeneration loop.** `Run[T]` takes optional variadic `finalizers ...func(*T) error`. Each runs after `T.Validate()` on the decoded final output; a returned error is fed back to the model and the output regenerated (same bound as `Validate()` failures). They exist because `T.Validate()` is a pure method that cannot see host-only context (e.g. a workspace field schema known only to the caller). A finalizer MUST be **side-effect-free** — a later attempt re-runs every one; committing the output happens after the turn, never in a finalizer. planexec stays domain-agnostic — it only calls `func(*T) error`.
  - **ModeCreate** wires a *validation-only* finalizer (`runCreateTurn`): it checks the proposed fields against the workspace schema (a non-RFC3339 `due_date`, a missing required field, an out-of-schema option), so such an error is fed back and the decision regenerated instead of killing the turn with no feedback. This preserves the "no in-loop commit-retry" stance: the case is committed by `Handler.Create` **after** the turn, and a persistence failure there is surfaced and falls back — it is NOT fed back to the model, which cannot repair an infrastructure error (e.g. a write conflict) by re-emitting the same JSON. The field/validation error (model's fault, model-fixable) and the persistence error (infra's fault, not model-fixable) deliberately take different paths.
  - **Mention-mode materialize** is likewise applied by the host from the returned `*T` **after** the turn (no finalizer) — it updates an already-existing case.

## On agentkit (the Strategy)

The same runtime exists a second time as an agentkit Strategy
(`planexec.Register` in `strategy.go`), which is what the migrated hosts run on.
Differences worth knowing before changing it:

- **One transition is one LLM call or one tool call.** The phases are
  `plan` → `collect` → `replan` → `final`, plus `planner_tool` for a tool call the
  planner asked for before deciding. A retry (a rejected plan, a rejected terminal
  output) is a fresh transition, never a loop inside one.
- **The planner may call tools.** When a planning call returns FunctionCalls
  instead of a decision, the run diverts to `planner_tool`, runs ONE of them per
  transition, and comes back. It is bounded by `plannerToolRoundsMax` per planning
  phase, because those calls are free of the round budget — nothing else would stop
  a model that only ever looks things up. The bound is a NUDGE, not a gate: past it
  the calls are still run and answered (dropping them breaks the conversation), and
  what changes is that the planner is told to stop — through the SYSTEM prompt
  (`plannerPrompt`), because the call that has to hear it sends no user turn at all.
  Each call is answered in the conversation by `Session().CallTool`, and the
  planning call that follows sends NOTHING — no restated request, no instruction.
  See `.claude/rules/architecture.md` § "a parallel tool-call turn is answered in
  ONE call" for both halves of that rule and the rejections that taught them.
- **The terminal call may ask for tools too, and is treated the same way.**
  agentkit's managed session declares the claim's tools on EVERY call, so a model
  that spent the run calling tools can answer the terminal call with another call
  instead of the answer. `stepFinal` diverts to the tool phase and comes back
  sending no user turn, since the conversation already carries the results; the
  final prompt rides in that call's SYSTEM prompt instead, because a budget notice
  can route plan / replan straight to final and make this the only terminal call
  the run ever makes — dropping it would leave the model never told to write the
  answer (`TestPlannerToolResultsReachTheTerminalCall`). Reading a function-call reply as "the
  final response was empty" ends the turn in a fallback with nothing to say, after
  the investigation has already been paid for — pinned by
  `TestTerminalCallMayAskForATool`. The wrap-up path needs the same care for the
  opposite reason: a budget notice sends plan / replan straight to final, so the
  terminal call is where an answered tool round is continued from
  (`TestPlannerToolResultsReachTheTerminalCall`).
- **A question ENDS the turn by default** (`Output.Kind == OutputQuestion`) rather
  than waiting on an await. Holding a run open while a person takes hours would pin
  its subject and block every later turn on that thread; the answer arrives as a
  fresh run. A host whose own record spans the wait sets `Input.SuspendOnQuestion`
  instead: the run parks on a question await, `Config.Asker` delivers the question
  with the key the answer must reach, and `Kernel.Respond` continues the SAME
  Process — one budget, one history, one run id. Only the interactive Job uses it,
  because only its run record covers the whole exchange.
- **`Config[T].Finalizers` take the run's Process metadata**, not just the output:
  a strategy is registered once at startup and then serves every run, so a
  finalizer must read its own scope (the workspace, the case) rather than close
  over one.
- **`Progress` is stateless.** The message id and the lines so far live in the
  checkpointed state, so a run picked up by another instance keeps drawing into
  the same Slack message instead of starting a second one.

## Where things live
- `.cckiro` and `.spec` are gitignored (not tracked). Put durable design docs in `docs/develop/` (next to `architecture.md`).

## Live LLM tests
- Regression tests hitting a live LLM are gated by `TEST_*` env vars plus `TEST_LLM_PROVIDER` / `TEST_LLM_MODEL` / `TEST_LLM_*_API_KEY`. Follow the existing patterns in `threadcase_test.go` (`TestThreadCase_MentionClose` / `MentionRespond` / `Creation`, `TestRealLLM_ThreadCaseCreate_VagueToCreate`).
- `zenv` resolves Slack/API tokens through GCP Secret Manager (ADC), so these tests fail inside the sandbox. Run them only with the sandbox disabled and explicit user instruction.

## gollem
- Structured output already has type-safe machinery: `Query[T]`, `ToSchema` (derives schema from a Go type), `queryWithRetry[T]`. Note it validates schema constraints only — it does NOT call a domain `Validate()`.
