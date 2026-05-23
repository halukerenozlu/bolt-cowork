# Bolt Cowork — Claude Code Project Memory

**Type:** Terminal-native File Agent Platform
**Primary Language:** Go 1.26+ | **Additional:** Shell (automation), TypeScript (GUI, v0.6+)
**Current Version:** v0.4.2
**Detailed Spec:** `/spec/bolt-cowork-project-spec-EN.md`

---

## Behavioral Guidelines

> This section defines **how Claude Code thinks and behaves**.
> It takes precedence over project details — it applies to every task.

### 1. Think Before Coding

**Don't assume. Surface confusion. Present tradeoffs.**

Before any implementation:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick one silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.
  **Project-specific:** `internal/agent/`, `internal/skill/`, and `internal/mcp/` are
  tightly coupled. Do not start writing code before understanding which layers
  a change will affect.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked — no abstractions, flexibility, or configurability.
- No interfaces for single-use code.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.
  **Ask yourself:** "Would a senior Go engineer look at this and say it's overcomplicated?"
  If yes, simplify.

**Project-specific:** The skill system (matcher, injector, registry) already has a
layered architecture. When adding a new feature, prefer extending an existing
layer over introducing a new abstraction layer.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match the existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it — don't delete it.
  When your changes create orphans:

- Remove imports/variables/functions that **your** changes made unused.
- Leave pre-existing dead code untouched unless explicitly asked.
  **Test:** Every changed line should trace directly to Haluk's request.

**Project-specific:** `internal/mcp/` is a large and deeply nested package.
Editing one file and accidentally side-effecting other MCP files is a
particularly high risk here.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:

| Instead of...    | Use...                                                         |
| ---------------- | -------------------------------------------------------------- |
| "Add validation" | "Write tests for invalid inputs, then make them pass"          |
| "Fix the bug"    | "Write a test that reproduces it, then make it pass"           |
| "Refactor X"     | "Ensure tests pass before and after the refactor"              |
| "Match a skill"  | "Assert the expected score in `matcher_test.go`, then pass it" |

For multi-step tasks, state a brief plan:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

**Project-specific:** `make test` must always pass. Don't write implementation
before the plan is approved — but implementation is not complete without tests.

---

**These guidelines are working if:**

- Diffs contain only the requested changes
- Code is simple the first time — no rewrites needed
- Clarifying questions come before implementation, not after mistakes

---

## Terminology — Do Not Confuse!

In this project, AI is used in two distinct contexts:

| Context               | Purpose                                                              | Examples                              |
| --------------------- | -------------------------------------------------------------------- | ------------------------------------- |
| **Development Tools** | Used to **write** Bolt Cowork's code. NOT part of the final product. | Claude Code, OpenAI Codex, Gemini CLI |
| **Runtime Providers** | Bolt Cowork's **own brain**. End users interact with these.          | OpenAI API, Anthropic API, Custom LLM |

Claude Code → Primary developer. Writes the code.
OpenAI Codex → Primary Code reviewer. Reviews the code. Secondary developer
Gemini CLI → Developer + reviewer. Can serve both roles.
Haluk → Product manager + architect. Makes decisions and approvals.
Runtime provider → Solves user tasks while Bolt Cowork is running.
These two contexts must never be confused.

---

## Folder Structure

