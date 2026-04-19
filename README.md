# Bolt Cowork

A CLI-based local file agent platform inspired by [Claude Cowork](https://claude.com/product/cowork). Give it access to a folder, describe a task in natural language, and it gets the work done.

## Status

**v0.1.6** -- Readline, runtime config/dir management, plan revision with feedback.

## Features

- **Sandbox** -- Restricts file access to allowed directories with path validation, denied patterns, symlink escape protection, read-only directories, and narrow traversal checks
- **Config** -- YAML configuration (`~/.bolt-cowork/config.yaml`), auto-created on first run, runtime reload via `/config reload`
- **LLM Providers** -- Pluggable provider interface with Anthropic Messages API, fallback chain
- **Agent Loop** -- Plan, approve, execute, report cycle with configurable approval gates
- **Readline REPL** -- Tab completion, persistent command history (`~/.bolt-cowork/history`), line editing shortcuts
- **7 Action Types** -- read, list, write, delete (recursive), move, copy, mkdir
- **Plan Revision** -- Revise plans with feedback up to 3 times before re-submitting
- **Runtime Controls** -- Switch models, change API keys, reload config, change working directory without leaving REPL
- **Typo Suggestions** -- Unknown slash commands suggest the closest match via Levenshtein distance
- **Clean Cancellation** -- Ctrl+C returns to REPL with `Command cancelled.`

## Quick Start

```bash
git clone https://github.com/halukerenozlu/bolt-cowork.git
cd bolt-cowork
make install

bolt-cowork
```

On first run, the setup wizard guides you through provider selection, API key, model, and workspace configuration.

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
| `/config`             | Show current config (keys masked)          |
| `/config path`        | Show config file path                      |
| `/config reload`      | Reload config from disk                    |
| `/dir`                | Show working directory                     |
| `/dir <path>`         | Change working directory                   |
| `/quit`               | Exit REPL                                  |

Tab completion works for all commands and subcommands. Unknown commands trigger typo suggestions.

## Approval Modes

| Mode             | Behavior                                                   |
| ---------------- | ---------------------------------------------------------- |
| `full`           | Every step requires approval, including reads (default)    |
| `plan-only`      | Only plan stage requires approval                          |
| `dangerous-only` | Read/list auto-approve; writes/deletes/moves require approval |
| `none`           | Fully automatic                                            |

## Project Structure

```
bolt-cowork/
├── cmd/bolt-cowork/     # CLI entry point, REPL, init wizard
├── internal/
│   ├── agent/           # Agent loop, planner, executor, approval, levenshtein
│   ├── config/          # YAML config loading and validation
│   ├── mcp/             # MCP client (v0.3)
│   ├── provider/        # LLM provider interface + fallback chain
│   ├── sandbox/         # File access restriction, read-only dirs
│   └── skill/           # Skill system (v0.2)
├── pkg/types/           # Shared types (Message, Role, StepAction)
├── testdata/fixtures/   # Test fixtures and sample configs
└── skills/              # Default SKILL.md files
```

## Configuration

Config file: `~/.bolt-cowork/config.yaml`

```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: your-api-key-here
    models:
      - claude-sonnet-4-6

sandbox:
  allowed_dirs:
    - ./workspace
  read_only_dirs:
    - ~/Documents/reference
  denied_patterns:
    - "*.env"
    - "*.key"
    - ".ssh/*"

approval_mode: full
```

Use `/config reload` to apply changes without restarting.

## Development

```bash
make build          # Build binary
make install        # Install with version injection
make test           # Run all tests with race detector
make lint           # Run go vet
make clean          # Remove binary
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution process and [SECURITY.md](SECURITY.md) for vulnerability reporting.

## Roadmap

| Version      | Feature                                                |
| ------------ | ------------------------------------------------------ |
| **v0.1.6**   | ✅ Readline, config/dir commands, plan revision        |
| v0.1.7       | Conversation history, OpenAI and Gemini providers      |
| v0.2         | Skill system (SKILL.md loading, auto-trigger)          |
| v0.3         | MCP client (JSON-RPC 2.0, external tool access)        |
| v0.4         | Sub-agent coordination (parallel tasks via goroutines) |
| v0.5         | Custom LLM provider (self-trained model support)       |
| v0.6         | GUI (Web UI with React + Go backend)                   |

See [VISION.md](VISION.md) for the full project vision and [CHANGELOG.md](CHANGELOG.md) for detailed release notes.

## License

[MIT](LICENSE)
