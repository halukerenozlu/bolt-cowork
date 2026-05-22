# Project: bolt-cowork

## Overview

bolt-cowork is a Terminal-native File Agent Platform written in Go. It takes natural language commands, creates an execution plan via an LLM, and performs file operations inside a sandboxed directory. Inspired by Claude Cowork.

- **Language:** Go 1.26+
- **Module path:** `github.com/halukerenozlu/bolt-cowork`
- **Current version:** v0.4.1
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
│   │   ├── actions/call_mcp_tool.go     # CallMCPToolAction
│   │   └── actions/read_mcp_resource.go # ReadMCPResourceAction
│   ├── config/               # YAML config loading and validation
│   ├── mcp/                  # MCP client, transport, registry (v0.3)
│   │   ├── types.go          # MCP type model (Tool, ToolSchema, CallToolResult, Initialize*)
│   │   ├── loader.go         # LoadConfig, DefaultConfigPath, expandTilde
│   │   ├── normalize.go      # NormalizeConfig: trim, validate, dedup
│   │   ├── registry.go       # Registry: AddServer, GetTool, LoadFromConfig, LoadFromFile
│   │   ├── tool_registry.go  # ToolRegistry: composite serverName/toolName key
│   │   ├── resource_types.go # MCP resource wire types (v0.3.7)
│   │   ├── resource_registry.go # ResourceRegistry (v0.3.7)
│   │   ├── notification.go   # NotificationRegistry (v0.3.7)
│   │   ├── jsonrpc.go        # JSON-RPC 2.0 core
│   │   ├── transport.go      # Transport interface
│   │   ├── stdio.go          # StdioTransport with cancellable locks
│   │   ├── process.go        # StartProcess helper
│   │   └── testutil/         # Mock MCP server + fakeserver e2e helpers (v0.3.7)
│   ├── ui/                   # Terminal user interface (v0.4+)
│   │   ├── app.go            # Root App model, view switching (Welcome → Session)
│   │   ├── keys/keymap.go    # Quit and palette key bindings
│   │   ├── theme/theme.go    # Centralized lipgloss color and style definitions
│   │   ├── views/welcome.go  # Welcome screen — centered title, text input, git branch + version status bar
│   │   ├── views/session.go  # Split layout placeholder (70% chat / 30% status)
│   │   ├── panels/           # chat.go, status.go, input.go (bubbles/textinput), statusbar.go
│   │   └── widgets/          # spinner.go (bubbles/spinner), plan.go (glamour fallback), approval.go, palette.go
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
| `call_mcp_tool` | Call a registered MCP tool after approval            | Yes        |

## REPL Features

- **TUI** powered by charmbracelet/bubbletea — welcome screen, split session layout (70% chat / 30% status). Readline removed in v0.4.0.
- **Commands:** /help, /quit, /model, /key, /config, /config path, /config reload, /dir, /dir <path>, /clear.
- **Conversation history:** multi-turn context, 20-turn FIFO cap.
- **Plan revision:** user can revise plans with feedback, max 3 attempts.

## Approval Modes

| Mode             | Behavior                                                                      |
| ---------------- | ----------------------------------------------------------------------------- |
| `full`           | Every step requires approval, including reads (default)                       |
| `plan-only`      | Only plan stage requires approval                                             |
| `dangerous-only` | Read/list auto-approve; write/delete/move/copy/mkdir/execute require approval |
| `none`           | No approvals                                                                  |

- When `--mcp-approval` is not set, MCP tool calls obey the global approval mode (same as all other tools)
- When `--mcp-approval` is set, MCP tool calls use the MCP-specific gate instead of the global gate
- `full` in MCP gate context means: prompt before every MCP tool call

## MCP v0.3.7 Capabilities

**MCP Resources (v0.3.7+):**

- `Client.DiscoverResources(ctx)` calls `resources/list` on connected servers and stores results in `ResourceRegistry`
- `Client.ReadResource(ctx, serverName, uri)` calls `resources/read` and returns resource contents
- `ResourceRegistry` stores discovered resources per server with thread-safe replacement and lookup
- `ReadMCPResourceAction` adds `read_mcp_resource` support to the planner/executor action flow

**MCP Notifications (v0.3.7+):**

- `NotificationRegistry` uses a method-to-callback map and recovers/logs panicking handlers
- Built-in handlers are separate from user handlers so stale flag behavior cannot be overwritten
- `notifications/resources/updated` sets `resourcesStale`; `notifications/tools/list_changed` sets `toolsStale`
- `ConnectAndInitialize(ctx, name, transport)` combines connection setup with the initialize handshake and `notifications/initialized`

**E2E Test Infrastructure (v0.3.7+):**

- `internal/mcp/testutil/mock_server.go` provides an in-process mock server for unit-style MCP tests
- `internal/mcp/testutil/fakeserver/main.go` provides the stdio fakeserver binary for subprocess e2e tests
- `internal/mcp/e2e_test.go` uses a `TestMain` pattern to build the fakeserver in a temp directory and clean it up

## Architecture Decisions

- `resolvePath` does NOT strip prefixes. Duplicate paths handled via planner prompt.
- Default global approval mode is `full`. Never change this default.
- Read-only directories auto-added to `allowedDirs` for read access; writes blocked before approval gate.
- `ActionCopy` does not overwrite. `ActionDelete` on non-empty dirs requires `recursive: true`.
- TUI is powered by bubbletea; `ui.New(cfg, version).Run()` is the entry point from `cmd/bolt/main.go`.

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
- v0.3.0 -- Skill system revision + real directory hardening
- v0.3.1 -- Cross-platform binary + contributing guide
- v0.3.2 (complete) -- JSON-RPC 2.0 core (`jsonrpc.go`), Transport interface (`transport.go`), StdioTransport with cancellable locks (`stdio.go`), StartProcess helper (`process.go`) -- 78 tests passing
- v0.3.3 (complete) -- MCP type model (`types.go`), config loader (`loader.go`), normalizer (`normalize.go`), registry extended (`LoadFromConfig`, `LoadFromFile`) -- 174 tests passing
- v0.3.4 (complete) -- Tool discovery, CallMCPToolAction, approval gate, provider schema injection -- 210+ tests passing
- v0.3.5 (complete) -- MCP approval gate + /mcp REPL commands
- v0.3.6 (complete) -- Allowlist/denylist permission profiles (`PermissionProfile`, `LoadPermissions`), `~/.bolt-cowork/mcp.json` protected path
- v0.3.7 (complete) -- E2E test infrastructure, MCP resources, notification event model
- v0.4.0 (complete) -- TUI foundation: bubbletea + lipgloss + bubbles + glamour, welcome screen, split layout skeleton, readline removed
- v0.4.1 (complete) -- Agent integration, streaming, spinner, plan viewer ([ ]/[+]/[x]), exec log, right panel live, command palette (Ctrl+P), REPL commands migrated to palette
- v0.4.2 (planned) -- MCP tool call visualization, permission warnings, skill status panel; Git status bar, theme support, keyboard shortcuts finalize
- v0.5 Sub-agent coordination (parallel tasks via goroutines) Go + Shell
- v0.6 Custom LLM provider (self-trained model support) Go + Shell
- v0.7 Desktop App — if needed (if TUI is insufficient)
