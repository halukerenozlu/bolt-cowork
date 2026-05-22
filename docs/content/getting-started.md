# Getting Started

## Requirements

- Go 1.26+ (if building from source)
- An API key for at least one supported LLM provider (OpenAI or Anthropic)

---

## Installation

### Option 1 — Download Binary

Download the pre-built binary for your platform from the [releases page](https://github.com/halukerenozlu/bolt-cowork/releases).

| Platform        | File                            |
| --------------- | ------------------------------- |
| Linux (amd64)   | `bolt-cowork-linux-amd64`       |
| macOS (amd64)   | `bolt-cowork-darwin-amd64`      |
| macOS (arm64)   | `bolt-cowork-darwin-arm64`      |
| Windows (amd64) | `bolt-cowork-windows-amd64.exe` |

### Option 2 — Build from Source

```bash
git clone https://github.com/halukerenozlu/bolt-cowork.git
cd bolt-cowork
make build
# Binary is created at dist/bolt-cowork
```

### Option 3 — Install via Go

```bash
go install github.com/halukerenozlu/bolt-cowork/cmd/bolt-cowork@latest
```

---

## First Run

```bash
# Set your API key
export OPENAI_API_KEY=your_key_here
# or
export ANTHROPIC_API_KEY=your_key_here

# Run on a directory
bolt-cowork --dir ./my-project "List all Go files"
```

Bolt Cowork will:

1. Show you a plan
2. Wait for your approval
3. Execute the task
4. Report the result

---

## Approval Modes

| Flag                        | Behavior                                         |
| --------------------------- | ------------------------------------------------ |
| `--approval full`           | Approve every step (default)                     |
| `--approval dangerous-only` | Auto-approve read/list, prompt for write/execute |
| `--approval none`           | Run without any prompts                          |

---

## Version History

| Version | Summary                                                             | Status  |
| ------- | ------------------------------------------------------------------- | ------- |
| v0.1    | Core agent: sandbox, LLM provider, fallback chain, file operations  | ✅ Done |
| v0.2    | Skill system: SKILL.md loading, keyword matching, prompt injection  | ✅ Done |
| v0.3    | MCP client: JSON-RPC 2.0, tool discovery, permission profiles       | ✅ Done |
| v0.4    | Terminal UI: Bubbletea, command palette, streaming, git integration | ✅ Done |
| v0.5    | Sub-agent coordination via goroutines                               | Planned |
| v0.6    | Custom LLM provider support                                         | Planned |
| v0.7    | Desktop app (Electron, if TUI proves insufficient)                  | Planned |
