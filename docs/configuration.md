# Configuration

Bolt Cowork reads its configuration from `~/.bolt-cowork/config.yaml`. If the file does not exist, it uses built-in defaults.

---

## Config File

```yaml
# ~/.bolt-cowork/config.yaml

provider: openai # openai | anthropic
approval: full # full | dangerous-only | none
model: gpt-4o # model name passed to the provider
```

---

## Environment Variables

API keys are read from environment variables and are never stored in the config file.

| Variable            | Provider  |
| ------------------- | --------- |
| `OPENAI_API_KEY`    | OpenAI    |
| `ANTHROPIC_API_KEY` | Anthropic |

---

## CLI Flags

Flags override the config file for a single run.

| Flag         | Description             | Example                     |
| ------------ | ----------------------- | --------------------------- |
| `--dir`      | Directory to operate in | `--dir ./my-project`        |
| `--provider` | LLM provider to use     | `--provider anthropic`      |
| `--approval` | Approval mode           | `--approval dangerous-only` |
| `--model`    | Model name              | `--model claude-3-5-sonnet` |

---

## Fallback Chain

If the primary provider fails (rate limit, network error, etc.), Bolt Cowork automatically switches to the next available provider.

```yaml
providers:
  - openai
  - anthropic
```

The chain is tried in order. If all providers fail, the task is aborted with an error message.

---

## MCP Servers

MCP (Model Context Protocol — Model Bağlam Protokolü) servers extend what Bolt Cowork can do. Configure them in `~/.bolt-cowork/mcp.json`:

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    }
  ]
}
```

See the [MCP documentation](https://modelcontextprotocol.io) for available servers.
