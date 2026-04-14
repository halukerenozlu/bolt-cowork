# Vision

## What bolt-cowork is

bolt-cowork is an open-source, terminal-native AI agent platform written in Go. It reads and writes files in a user-defined sandbox, takes tasks in natural language, and executes them through a swappable LLM (Large Language Model) backend.

It takes the core philosophy of Claude Cowork — *"don't just answer, do the work"* — and combines it with Go's strengths: concurrency, single-binary builds, and fast file operations.

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

### v0.2 — Skill system
Load `SKILL.md` files from `~/.bolt-cowork/skills/` and `./bolt-skills/`. YAML frontmatter for metadata. Automatic triggering by description match, manual invocation via `/skill-name`. Skill content injected into the LLM context.

### v0.3 — MCP client
JSON-RPC 2.0 MCP protocol implementation in Go. Stdio and HTTP transports. Server registry via `~/.bolt-cowork/mcp.json`. Initial targets: filesystem and web-search servers.

### v0.4 — Sub-agent coordination
Task decomposition. Parallel execution via Go goroutines. Dependency tracking between sub-tasks. Progress reporting.

### v0.5 — Custom LLM provider
HTTP-based provider for self-hosted or custom-trained models. Benchmark harness for comparing providers.

### v0.6 — User interface
TUI (Bubble Tea), web UI, or desktop (Wails). Decision deferred until v0.5 ships.

## Non-goals

- **Replacing IDEs.** bolt-cowork is a CLI agent, not an editor.
- **Cloud hosting.** It runs on your machine. If you want a hosted version, fork it.
- **A marketplace.** Skills and connectors are plain files. Distribution is a problem for users and communities, not the core project.
- **Supporting every LLM on day one.** Providers land as real users need them, not speculatively.

## How decisions get made

This is a solo-led project in its current phase. Major changes are discussed in GitHub issues before implementation. Roadmap shifts are documented in `CHANGELOG.md` and, for architectural changes, in this file.

As the project grows, governance will formalize. For now: open an issue, make your case, and the maintainer decides.
