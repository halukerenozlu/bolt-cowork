# Bolt Cowork

A CLI-based local file agent platform inspired by [Claude Cowork](https://claude.com/product/cowork). Give it access to a folder, describe a task in natural language, and it gets the work done.

## Status

**v0.1.2** — Core agent loop, interactive REPL, setup wizard.

## Features (v0.1)

- **Sandbox** — Restricts file access to allowed directories with path validation, denied patterns, and symlink escape protection
- **Config** — YAML configuration with environment variable expansion and validation (`~/.bolt-cowork/config.yaml`)
- **LLM Providers** — Pluggable provider interface with real Anthropic Messages API implementation, fallback chain (auto-switches when a model hits its limit)
- **Agent Loop** — Plan → approve → execute → report cycle with 4-stage approval gates
- **Interactive REPL** — Run `bolt-cowork` without arguments to enter interactive mode with persistent session
- **Init Wizard** — `bolt-cowork init` guides you through first-time setup (provider, API key, model, workspace)
- **Runtime Model Switching** — `/model haiku|sonnet|opus` to switch models during a REPL session

## Quick Start

```bash
# Build
go build -o bolt-cowork ./cmd/bolt-cowork

# First-time setup
./bolt-cowork init

# Set your API key (shown after init)
# PowerShell:
$env:ANTHROPIC_API_KEY = 'sk-ant-...'
# bash/zsh:
export ANTHROPIC_API_KEY='sk-ant-...'

# Interactive mode (recommended)
./bolt-cowork

# Single command mode
./bolt-cowork --dir ./workspace "List all files in this directory"
```

## REPL Commands

| Command           | Description                              |
| ----------------- | ---------------------------------------- |
| `/help`           | Show available commands                  |
| `/model`          | Show current model                       |
| `/model haiku`    | Switch to claude-haiku-4-5 (session only)|
| `/model sonnet`   | Switch to claude-sonnet-4-6 (session only)|
| `/model opus`     | Switch to claude-opus-4-6 (session only) |
| `/quit` or `/exit`| Exit REPL                                |

## Approval Modes

| Mode             | Behavior                                                   |
| ---------------- | ---------------------------------------------------------- |
| `full`           | Pauses at every stage (default)                            |
| `plan-only`      | Pauses only at planning stage                              |
| `dangerous-only` | Pauses only for destructive operations (delete, overwrite) |
| `none`           | Fully automatic                                            |

## Project Structure

```
bolt-cowork/
├── cmd/bolt-cowork/     # CLI entry point, REPL, init wizard
├── internal/
│   ├── agent/           # Agent loop, planner, executor, approval system
│   ├── config/          # YAML config loading and validation
│   ├── mcp/             # MCP client (v0.3)
│   ├── provider/        # LLM provider interface + fallback chain
│   ├── sandbox/         # File access restriction
│   └── skill/           # Skill system (v0.2)
├── pkg/types/           # Shared types (Message, Role)
├── testdata/            # Test fixtures (never uses real user directories)
├── scripts/             # Build, test, lint scripts
└── skills/              # Default SKILL.md files
```

## Configuration

Run `bolt-cowork init` for guided setup, or create `~/.bolt-cowork/config.yaml` manually:

```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    models:
      - claude-sonnet-4-6
  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o

fallback_chain:
  - provider: anthropic
    model: claude-sonnet-4-6
  - provider: openai
    model: gpt-4o

sandbox:
  allowed_dirs:
    - ./workspace
  denied_patterns:
    - "*.env"
    - "*.key"
    - ".ssh/*"

approval_mode: full
```

> **Note:** API keys are stored as environment variable references (`${ANTHROPIC_API_KEY}`), never as plaintext. Set them in your shell before running.

## Development

```bash
go build -o bolt-cowork ./cmd/bolt-cowork   # Build binary
go test ./... -v                             # Run all tests
```

## Roadmap

| Version      | Feature                                                |
| ------------ | ------------------------------------------------------ |
| **v0.1.2**   | ✅ Core agent, sandbox, config, providers, CLI, REPL, init wizard |
| v0.2         | Skill system (SKILL.md loading, auto-trigger)          |
| v0.3         | MCP client (JSON-RPC 2.0, external tool access)        |
| v0.4         | Sub-agent coordination (parallel tasks via goroutines) |
| v0.5         | Custom LLM provider (self-trained model support)       |
| v0.6         | GUI (Web UI with React + Go backend)                   |

## Tech Stack

- **Go 1.25+** — Core agent, CLI, all backend logic
- **Shell** — Build/test automation
- **TypeScript** — GUI (v0.6, planned)

## License

TBD
