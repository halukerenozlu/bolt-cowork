# Bolt Cowork

A CLI-based local file agent platform inspired by [Claude Cowork](https://claude.com/product/cowork). Give it access to a folder, describe a task in natural language, and it gets the work done.

## Status

**v0.1.4** — Core agent loop, interactive REPL, setup wizard, runtime model/key management, hardened path resolution, improved UX.

## Features (v0.1)

- **Sandbox** — Restricts file access to allowed directories with path validation, denied patterns, symlink escape protection, and narrow path-traversal checks that don't false-positive on legitimate `..hidden` directories
- **Config** — YAML configuration with validation (`~/.bolt-cowork/config.yaml`), auto-created on first run
- **LLM Providers** — Pluggable provider interface with real Anthropic Messages API, fallback chain (auto-switches when a model hits its limit)
- **Agent Loop** — Plan → approve → execute → report cycle with approval gates
- **Interactive REPL** — Run `bolt-cowork` to enter interactive mode with persistent session
- **Auto Setup** — First run automatically starts the setup wizard (provider, API key, model, workspace)
- **Runtime Model Switching** — `/model haiku|sonnet|opus` to switch models during a session
- **API Key Management** — `/key` to view, `/key set` to change API keys without leaving REPL
- **Typo Suggestions** — Unknown slash commands suggest the closest match (`Did you mean '/model'?`) via Levenshtein distance
- **File Content Display** — Read actions return actual contents (truncated at 200 lines with a `[truncated]` marker), not just byte counts
- **Clean Cancellation** — `Ctrl+C` during an approval prompt returns to the REPL with `Command cancelled.` instead of a raw EOF error

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

| Command               | Description                                |
| --------------------- | ------------------------------------------ |
| `/help`               | Show available commands                    |
| `/model`              | Show current model                         |
| `/model <name>`       | Switch model: haiku, sonnet, opus          |
| `/key`                | Show current API key (masked)              |
| `/key set`            | Change API key for active provider         |
| `/key <provider>`     | Show API key for specific provider         |
| `/key set <provider>` | Change API key for specific provider       |
| `/quit`               | Exit REPL                                  |

Unknown commands trigger a typo suggestion when a close match exists.

## Approval Modes

| Mode             | Behavior                                                   |
| ---------------- | ---------------------------------------------------------- |
| `full`           | Pauses at every stage, including reads and lists (default) |
| `plan-only`      | Pauses only at the planning stage                          |
| `dangerous-only` | Auto-approves read/list actions (shown with `[auto]`); pauses for writes, executes, deletes, overwrites |
| `none`           | Fully automatic                                            |

## Project Structure

```
bolt-cowork/
├── cmd/bolt-cowork/     # CLI entry point, REPL, init wizard, key/model management
├── internal/
│   ├── agent/           # Agent loop, planner, executor, approval system, levenshtein
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

The startup banner shows the resolved absolute path of the working directory, so you always know exactly which folder the agent is operating on.

## Development

```bash
make build          # Build binary
make test           # Run all tests
make lint           # Run golangci-lint
go install ./cmd/bolt-cowork   # Install to PATH
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution process and [SECURITY.md](SECURITY.md) for vulnerability reporting.

## Roadmap

| Version      | Feature                                                |
| ------------ | ------------------------------------------------------ |
| **v0.1.4**   | ✅ Core agent, REPL, auto-setup, model/key management, hardened paths, UX polish |
| v0.2         | Skill system (SKILL.md loading, auto-trigger)          |
| v0.3         | MCP client (JSON-RPC 2.0, external tool access)        |
| v0.4         | Sub-agent coordination (parallel tasks via goroutines) |
| v0.5         | Custom LLM provider (self-trained model support)       |
| v0.6         | GUI (Web UI with React + Go backend)                   |

See [VISION.md](VISION.md) for the full project vision and design principles, and [CHANGELOG.md](CHANGELOG.md) for detailed release notes.

## Tech Stack

- **Go 1.25+** — Core agent, CLI, all backend logic
- **Shell** — Build/test automation
- **TypeScript** — GUI (v0.6, planned)

## License

[MIT](LICENSE)
