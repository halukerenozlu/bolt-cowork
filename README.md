# Bolt Cowork

A CLI-based local file agent platform inspired by [Claude Cowork](https://claude.com/product/cowork). Give it access to a folder, describe a task in natural language, and it gets the work done.

## Status

**v0.1.3** — Core agent loop, interactive REPL, setup wizard, runtime model/key management.

## Features (v0.1)

- **Sandbox** — Restricts file access to allowed directories with path validation, denied patterns, and symlink escape protection
- **Config** — YAML configuration with validation (`~/.bolt-cowork/config.yaml`), auto-created on first run
- **LLM Providers** — Pluggable provider interface with real Anthropic Messages API, fallback chain (auto-switches when a model hits its limit)
- **Agent Loop** — Plan → approve → execute → report cycle with 4-stage approval gates
- **Interactive REPL** — Run `bolt-cowork` to enter interactive mode with persistent session
- **Auto Setup** — First run automatically starts the setup wizard (provider, API key, model, workspace)
- **Runtime Model Switching** — `/model haiku|sonnet|opus` to switch models during a session
- **API Key Management** — `/key` to view, `/key set` to change API keys without leaving REPL

## Quick Start

```bash
# Clone and install
git clone https://github.com/halukerenozlu/bolt-cowork.git
cd bolt-cowork
go install ./cmd/bolt-cowork

# Run (first time: auto setup, then REPL)
bolt-cowork
```

That's it. On first run, the setup wizard will guide you through provider selection, API key, model, and workspace configuration.

## REPL Commands

| Command              | Description                                |
| -------------------- | ------------------------------------------ |
| `/help`              | Show available commands                    |
| `/model`             | Show current model                         |
| `/model <name>`      | Switch model: haiku, sonnet, opus          |
| `/key`               | Show current API key (masked)              |
| `/key set`           | Change API key for active provider         |
| `/key <provider>`    | Show API key for specific provider         |
| `/key set <provider>`| Change API key for specific provider       |
| `/quit`              | Exit REPL                                  |

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
├── cmd/bolt-cowork/     # CLI entry point, REPL, init wizard, key/model management
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

Run `bolt-cowork` for the first time for guided setup, or run `bolt-cowork init` to reconfigure. Config file is stored at `~/.bolt-cowork/config.yaml`:

```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: your-api-key-here
    models:
      - claude-sonnet-4-6
  openai:
    api_key: your-api-key-here
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

## Single Command Mode

You can also run a single command without entering the REPL:

```bash
bolt-cowork --dir ./workspace "List all files in this directory"
bolt-cowork --provider anthropic --approval none "Organize files by type"
```

## Development

```bash
go build -o bolt-cowork ./cmd/bolt-cowork   # Build binary
go test ./... -v                             # Run all tests
go install ./cmd/bolt-cowork                 # Install to PATH
```

## Roadmap

| Version      | Feature                                                |
| ------------ | ------------------------------------------------------ |
| **v0.1.3**   | ✅ Core agent, REPL, auto-setup, model/key management  |
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
