# Skills

Skills extend Bolt Cowork's behavior for specific tasks. A skill is a `SKILL.md` file with a YAML frontmatter header and a Markdown body that provides instructions to the LLM.

---

## Skill Scopes

Skills are loaded in the following order. A skill with the same name in a later scope overrides earlier ones.

| Scope             | Location                 | Purpose                                         |
| ----------------- | ------------------------ | ----------------------------------------------- |
| **Bundled**       | Built into the binary    | Default skills, always available                |
| **Global**        | `~/.bolt-cowork/skills/` | Your personal skills, available in all projects |
| **Project-local** | `./bolt-skills/`         | Skills specific to one project                  |

---

## Built-in Skills

| Skill                | Description                              |
| -------------------- | ---------------------------------------- |
| `file-organizer`     | Organizes files by type into directories |
| `summarizer`         | Summarizes file contents                 |
| `code-reviewer`      | Reviews code for issues                  |
| `git-helper`         | Helps with git operations                |
| `project-scaffolder` | Creates project structures               |
| `pdf-converter`      | Converts files to PDF                    |

---

## Skill File Format

```yaml
---
name: my-skill
description: What this skill does (used for keyword matching)
auto_trigger: true
tags:
  - files
  - automation
priority: 10
requires_approval: false
---
# Instructions for the LLM

Provide detailed instructions here in Markdown.
The LLM receives this content as additional context when the skill is active.
```

### Frontmatter Fields

| Field               | Required | Default | Description                                  |
| ------------------- | -------- | ------- | -------------------------------------------- |
| `name`              | Yes      | —       | Unique skill identifier                      |
| `description`       | Yes      | —       | Used for keyword matching                    |
| `auto_trigger`      | No       | `false` | Automatically activate based on user command |
| `tags`              | No       | `[]`    | Used for categorization and search           |
| `priority`          | No       | `0`     | Higher priority skills are injected first    |
| `requires_approval` | No       | `false` | Ask for approval before activating           |

---

## Activating Skills

### Automatic

If `auto_trigger: true`, Bolt Cowork matches keywords from the skill's `description` against your command. If there is a match, the skill is activated automatically.

### Manual

Use `/use <skill-name>` to activate a skill for the next task:

```
/use code-reviewer
```

This is a one-shot activation — the skill is automatically deactivated after the task completes.

---

## Creating a Skill

Use the built-in command to create a new skill interactively:

```bash
/skill create
```

Bolt Cowork will prompt you for a name, description, and scope (global or project-local), then generate a `SKILL.md` template.

Or create the file manually in `~/.bolt-cowork/skills/` or `./bolt-skills/`.
