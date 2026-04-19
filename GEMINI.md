# Project: bolt-cowork

## Overview

bolt-cowork is a CLI-based local file agent platform written in Go. It takes natural language commands, creates an execution plan via an LLM, and performs file operations inside a sandboxed directory. Inspired by Claude Cowork.

- **Language:** Go 1.25+
- **Module path:** `github.com/halukerenozlu/bolt-cowork`
- **Current version:** v0.1.4
- **License:** MIT

## Your Role

You serve as both **developer** and **code reviewer** depending on the task. The user will direct you.

When developing: write code, write tests, refactor, fix bugs — same as any Go developer.
When reviewing: analyze code for bugs, security issues, test coverage gaps, and standard violations. Use the review output format below.

## Project Structure

```
bolt-cowork/
├── cmd/bolt-cowork/          # CLI entry point, REPL, init wizard
├── internal/
│   ├── agent/                # Agent loop, planner, executor, approval, levenshtein
│   ├── config/               # YAML config loading and validation
│   ├── mcp/                  # MCP client (v0.3, not yet implemented)
│   ├── provider/             # LLM provider interface + fallback chain
│   ├── sandbox/              # File access restriction, read-only dirs
│   └── skill/                # Skill system (v0.2, not yet implemented)
├── pkg/types/                # Shared types (Message, Role, StepAction)
├── testdata/fixtures/        # Test fixtures and sample configs
├── scripts/                  # Build, test, lint scripts
└── skills/                   # Default SKILL.md files
```

## Coding Standards

- All code, comments, variable names, error messages, and documentation in **English**.
- Error handling: always wrap errors with context using `fmt.Errorf("operation: %w", err)`. Never discard errors with `_` unless explicitly justified.
- No `panic()` in production code. Return errors instead.
- Functions should do one thing. If a function exceeds ~50 lines, consider splitting.
- Use `filepath.Join()` for all path operations. Never concatenate paths with `+` or `fmt.Sprintf`.
- Use ASCII characters in user-facing output. No em-dashes, no Unicode arrows — plain `-`, `->`, `--` only (Windows terminal compatibility).

## Test Rules — CRITICAL

These rules are absolute and must never be violated:

1. **NEVER access real user directories** in tests. No `~`, no `$HOME`, no `os.UserHomeDir()`, no hardcoded paths like `/Users/` or `C:\Users\`.
2. All tests run inside `testdata/` or `t.TempDir()` — nothing else.
3. Test cleanup is mandatory. Use `t.Cleanup()` or `defer`.
4. Tests must be deterministic. No random values, no time-dependent assertions, no network calls.
5. Use table-driven tests for functions with multiple input/output scenarios.
6. Run `go test ./...` before considering any task complete.

## Sandbox Security Model

The sandbox restricts file access. Understanding it is essential for any code changes in `internal/sandbox/` or `internal/agent/executor.go`.

- `allowed_dirs`: directories where the agent can read and write.
- `read_only_dirs`: directories where the agent can read/list but NOT write/delete/move/copy/mkdir.
- `denied_patterns`: glob patterns blocked across all action types.
- Path traversal protection: `resolvePath` joins paths to sandbox root, then verifies the result is inside the sandbox. No prefix stripping. Traversal check uses narrow logic: `rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))` — so `..hidden` directories are not false-positived.

## Action Types

| Action | Description | Dangerous? |
|--------|-------------|------------|
| `read` | Read file contents (truncated at 200 lines) | No |
| `list` | List directory contents | No |
| `write` | Write content to file (rejects empty content) | Yes |
| `delete` | Delete file or directory (recursive requires explicit flag) | Yes |
| `move` | Move/rename file (blocked from read-only source) | Yes |
| `copy` | Copy file (fails if destination exists) | Yes |
| `mkdir` | Create directory (idempotent via MkdirAll) | Yes |

## Approval Modes

| Mode | Behavior |
|------|----------|
| `full` | Every step requires approval, including reads (default) |
| `plan-only` | Only plan stage requires approval |
| `dangerous-only` | Read/list auto-approve; write/delete/move/copy/mkdir/execute require approval |
| `none` | No approvals |

## Architecture Decisions

- `resolvePath` does NOT strip prefixes. If the LLM produces duplicate paths like `workspace/workspace/file.txt`, this is handled via the planner system prompt instruction ("Do NOT repeat the working directory name as a prefix"), not via path manipulation.
- Default approval mode is `full`. Never change this default.
- Read-only directories are automatically added to `allowedDirs` (for read access) but write operations are blocked before the approval gate.
- `ActionCopy` does not overwrite — if destination exists, it returns an error. User must delete first.
- `ActionDelete` on non-empty directories requires `recursive: true`. Without it, returns an error.

## Review Output Format

When reviewing code, structure your output as:

```
## Summary
One paragraph describing what changed and overall assessment.

## Issues

### Critical
- [issue description] — [file:line]

### High
- [issue description] — [file:line]

### Medium
- [issue description] — [file:line]

### Low
- [issue description] — [file:line]

## Suggestions
- [optional improvements]

## Verdict
**APPROVE** or **REQUEST CHANGES**
[one-line justification]
```

Severity guide:
- **Critical:** Security vulnerability, data loss risk, or logic error that breaks core functionality.
- **High:** Bug that affects correctness but does not compromise security.
- **Medium:** Missing test coverage, code smell, or inconsistency with project standards.
- **Low:** Style, naming, or documentation nit.

APPROVE requires zero Critical and zero High issues.

## Review Checklist

When reviewing, always check:
- [ ] No real user directories in tests (no `~`, `$HOME`, hardcoded user paths)
- [ ] Errors wrapped with `%w` and context
- [ ] New behavior has corresponding tests
- [ ] Sandbox boundaries respected (read-only, denied patterns, path traversal)
- [ ] No Turkish in user-facing messages (all English)
- [ ] No Unicode special characters in terminal output (ASCII only)
- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean

## Development vs Runtime

- **Development tools** (Claude Code, Gemini CLI, Codex): used to write bolt-cowork's code. Not part of the final product.
- **Runtime providers** (Anthropic API, OpenAI API): bolt-cowork's brain. Called at runtime when the user gives a task. Swappable via config.

These are completely independent. Do not confuse them.

## Roadmap Context

- v0.1.4 (current): Core agent, sandbox with read-only dirs, 7 action types, approval gates, REPL, typo suggestions.
- v0.1.6 (next): Readline (tab completion, command history), `/config` and `/dir` commands, revise fix.
- v0.1.7: Conversation history, OpenAI and Gemini providers.
- v0.2: Skill system (SKILL.md loading, auto-trigger, context injection).
- v0.3: MCP client (JSON-RPC 2.0, external tool access).
