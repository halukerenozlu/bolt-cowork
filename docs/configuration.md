# Configuration

Bolt Cowork reads its configuration from `~/.bolt-cowork/config.yaml`. If the file does not exist, the TUI setup wizard starts on first run and helps you choose a provider and store an API key in the system keyring.

API keys are stored with `zalando/go-keyring` in the operating system credential store:

- Windows Credential Manager
- macOS Keychain
- Linux Secret Service

API keys are not written to `config.yaml`.

---

## Config File

```yaml
# ~/.bolt-cowork/config.yaml

default_provider: anthropic

providers:
  anthropic:
    # API key stored in system keyring - not here
    models:
      - claude-opus-4-6
      - claude-sonnet-4-6
      - claude-haiku-4-5

  openai:
    # API key stored in system keyring - not here
    models:
      - gpt-4o
      - gpt-4o-mini
      - gpt-4.1

  gemini:
    # API key stored in system keyring - not here
    models:
      - gemini-2.5-pro
      - gemini-2.5-flash
      - gemini-2.0-flash

approval: full # full | plan-only | dangerous-only | none
theme: dark # dark | light | system

trusted_dirs: []

sandbox:
  denied_patterns:
    - "*.env"
    - "*.key"
    - ".ssh/*"
    - ".gnupg/*"
    - "*.pem"
    - "*.p12"
    - "*.pfx"
    - "*.secret"
    - "*.token"
    - "credentials.json"
    - "service-account*.json"

skills:
  dirs:
    - cmd/bolt-cowork/skills
    - ~/.bolt-cowork/skills
    - ./bolt-skills
```

---

## Trusted Directories

On first run in a workspace, Bolt Cowork asks whether you trust that directory. Trusted directories are stored in `trusted_dirs`.

Trust is exact-match only. Trusting `/work/project` does not automatically trust `/work/project/subdir`.

---

## API Keys

Use the TUI setup wizard or the provider connection flow to store API keys in the keyring. The config file keeps provider and model preferences only.

Environment variables may still be useful for local development workflows, but the product configuration model treats the keyring as the persistent storage location.

---

## CLI Flags

Flags override the config file for a single run.

| Flag         | Description             | Example                     |
| ------------ | ----------------------- | --------------------------- |
| `--dir`      | Directory to operate in | `--dir ./my-project`        |
| `--provider` | LLM provider to use     | `--provider anthropic`      |
| `--approval` | Approval mode           | `--approval dangerous-only` |
| `--model`    | Model name              | `--model claude-sonnet-4-6` |

---

## Approval Modes

| Flag                        | Behavior                                             |
| --------------------------- | ---------------------------------------------------- |
| `--approval full`           | Prompt for skill matching, plan approval, and steps  |
| `--approval plan-only`      | Prompt at the plan stage only                        |
| `--approval dangerous-only` | Prompt for dangerous execution steps                 |
| `--approval none`           | Run without prompts                                  |

`--approval dangerous` is not a valid alias.

---

## MCP Servers

MCP (Model Context Protocol) servers extend what Bolt Cowork can do. Configure them in `~/.bolt-cowork/mcp.json`:

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "allowed_tools": ["read_*"],
      "denied_tools": ["write_*", "delete_*"]
    }
  ]
}
```

MCP permission profiles support allow and deny patterns. Deny rules win when a tool matches both lists.

See the [MCP documentation](https://modelcontextprotocol.io) for available servers.
