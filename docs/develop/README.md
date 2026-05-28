# Development

This directory holds material for human contributors working on
Hecatoncheires: the reasoning behind internal design and how to extend it. If
you are setting up the project locally, start with
[Getting Started](../getting_started.md).

## Where each kind of guidance lives

Hecatoncheires keeps three distinct kinds of "rules and context" in three
distinct places. Put new material where it belongs:

| Location | Audience | Nature |
|---|---|---|
| [`.claude/rules/*.md`](../../.claude/rules/) | Claude / agents | Enforced rules — breaking them breaks the system (architecture invariants, repository write contract, keyboard/IME policy, …) |
| [`CLAUDE.md`](../../CLAUDE.md) (root) | Claude / all agents | Project-wide AI context, kept lightweight |
| `docs/develop/` (this dir) | Human contributors | "Why we designed it this way" and "how to extend it" |

When you are unsure: if violating it would corrupt data or behavior, it is a
rule (`.claude/rules/`). If it is background an agent should always carry, it is
`CLAUDE.md`. If it is explanatory prose for a person, it belongs here.

## Contents

- [Architecture (internals)](architecture.md) — the "why/how" of internal
  design: GraphQL DataLoader (request-scoped batching) and the Agent thread
  session implementation (Cloud Storage layout, IAM, turn-lock / state
  persistence).

## See Also

- [Concepts](../concepts.md) — product vocabulary
- [Configuration](../configuration.md) and [CLI Reference](../cli.md)
- Root [README](../../README.md) and [CLAUDE.md](../../CLAUDE.md)
