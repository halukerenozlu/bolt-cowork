# Bolt Cowork

**Terminal-native File Agent Platform**

Bolt Cowork is an open-source agent platform that accesses files on your computer, receives tasks through natural language commands, and solves them using an LLM (Large Language Model) of your choice.

> "Don't just answer — do the work."

---

## What Does It Do?

You give Bolt Cowork a natural language command. It creates a plan, asks for your approval, and executes the task step by step — reading, writing, and organizing files on your behalf.

```bash
bolt-cowork --dir ./my-project "Summarize all markdown files and create an index"
```

---

## Key Features

- **Sandboxed file access** — only operates within the directory you specify
- **Provider-agnostic** — works with OpenAI, Anthropic, or your own LLM
- **Fallback chain** — automatically switches to the next provider if one fails
- **Skill system** — extend behavior with custom `SKILL.md` files
- **MCP client** — connects to external tools via the MCP (Model Context Protocol) standard
- **Terminal UI** — built with Bubbletea, no browser required
- **Single binary** — no runtime dependencies, just download and run

---

## Current Version

**v0.4.2** — TUI (Terminal User Interface) with command palette, live status panel, git integration, and streaming output.

[See the full version history →](getting-started.md)

---

## Quick Start

```bash
# Download the binary for your platform
# (see Getting Started for full instructions)

bolt-cowork --dir ./workspace "List all files and summarize their contents"
```

[Getting Started →](getting-started.md)
