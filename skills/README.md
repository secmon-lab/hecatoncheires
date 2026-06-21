# Skills

This directory ships [Claude Code](https://claude.com/claude-code) **Skills** that
are useful when working with Hecatoncheires. A Skill is a directory containing a
`SKILL.md` file (with YAML frontmatter) that Claude Code loads on demand when its
`description` matches what you are doing.

## Available skills

| Skill | What it does |
| --- | --- |
| [`hecatoncheires-build-scenario`](./hecatoncheires-build-scenario/SKILL.md) | Author an eval scenario TOML for `hecatoncheires eval`. |

## Installing

Claude Code discovers skills from two locations:

- **Project scope** — `.claude/skills/` in the repository you are working in.
  Available only inside that repository, and shareable with everyone who clones
  it.
- **Personal scope** — `~/.claude/skills/` in your home directory. Available in
  every project you open.

Pick whichever scope fits, then make the skill available there. Run these
commands from the repository root.

### Project scope (recommended for this repo)

```sh
mkdir -p .claude/skills
ln -s "$(pwd)/skills/hecatoncheires-build-scenario" .claude/skills/hecatoncheires-build-scenario
```

A symlink keeps the skill in sync with the version checked into the repository.
If you prefer an independent copy, use `cp -R` instead of `ln -s`:

```sh
mkdir -p .claude/skills
cp -R skills/hecatoncheires-build-scenario .claude/skills/
```

### Personal scope (use everywhere)

```sh
mkdir -p ~/.claude/skills
cp -R skills/hecatoncheires-build-scenario ~/.claude/skills/
```

## Verifying

After installing, start Claude Code and run `/help` (or trigger a task that
matches the skill's `description`). The skill should appear in the available
skills list and activate automatically when relevant.

## Authoring new skills

Add a new directory under `skills/<your-skill-name>/` containing a `SKILL.md`
with frontmatter:

```markdown
---
name: your-skill-name
description: One sentence describing what the skill does and when to use it.
---

# Skill body

Instructions Claude Code follows when the skill activates.
```

Keep `name` in kebab-case and matching the directory name, and write a
`description` that clearly states *when* the skill should trigger — that text is
how Claude Code decides to load it. Then add a row to the table above.
