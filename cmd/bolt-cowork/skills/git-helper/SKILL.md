---
name: git-helper
description: Help with git commands, branching, and workflows
tags: ["git-commit", "git-branch", "git-merge", "git-rebase", "git-push", "git-pull", "git-stash", "git-log", "git-diff"]
category: development
version: "1.0.0"
auto_trigger: true
---
When this skill is active, assist with git operations.
Follow these rules:
- Suggest the appropriate git command for the task
- Explain what each command does before executing
- For destructive operations (force push, reset --hard, rebase),
  warn the user and explain consequences
- Suggest conventional commit message format:
  type(scope): description
  Types: feat, fix, docs, style, refactor, test, chore
- For merge conflicts, explain the conflict and suggest resolution
- Prefer safe operations when project convention allows: pull --rebase over merge for linear history
