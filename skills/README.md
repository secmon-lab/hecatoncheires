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

These skills are published through this repo's plugin marketplace
(`.claude-plugin/marketplace.json`), so the supported way to install them is via
Claude Code's plugin system. From inside Claude Code:

```text
/plugin marketplace add secmon-lab/hecatoncheires
/plugin install hecatoncheires-build-scenario@hecatoncheires
```

The first command registers this repository as a marketplace; the second installs
the skill from it. Run `/plugin` to manage installed plugins.

## Authoring new skills

1. Add a directory under `skills/<your-skill-name>/` containing a `SKILL.md`
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
   `description` that clearly states *when* the skill should trigger — that text
   is how Claude Code decides to load it.

2. Register the skill as a plugin in `.claude-plugin/marketplace.json` (add an
   entry under `plugins` with `name`, `source`, `description`, `version`) so it
   can be installed via `/plugin install`.

3. Add a row to the **Available skills** table above.