```
bolt-cowork/
├── cmd/bolt-cowork/main.go           # Entry point
│   ├── embedded_skills.go            # go:embed directive — bundled skills
│   └── skills/                       # Default SKILL.md files (file-organizer, summarizer, code-reviewer, git-helper, project-scaffolder, pdf-converter)
├── internal/
│   ├── agent/                   # Agent loop, planning, execution
│   │   ├── actions/call_mcp_tool.go      # CallMCPToolAction
│   │   └── actions/read_mcp_resource.go  # ReadMCPResourceAction
│   ├── provider/                # LLM providers + fallback chain
│   ├── skill/                   # Skill system: loader, matcher, injector (v0.2.4)
│   │   ├── skill.go             # SkillScope, SkillMetadata, Skill struct, SkillStore interface
│   │   ├── frontmatter.go       # parseFrontMatter, descriptionFallback, nameFromPath
│   │   ├── loader.go            # ParseFile, LoadAll (scope assignment), LoadEmbedded, Store
│   │   ├── matcher.go           # Hybrid matching: keyword+tags scoring, LLM disambiguation fallback
│   │   ├── injector.go          # BuildSkillContext, InjectSkills (<active_skills> XML)
│   │   ├── registry.go          # SearchByTag, ListCategories, GetByCategory, Search methods
│   │   └── template.go          # GenerateTemplate — SKILL.md template generator
│   ├── mcp/                     # MCP client, transport, registry
│   │   ├── types.go             # MCP type model: Tool, ToolSchema, CallToolResult, Initialize*
│   │   ├── loader.go            # LoadConfig, DefaultConfigPath, expandTilde
│   │   ├── normalize.go         # NormalizeConfig: trim, validate, dedup
│   │   ├── registry.go          # Registry: AddServer, GetTool, LoadFromConfig, LoadFromFile
│   │   ├── tool_registry.go     # ToolRegistry: composite serverName/toolName key
│   │   ├── permissions.go       # PermissionProfile: IsAllowed, LoadPermissions (v0.3.6)
│   │   ├── resource_types.go    # MCP resource wire types (v0.3.7)
│   │   ├── resource_registry.go # ResourceRegistry (v0.3.7)
│   │   ├── notification.go      # NotificationRegistry (v0.3.7)
│   │   ├── jsonrpc.go           # JSON-RPC 2.0 core (Request, Response, PendingRegistry)
│   │   ├── transport.go         # Transport interface (Send/Receive/Close)
│   │   ├── stdio.go             # StdioTransport with cancellable locks
│   │   ├── process.go           # StartProcess helper
│   │   └── testutil/            # Mock MCP server + fakeserver e2e helpers (v0.3.7)
│   ├── tool/                    # Tool definitions and helpers
│   ├── prompt/                  # Prompt templates and helpers
│   ├── ui/                      # Terminal user interface (v0.4+)
│   │   ├── app.go               # Root App model, view switching (Welcome → Session)
│   │   ├── keys/keymap.go       # Quit and palette key bindings
│   │   ├── theme/theme.go       # Centralized lipgloss color and style definitions
│   │   ├── views/welcome.go     # Welcome screen — title, text input, git branch + version status bar
│   │   ├── views/session.go     # Split layout placeholder (70% chat / 30% status)
│   │   ├── panels/chat.go       # Chat panel
│   │   ├── panels/status.go     # Status panel
│   │   ├── panels/input.go      # Input panel (bubbles/textinput)
│   │   ├── panels/statusbar.go  # Status bar panel
│   │   ├── widgets/spinner.go   # Spinner (bubbles/spinner)
│   │   ├── widgets/plan.go      # Plan widget (glamour fallback)
│   │   ├── widgets/approval.go  # Approval widget
│   │   └── widgets/palette.go   # Palette widget
│   ├── sandbox/                 # File access restriction
│   │   │                        # Exported: IsUnderDir (filepath.Rel-based boundary check)
│   │   │                        # Exported: WrapFSError (user-friendly FS error messages)
│   └── config/                  # Configuration management
├── pkg/types/                   # Shared type definitions
├── testdata/                    # ⛔ Tests run HERE ONLY
│   ├── sample-dir/              # Fake user directory
│   └── fixtures/                # Fixed test data
├── scripts/                     # build.sh, test.sh, lint.sh
├── web/                         # Added in v0.6 (React + TS)
├── go.mod / go.sum
└── Makefile
```

---

## Core Interfaces

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    StreamChat(ctx context.Context, messages []Message) (<-chan string, error)
    Name() string
    Available() bool
}

type FallbackChain struct {
    providers []LLMProvider
    current   int
}

