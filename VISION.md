# Vision

## What bolt-cowork is

bolt-cowork is an open-source, terminal-native AI agent platform written in Go. It reads and writes files in a user-defined sandbox, takes tasks in natural language, and executes them through a swappable LLM (Large Language Model) backend.

It takes the core philosophy of Claude Cowork — _"don't just answer, do the work"_ — and combines it with Go's strengths: concurrency, single-binary builds, and fast file operations.

## Why it exists

Most AI coding assistants assume you live inside an IDE or a chat window. bolt-cowork assumes you live in a terminal. It aims to be the tool you reach for when you want an agent that is:

- **Local-first** — your files stay on your machine.
- **Sandboxed** — the agent can only touch directories you explicitly allow.
- **Provider-agnostic** — today Anthropic, tomorrow OpenAI, Gemini, or your own model.
- **Extensible** — skills and MCP (Model Context Protocol) connectors let you teach the agent new tricks without touching its core.
- **Scriptable** — a single binary that works in pipes, cron jobs, and CI (Continuous Integration) pipelines.

## Design principles

1. **Safety over convenience.** Approval gates, sandboxing, and path-traversal protection are not optional. Defaults err on the side of caution (`approval: full` by default).
2. **One binary, no runtime.** Install means download a binary and run it. No Node, no Python, no Docker.
3. **The LLM is a component, not the product.** Swapping providers is a config change, not a rewrite.
4. **Skills and MCP are the extension points.** Core stays small. Capability grows through markdown files and protocol connectors.
5. **Tests are not optional.** Every behavior that matters has a test. Regressions are caught by CI, not by users.

## Roadmap

The roadmap is versioned. Each version is a self-contained increment with its own success criteria.

### v0.1 — Core agent ✅

Sandbox, config, LLM provider interface with fallback chain, agent loop with approval gates, CLI, Anthropic provider.

### v0.2 — Skill system ✅

Load `SKILL.md` files from `~/.bolt-cowork/skills/` and `./bolt-skills/`. YAML frontmatter for metadata. Automatic triggering by description match, manual invocation via `/skill-name`. Skill content injected into the LLM context.

### v0.3 — MCP client -> Next

JSON-RPC 2.0 MCP protocol implementation in Go. Stdio and HTTP transports. Server registry via `~/.bolt-cowork/mcp.json`. Initial targets: filesystem and web-search servers.

Planned increments:

- **v0.3.0 — Foundation I: Skill + Real Directory:** Skill registry redesign, matcher improvements, default skill updates, and real filesystem sandbox/path/permission/error handling tests.
- **v0.3.1 — Foundation II: Distribution + Contributing:** Cross-platform binaries (.exe / Linux / macOS), automated GitHub Releases upload, sustainable contributing guide, issue/PR template revision, and dev environment setup guide.
- **v0.3.2 — MCP Skeleton I: JSON-RPC + Transport:** JSON-RPC 2.0 core with request IDs, pending requests, notification dispatch, transport interface, and stdio implementation.
- **v0.3.3 — MCP Skeleton II: Types + Registry:** MCP type model (`Tool`, `ToolSchema`, `CallToolResult`), lifecycle (`initialize`, `initialized`, `close`, timeout), server registry, and `~/.bolt-cowork/mcp.json` loader.
- **v0.3.4 — Tool Discovery + Execution:** `tools/list`, `tools/call`, registry integration, and `CallMCPTool` action flow through Provider → Agent → Approval gate → MCP client.
- **v0.3.5 — CLI Integration + Approval:** MCP approval gate compatibility with full/plan-only/dangerous-only/none modes, plus `/mcp list` and `/mcp tools` REPL commands.
- **v0.3.6 — Security:** Allowlist / denylist permission profile and protected MCP config paths so the agent cannot automatically modify `.mcp.json`.
- **v0.3.7 — Stabilization + Tests:** Fake MCP server e2e tests, `resources/list`, `resources/read`, and basic notification event model.

### v0.4 — TUI (Terminal User Interface) (Go)

Terminal user interface with charmbracelet/bubbletea
Real-time task monitoring panel
File browser and directory selector
Skill and MCP server management panel

### v0.5 — Sub-agent Coordination (Go)

Task decomposition: break complex tasks into parts
Parallel task execution via Go goroutines
Dependency tracking between sub-tasks
Progress reporting and error handling

### v0.6 — Custom LLM Provider (Go + Shell)

Support for custom-trained models wrapped with Python + FastAPI
HTTP-based custom provider implementation
Go performance optimizations: large file reading (>100MB), tokenization
Provider benchmark harness

### v0.7 — Desktop App (Go + TypeScript) — if needed

Decision deferred until after v0.6: skipped if TUI is sufficient
Electron desktop app (TypeScript frontend + Go backend)
Real-time task monitoring, file browser, Skill/MCP management panel

## Non-goals

- **Replacing IDEs.** bolt-cowork is a CLI agent, not an editor.
- **Cloud hosting.** It runs on your machine. If you want a hosted version, fork it.
- **A marketplace.** Skills and connectors are plain files. Distribution is a problem for users and communities, not the core project.
- **Supporting every LLM on day one.** Providers land as real users need them, not speculatively.

## How decisions get made

This is a solo-led project in its current phase. Major changes are discussed in GitHub issues before implementation. Roadmap shifts are documented in `CHANGELOG.md` and, for architectural changes, in this file.

As the project grows, governance will formalize. For now: open an issue, make your case, and the maintainer decides.
