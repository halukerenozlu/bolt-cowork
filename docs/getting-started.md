# Getting Started

## Requirements

- Go 1.26+ if building from source
- An API key for at least one supported runtime provider: Anthropic, OpenAI, or Gemini

---

## Installation

### Option 1 - Download Binary

Download the pre-built binary for your platform from the [releases page](https://github.com/halukerenozlu/bolt-cowork/releases).

| Platform        | File                            |
| --------------- | ------------------------------- |
| Linux (amd64)   | `bolt-cowork-linux-amd64`       |
| macOS (amd64)   | `bolt-cowork-darwin-amd64`      |
| macOS (arm64)   | `bolt-cowork-darwin-arm64`      |
| Windows (amd64) | `bolt-cowork-windows-amd64.exe` |

### Option 2 - Build from Source

```bash
git clone https://github.com/halukerenozlu/bolt-cowork.git
cd bolt-cowork
make build
# Binary is created at dist/bolt-cowork
```

### Option 3 - Install via Go

```bash
go install github.com/halukerenozlu/bolt-cowork/cmd/bolt-cowork@latest
```

---

## First Run

```bash
bolt-cowork --dir ./my-project "List all Go files"
```

If `~/.bolt-cowork/config.yaml` does not exist, Bolt Cowork starts the TUI setup wizard. The wizard asks you to choose a provider and enter an API key. The key is stored in the system keyring, not in the config file.

After setup, Bolt Cowork will:

1. Ask whether you trust the selected workspace directory
2. Match relevant skills when needed
3. Show you a plan
4. Wait for your approval or revision
5. Execute the task step by step
6. Report the result

---

## Runtime Providers

Bolt Cowork currently supports:

| Provider  | Example models                                      |
| --------- | --------------------------------------------------- |
| Anthropic | `claude-opus-4-6`, `claude-sonnet-4-6`              |
| OpenAI    | `gpt-4o`, `gpt-4o-mini`, `gpt-4.1`                  |
| Gemini    | `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.0-flash` |

You can switch models during a session with `/model`. Bolt Cowork auto-detects the provider from known model names.

---

## Approval Modes

| Flag                        | Behavior                                             |
| --------------------------- | ---------------------------------------------------- |
| `--approval full`           | Prompt for skill matching, plan approval, and steps  |
| `--approval plan-only`      | Prompt at the plan stage only                        |
| `--approval dangerous-only` | Prompt for dangerous execution steps                 |
| `--approval none`           | Run without prompts                                  |

The interactive `/mode build` shortcut maps to `dangerous-only`.

---

## Common Commands

| Command         | Purpose                                  |
| --------------- | ---------------------------------------- |
| `/help`         | Show available commands                  |
| `/model`        | Switch model and provider                |
| `/key`          | Manage provider API keys                 |
| `/config`       | Inspect or adjust configuration          |
| `/dir`          | Change workspace directory               |
| `/clear`        | Clear conversation history and counters  |
| `/skills`       | Show loaded skills                       |
| `/skill <name>` | Inspect one skill                        |
| `/use <name>`   | Activate a skill for the next task       |
| `/mode`         | Change approval mode                     |
| `/init`         | Initialize project context               |

---

## Version History

| Version | Summary                                                                                       | Status  |
| ------- | --------------------------------------------------------------------------------------------- | ------- |
| v0.1    | Core agent: sandbox, LLM provider, fallback chain, file operations                            | Done    |
| v0.2    | Skill system: SKILL.md loading, keyword matching, prompt injection                            | Done    |
| v0.3    | MCP client: JSON-RPC 2.0, tool discovery, permission profiles, resources, notifications       | Done    |
| v0.4.0  | TUI foundation with Bubble Tea, welcome screen, and split layout                              | Done    |
| v0.4.1  | Agent integration, streaming output, plan widget, execution log, and command palette          | Done    |
| v0.4.2  | Palette overlay, grouped commands, ctrl+x chords, git status, and live right panel            | Done    |
| v0.4.3  | Modal system, setup wizard, keyring, animations, approval modal, and trusted directories      | Done    |
| v0.4.4  | Improvements and refinements based on v0.4.3 feedback                                        | In progress |