type SkillMetadata struct {
    Name             string
    Description      string
    Tags             []string
    Category         string
    Version          string
    Priority         int
    AutoTrigger      bool
    RequiresApproval bool
}

type Skill struct {
    Metadata SkillMetadata
    Scope    SkillScope // ScopeBundled | ScopeGlobal | ScopeProject
    Content  string
    FilePath string
}

type SkillStore interface {
    LoadAll(dirs []string) []string
    GetAll() []Skill
    GetByName(name string) (*Skill, error)
}

type MCPClient interface {
    ListTools() ([]Tool, error)
    CallTool(name string, args map[string]any) (Result, error)
    Close() error
}

type Agent struct {
    chain      *FallbackChain
    skills     []Skill
    mcpClients []MCPClient
    sandbox    *Sandbox
}
```

---

## Approval Gates

The agent loop waits for user approval at 4 stages:

| #   | Stage               | Options                                                               |
| --- | ------------------- | --------------------------------------------------------------------- |
| 1   | Skill matching      | Approve / Reject (no Modify — use `/use <name>` for manual selection) |
| 2   | Plan generation     | Approve / Reject / Revise                                             |
| 3   | Each execution step | Continue / Approve all / Stop                                         |
| 4   | Result              | Accept / Roll back                                                    |

**Speed Modes:**

- `--approval full` — stop at every step; skill approval **prompts** (default)
- `--approval plan-only` — stop only at plan stage; skill approval **skipped** (auto-approved)
- `--approval dangerous-only` — stop only for delete/overwrite operations; skill approval **skipped**
- `--approval none` — fully automatic; skill approval **skipped**
  **MCP Tool Approval (v0.3.5+):**

- MCP tool calls are controlled by `MCPApprovalMode` when `--mcp-approval` is set
- If `--mcp-approval` is not set, MCP tools follow the global approval mode like all other tools
- `IsDangerousTool()` determines danger level via 26 keywords + empty description check
- `/mcp list` and `/mcp tools` are available as REPL slash commands
  **MCP Permission Profile (v0.3.6+):**

- `PermissionProfile`: `AllowedTools` and `DeniedTools` fields — `filepath.Match` wildcard support (`delete_*`, `*`, exact name)
- **Denylist wins:** If a tool matches both lists, it is blocked
- `client.LoadPermissions(cfg)` — called after config load; sets `SetPermissions` per server
- `~/.bolt-cowork/mcp.json` added to `protectedPaths`; the agent cannot automatically read or write this file
  **MCP Resources (v0.3.7+):**

- `Client.DiscoverResources(ctx)` calls `resources/list` on servers and writes results into `ResourceRegistry`
- `Client.ReadResource(ctx, serverName, uri)` reads a single resource's content via `resources/read`
- `ResourceRegistry` stores resource lists per server name in a thread-safe manner
- `ReadMCPResourceAction` supports the `read_mcp_resource` step in the planner/executor flow
  **MCP Notifications (v0.3.7+):**

- `NotificationRegistry` uses a method-based callback map and recovers handler panics by logging them
- Built-in handlers are kept separate from user handlers; stale flag behavior cannot be overwritten
- `notifications/resources/updated` sets `resourcesStale`; `notifications/tools/list_changed` sets `toolsStale`
- `ConnectAndInitialize(ctx, name, transport)` combines connection + initialize handshake + `notifications/initialized` flow into a single API
  **E2E Test Infrastructure (v0.3.7+):**

- `internal/mcp/testutil/mock_server.go` provides an in-process mock server
- `internal/mcp/testutil/fakeserver/main.go` is a stdio-based fakeserver binary used in e2e tests
- `internal/mcp/e2e_test.go` builds the fakeserver in a temp directory inside `TestMain` and cleans it up after tests

---

## Coding Standards

### Go

- Use Go 1.26+
- Error handling: wrap with `fmt.Errorf("context: %w", err)`
- Write table-driven tests
- Comments in English
- Run `golangci-lint` for lint checks
- Package names should be short and descriptive
- Skill matching must be keyword-based; LLM-based matching is v0.3+ scope (not done in v0.2)
- `/use <name>` command force-activates a skill for the next Run via `SetForceSkills()` (one-shot: auto-cleared after Run)

### Shell

- Bash 5+, start with `#!/usr/bin/env bash`
- `set -euo pipefail` at the top of every script
- Run ShellCheck for lint checks

