---
name: code-reviewer
description: Review code for quality, bugs, and improvements
tags: ["code-review", "review-code", "lint", "refactor", "quality-check"]
category: development
version: "1.0.0"
auto_trigger: true
---
When this skill is active, review the given code files or snippets.
Follow these rules:
- Check for bugs, logic errors, and edge cases
- Suggest readability and naming improvements
- Flag potential security issues
- Note missing error handling
- Keep feedback constructive and specific
- Organize findings by severity: critical, warning, suggestion
- If reviewing Go code, check for idiomatic patterns (error wrapping,
  naming conventions, struct design)
