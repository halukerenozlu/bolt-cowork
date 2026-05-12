# Project: bolt-cowork

## Overview

bolt-cowork is a CLI-based local file agent platform written in Go. It takes natural language commands, creates an execution plan via an LLM, and performs file operations inside a sandboxed directory. Inspired by Claude Cowork.

- **Language:** Go 1.26+
- **Module path:** `github.com/halukerenozlu/bolt-cowork`
- **Current version:** v0.3.0
- **License:** MIT
- **Spec:** `/spec/bolt-cowork-project-spec-EN.md`

## Your Role

You serve as both **developer** and **code reviewer** depending on the task. The user will direct you.

When developing: write code, write tests, refactor, fix bugs.
When reviewing: analyze code for bugs, security issues, test coverage gaps, and standard violations. Use the review output format below.

## Project Structure

```
bolt-cowork/
├── cmd/bolt-cowork/          # CLI entry point, REPL, init wizard
├── internal/
│   ├── agent/                # Agent loop, planner, executor, approval, levenshtein
│   ├── config/               # YAML config loading and validation
│   ├── mcp/                  # MCP client (v0.3, not yet implemented)
│   ├── tool/                 # Tool definitions and helpers
│   ├── prompt/               # Prompt templates and helpers
│   ├── provider/             # LLM provider interface + fallback chain
│   ├── sandbox/              # File access restriction, read-only dirs
│   └── skill/                # Skill system (v0.2, next) — skill.go, loader.go, matcher.go, injector.go
├── pkg/types/                # Shared types (Message, Role, StepAction)
├── testdata/fixtures/        # Test fixtures and sample configs
└── skills/                   # Default SKILL.md files
```

## Coding Standards

- All code, comments, variable names, error messages, and documentation in **English**.
- Error handling: always wrap errors with context using `fmt.Errorf("operation: %w", err)`. Never discard errors with `_` unless explicitly justified.
- No `panic()` in production code. Return errors instead.
- Functions should do one thing. If a function exceeds ~50 lines, consider splitting.
- Use `filepath.Join()` for all path operations. Never concatenate paths with `+` or `fmt.Sprintf`.
- Use ASCII characters in user-facing output. No em-dashes, no Unicode arrows -- plain `-`, `->`, `--` only (Windows terminal compatibility).

## Test Rules -- CRITICAL

1. **NEVER access real user directories** in tests. No `~`, no `$HOME`, no `os.UserHomeDir()`.
2. All tests run inside `testdata/` or `t.TempDir()`.
3. Test cleanup is mandatory. Use `t.Cleanup()` or `defer`.
4. Tests must be deterministic. No random values, no time-dependent assertions, no network calls.
5. Use table-driven tests for functions with multiple input/output scenarios.
6. Run `go test ./...` before considering any task complete.

## Sandbox Security Model

- `allowed_dirs`: directories where the agent can read and write.
- `read_only_dirs`: directories where the agent can read/list but NOT write/delete/move/copy/mkdir.
- `denied_patterns`: glob patterns blocked across all action types.
- Path traversal protection: narrow check using `rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))`.

## Action Types

| Action   | Description                                                 | Dangerous? |
| -------- | ----------------------------------------------------------- | ---------- |
| `read`   | Read file contents (truncated at 200 lines)                 | No         |
| `list`   | List directory contents                                     | No         |
| `write`  | Write content to file (rejects empty content)               | Yes        |
| `delete` | Delete file or directory (recursive requires explicit flag) | Yes        |
| `move`   | Move/rename file (blocked from read-only source)            | Yes        |
| `copy`   | Copy file (fails if destination exists)                     | Yes        |
| `mkdir`  | Create directory (idempotent via MkdirAll)                  | Yes        |

## REPL Features

- **Readline** via `chzyer/readline` -- tab completion, command history, line editing.
- **Commands:** /help, /quit, /model, /key, /config, /config path, /config reload, /dir, /dir <path>, /clear.
- **Conversation history:** multi-turn context, 20-turn FIFO cap.
- **Plan revision:** user can revise plans with feedback, max 3 attempts.
- **Fallback mode:** bufio-based REPL when readline fails (piped stdin).

## Approval Modes

| Mode             | Behavior                                                                      |
| ---------------- | ----------------------------------------------------------------------------- |
| `full`           | Every step requires approval, including reads (default)                       |
| `plan-only`      | Only plan stage requires approval                                             |
| `dangerous-only` | Read/list auto-approve; write/delete/move/copy/mkdir/execute require approval |
| `none`           | No approvals                                                                  |

## Architecture Decisions

- `resolvePath` does NOT strip prefixes. Duplicate paths handled via planner prompt.
- Default approval mode is `full`. Never change this default.
- Read-only directories auto-added to `allowedDirs` for read access; writes blocked before approval gate.
- `ActionCopy` does not overwrite. `ActionDelete` on non-empty dirs requires `recursive: true`.
- All user input in REPL reads through readline instance (single input source). No separate bufio.Reader for stdin.

## Review Output Format

```
## Summary
One paragraph.

## Issues
### Critical / High / Medium / Low
- [description] -- [file:line]

## Suggestions
- [optional]

## Verdict
**APPROVE** or **REQUEST CHANGES**
[justification]
```

APPROVE requires zero Critical and zero High issues.

## Review Checklist

- [ ] No real user directories in tests
- [ ] Errors wrapped with `%w` and context
- [ ] New behavior has corresponding tests
- [ ] Sandbox boundaries respected
- [ ] No Turkish in user-facing messages
- [ ] No Unicode special characters in terminal output (ASCII only)
- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean

## Roadmap Context

- v0.1.8: Bug fixes (signal handling, sandbox, provider fallback, tilde expansion).
- v0.2.0: Skill system -- SKILL.md loading, keyword matching, prompt injection, /use manual activation.
- v0.2.3: Context trimming, /dir workspace switching, global skill warnings, security fixes.
- v0.2.4: SkillMetadata, SkillScope enum, frontmatter parser, system prompt builder, tool registry.
- v0.2.5: Security + quality tests: redaction, protected paths, permission reasons, e2e scenarios, skill parser, MCP config validation.
- v0.2.6: Stabilization — Windows security hardening, reserved filenames, write size limit, error style, banner fix, startup sequence polish.
- v0.3.0 (current) -- Skill system revision + real directory hardening
- v0.3.1 (next) -- Distribution + MCP skeleton
- v0.4 TUI (charmbracelet/bubbletea terminal interface) Go
- v0.5 Sub-agent coordination (parallel tasks via goroutines) Go + Shell
- v0.6 Custom LLM provider (self-trained model support) Go + Shell
- v0.7 Desktop App — if needed (if TUI is insufficient)