### TypeScript (v0.6+)

- React 19+ and TypeScript 5+
- ESLint + Prettier
- Functional components only (no class components)

---

## Skill File Format (v0.2.4)

SKILL.md files use YAML frontmatter + Markdown body format:

```yaml
---
name: file-organizer
description: Organizes files by type into directories
auto_trigger: true
tags:
  - files
  - automation
priority: 10
requires_approval: false
---
[Markdown body — instructions for the LLM]
```

- Frontmatter fields: `name` (required), `description` (required), `auto_trigger` (optional, default: false), `tags` (optional), `priority` (optional, default: 0), `requires_approval` (optional, default: false)
- If no frontmatter: `name` is derived from file path, `description` from the first paragraph (max 512 chars)
- CRLF line endings are automatically normalized
- **Load order (override chain):**
  1. Bundled — `skills/` directory next to the binary (shipped with the software)
  2. Global — `~/.bolt-cowork/skills/` (user's own skills)
  3. Project-local — `./bolt-skills/` (project-scoped)
  - Conflict: if the same `name` exists, **the later layer overrides the earlier one** (local > global > bundled)
- Matching: keyword-based (searches description words in the user's command); `auto_trigger: false` skills do not match automatically
- Injection: as an `<active_skills>` XML block into the planner system message
- **`/skill create`** — generates a new SKILL.md template via interactive prompts; writes to global (`~/.bolt-cowork/skills/`) or project-local (`./bolt-skills/`) scope and reloads the store
- **ForceSkills (`/use <name>`):**
  - Set via `SetForceSkills()`; **auto-cleared** after the next `Run()` (one-shot)
  - While ForceSkills is active, `Match()` is skipped and the skill is resolved by name via `GetByName()`
  - Skills with `auto_trigger: false` can also be activated via `/use`
  - If an unknown name is given, a warning is written to stderr and the skill is skipped

---

## Test Isolation Rules ⛔

**No exceptions.**

### Strict Prohibitions

- NEVER use `~/Documents`, `~/Desktop`, `~/Downloads`, or any real user directory in tests
- NEVER access real paths via `os.UserHomeDir()` or `os.Getenv("HOME")` in tests
- NEVER write outside the project folder except to `/tmp` in tests
- Claude Code MUST NEVER leave the `bolt-cowork/` directory during development

### Mandatory Rules

- All file operation tests run inside `testdata/` or `t.TempDir()`
- `testdata/sample-dir/` is used as the fake user directory
- `testdata/fixtures/` is used for fixed test data
- Test data is created before each test run and cleaned up afterward (setup/teardown)
- The sandbox module is also tested inside `testdata/`

---

## Commit Standards

Conventional Commits format, scoped by language:

- `feat(go/agent): add plan approval step`
- `fix(ts/components): fix button alignment`
- `chore(shell/build): update test script`

---

## Development Commands

```bash
make build          # Compile Go binary → dist/bolt-cowork[.exe]
make release        # Cross-compile for 5 platforms into dist/
make install        # Install to $GOPATH/bin
make test           # Run all tests
make lint           # Lint all languages
make dev-web        # Web frontend dev server (v0.6+)

# Run directly
./dist/bolt-cowork --dir ./workspace "List files in this directory"
./dist/bolt-cowork --provider openai --dir ./workspace "Create README.md"
```

**CI:** Tests + vet + build run on every push/PR via GitHub Actions. Dependabot tracks Go module updates.

---

## Development Workflow

1. **Idea** — Human defines the new feature or change
2. **Plan** — Claude Code presents an implementation plan → Human approves or revises
3. **Coding** — Claude Code writes code, stops at each file/function → Human reviews
4. **Testing** — Claude Code writes and runs tests → Human approves coverage
5. **Review** — Codex and/or Gemini CLI reviews the same code from a different perspective → Human evaluates
6. **Merge** — Human makes the final decision; Claude Code creates the commit/PR → Human approves the merge

### Review Chain Rules

1. **The tool that wrote the code cannot review the same code.** If Claude Code wrote it → Codex or Gemini reviews it.
2. **If the review result is "REQUEST CHANGES"** → the writing tool fixes it, and the same reviewer re-inspects.
   **Principle:** Architectural decisions, prioritization, and product vision always belong to the human.

---

## Version Plan

| Version | Summary                                                                                                                                    | Languages  | Status           |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ---------- | ---------------- |
| v0.1    | Core agent: sandbox, LLM provider, fallback chain, file operations, approval loop                                                          | Go + Shell | ✅ Done (v0.1.6) |
| v0.1.7  | Conversation history, OpenAI + Gemini providers                                                                                            | Go         | ✅ Done          |
| v0.1.8  | Bug fixes (signal handling, sandbox, fallback, tilde expansion) — Final bug fix release before v0.2                                        | Go         | ✅ Done          |
| v0.2    | Skill system: SKILL.md reading, keyword matching, prompt injection, /use activation                                                        | Go         | ✅ Done          |
| v0.2.4  | SkillMetadata, SkillScope enum, frontmatter parser, system prompt builder, tool registry                                                   | Go         | ✅ Done          |
| v0.2.5  | Security + quality tests                                                                                                                   | Go         | ✅ Done          |
| v0.2.6  | Stabilization + documentation                                                                                                              | Go         | ✅ Done          |
| v0.3.0  | Skill system revision + real directory hardening                                                                                           | Go         | ✅ Done          |
| v0.3.1  | Cross-platform binary + contributing guide                                                                                                 | Go + Shell | ✅ Done          |
| v0.3.2  | JSON-RPC 2.0 core + transport interface — 78 tests passing                                                                                 | Go         | ✅ Done          |
| v0.3.3  | MCP type model, server registry, .mcp.json loader — 174 tests passing                                                                      | Go         | ✅ Done          |
| v0.3.4  | Tool discovery, CallMCPToolAction, approval gate, provider schema injection — 210+ tests passing                                           | Go         | ✅ Done          |
| v0.3.5  | MCP approval gate + /mcp REPL commands                                                                                                     | Go         | ✅ Done          |
| v0.3.6  | Allowlist/denylist permission profiles + protected config path                                                                             | Go         | ✅ Done          |
| v0.3.7  | E2E test infrastructure, MCP resources, notification event model                                                                           | Go         | ✅ Done          |
| v0.4.0  | TUI foundation: bubbletea + lipgloss + bubbles + glamour, welcome screen, split layout skeleton, readline removed                          | Go         | ✅ Done          |
| v0.4.1  | Agent integration, streaming, spinner, plan viewer widget, exec log, right panel live, command palette (Ctrl+P), REPL commands → palette   | Go         | ✅ Done          |
| v0.4.2  | Palette ANSI overlay, grouped commands, ctrl+x chords, git dirty indicator, right panel 5-section live, narrow collapse, StepStartCallback | Go         | ✅ Done          |
| v0.4.3  | Testing & feedback — run Bolt Cowork against real tasks, collect feedback, identify pain points                                            | Go         | 🔄 In progress   |
| v0.4.4  | Improvements & refinements based on v0.4.3 feedback                                                                                        | Go         |                  |
| v0.4.5  | Sub-agent coordination (parallel tasks via goroutines)                                                                                     | Go + Shell |                  |
| v0.4.6  | Custom LLM provider (self-trained model support)                                                                                           | Go + Shell |                  |
| v0.4.7  | Desktop App — if needed (if TUI is insufficient)                                                                                           |            |                  |
