# Bolt Cowork

**Terminal-native File Agent Platform**

Bolt Cowork is an open-source agent platform that accesses files on your computer, receives tasks through natural language commands, and solves them using the LLM provider of your choice.

> "Don't just answer - do the work."

---

## What Does It Do?

You give Bolt Cowork a natural language command. It creates a plan, asks for your approval, and executes the task step by step - reading, writing, and organizing files on your behalf.

```bash
bolt-cowork --dir ./my-project "Summarize all markdown files and create an index"
```

---

## Key Features

- **Sandboxed file access** - only operates within the directory you specify
- **TUI-first workflow** - Bubble Tea interface with command palette, modals, live status, chat viewport, and streaming output
- **Connection wizard** - step-by-step provider setup with auth method selection, key verification, and model discovery
- **Broad provider support** - Anthropic, OpenAI, Gemini, plus OpenAI-compatible hosted presets (OpenRouter, DeepSeek, Mistral, Groq) and local models (Ollama, LM Studio)
- **Secure setup** - API keys stored in the system keyring, not in config files
- **Approval gates** - review plans, revise them, approve execution steps, or approve all from the TUI
- **Skill system** - extend behavior with custom `SKILL.md` files
- **MCP client** - connects to external tools via the MCP (Model Context Protocol) standard
- **Single binary** - no runtime dependencies, just download and run

---

## Current Version

**v0.4.5** - TUI feedback fixes: no duplicate plan/result output for single-step runs, a welcome-screen connection wizard that matches the session screen, credential replace/remove with a persisted "provider selection required" state, live slash-command suggestions with Tab completion, and one-entry-per-line directory listings.

[See the full version history ->](getting-started.md)

---

## Quick Start

```bash
# Download the binary for your platform
# (see Getting Started for full instructions)

bolt-cowork --dir ./workspace "List all files and summarize their contents"
```

[Getting Started ->](getting-started.md)
