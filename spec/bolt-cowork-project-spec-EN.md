# Bolt Cowork — Project Specification Document

**Project Name:** Bolt Cowork
**Primary Language:** Go 1.26+
**Additional Languages:** Shell (automation), TypeScript (Electron) or TUI (bubbletea)
**Type:** CLI-based local file agent platform
**Inspiration:** Claude Cowork (Anthropic)
**Development Model:** Human-directed, AI-assisted development (Claude Code + OpenAI Codex + Gemini CLI)
**License:** Open source (MIT)

---

## 1. Vision

Bolt Cowork is an open-source agent platform that accesses files on the user's computer, receives tasks through natural language commands, and solves those tasks through an LLM (Large Language Model).

It takes the core philosophy of Claude Cowork — "do not just answer, do the work" — and combines it with the strengths of Go (concurrency, single-binary compilation, fast file operations). In later versions, it is enriched with Shell (automation) and TypeScript (user interface).

---

## 2. Terminology — Development Tools vs Runtime

AI is used in two different contexts in this project. To avoid confusion:

### Development Tools

These are used to **write the code** of Bolt Cowork. They are not part of the final product.

| Tool             | Role                        | When It Is Used                                               |
| ---------------- | --------------------------- | ------------------------------------------------------------- |
| **Claude Code**  | Primary developer           | Writes, tests, and refactors Bolt Cowork's Go/TS/Shell code   |
| **OpenAI Codex** | Code reviewer               | Reviews code written by Claude Code and suggests alternatives |
| **Gemini CLI**   | Developer + reviewer        | Writes code like Claude Code and also reviews code like Codex |
| **You**          | Product manager + architect | Makes decisions, approves, and directs                        |

### Runtime Providers

These run as Bolt Cowork's **own brain**. End users interact with them.

| Provider                          | Role         | When It Runs                           |
| --------------------------------- | ------------ | -------------------------------------- |
| **OpenAI API** (GPT models)       | LLM provider | When the user gives Bolt Cowork a task |
| **Anthropic API** (Claude models) | LLM provider | When the user gives Bolt Cowork a task |
| **Your Own LLM** (v0.5)           | LLM provider | When the user gives Bolt Cowork a task |

### Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│  DEVELOPMENT TIME                                                │
│                                                                   │
│  You ──▶ Claude Code / Codex / Gemini CLI ──▶ writes             │
│                                                Bolt Cowork code   │
│                                                                   │
│  These tools are NOT part of the final product.                   │
└──────────────────────────────────────────────────────────────────┘

                           ▼ build ▼

┌──────────────────────────────────────────────────────────────────┐
│  RUNTIME                                                          │
│                                                                   │
│  End User ──▶ Bolt Cowork ──▶ OpenAI / Anthropic /               │
│                              Your Own LLM                         │
│                                   │                               │
│                                   ▼                               │
│                              Performs the task                    │
│                              (edit files, summarize, etc.)        │
│                                                                   │
│  The user selects the provider they want from the config.         │
│  There is no connection to Claude Code / Codex / Gemini.          │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Language Strategy

Each language joins the project at a specific stage and for a specific reason:

| Language       | Entry Time                                    | Usage Area                                                                                          |
| -------------- | --------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| **Go 1.26+**   | Starting with v0.1                            | Core agent, CLI, MCP client, skill system, performance-critical operations — the project's backbone |
| **Shell**      | Starting with v0.1 (minimal), expands in v0.4 | Build/test automation, MCP server startup, CI/CD pipeline, environment setup scripts                |
| **TypeScript** | v0.6                                          | Desktop application with Electron or TUI (terminal user interface) with bubbletea                   |

**Principle:** A new language is added only when a problem appears that Go cannot solve efficiently on its own. Premature optimization is avoided.

---

## 4. Core Features (Version Plan)

### v0.1 — Core Agent _(Go + minimal Shell)_

- Access to a user-selected directory (sandbox/protected-area logic)
- Task definition through natural language
- Replaceable model support through an LLM Provider Interface
- Fallback Chain: automatically switch to the next model when a model limit is reached
- Initial providers: OpenAI API + Anthropic API
- Agent Loop: user command → create plan → user approval → execute → report result
- Basic file operations: read, write, move, delete, rename, content analysis
- Shell: basic automation scripts such as `build.sh`, `test.sh`

**Status:** ✅ Completed (v0.1.0 → v0.1.6)

#### v0.1.6 Highlights

- Readline integration (tab completion, command history)
- /config and /dir commands
- Plan revision feedback (RevisionPrompter, max 3 revisions)

### v0.1.7 — Conversation History + New Providers _(Go)_ ✅

- REPL conversation history (multi-turn context)
- OpenAI API provider implementation
- Google Gemini API provider implementation

### v0.1.8 — Bug Fixes _(Go)_ ✅

- Ctrl+C signal handling (signal canceller, both REPL paths)
- Write approval fix in dangerous-only mode (isDangerous)
- `..hidden` sandbox bypass fix
- Provider fallback support for 401/403
- Delete intent recursive ambiguity resolved
- Conversation memory support for meta questions (empty steps)
- Tilde (~) expansion support in config

### v0.2 — Skill System _(Go)_ ✅

- Read SKILL.md files from `~/.bolt-cowork/skills/` and `./bolt-skills/`
- Define skill metadata through YAML frontmatter
- Automatically trigger skills (based on description) or manually call them (`/skill-name`)
- Inject skill content into the LLM prompt as context
- Default skills: file-organizer, summarizer

#### Skill File Format

YAML frontmatter + Markdown body:

```yaml
---
name: file-organizer
description: Organizes files by type into directories
auto_trigger: true
---
[Markdown body with instructions for LLM]
```

Frontmatter fields (minimal): `name`, `description`, `auto_trigger`

#### Loading Strategy

**Eager loading** — all SKILL.md files are read, parsed, and kept in memory when the application starts.

#### Directories (Load Order)

1. **Bundled** — `skills/` directory next to the binary (default skills shipped with the software)
2. **Global** — `~/.bolt-cowork/skills/` (the user's own skills)
3. **Project-local** — `./bolt-skills/` (project-specific skills)

**Conflict rule:** If skills have the same `name`, the later layer overrides the earlier one (project-local > global > bundled).

#### Matching

**Keyword-based matching:**

- Words in the skill `description` field are searched in the user command
- Case-insensitive matching
- LLM-based matching will be considered for v0.3+ (NOT in v0.2 scope)

#### Context Injection

- Matching skills are injected into the planner system message as an `Active skills:` block
- If multiple skills match, all of them are injected

#### Manual Invocation

- With the `/use <name>` command in the REPL (for example: `/use file-organizer`)
- Activated for the next command; automatically cleared after the command (one-shot)
- Skills with `auto_trigger: false` can also be activated this way
- Tab completion support (for the `/use` command)

#### Default Skills

Shipped by default in the `skills/` directory:

- `file-organizer` — organizes files into directories by type
- `summarizer` — summarizes file/directory content

#### Module Plan (`internal/skill/`)

| File          | Responsibility                                                          |
| ------------- | ----------------------------------------------------------------------- |
| `skill.go`    | `Skill` struct, `SkillStore` interface                                  |
| `loader.go`   | SKILL.md parsing (YAML frontmatter + Markdown body), directory scanning |
| `matcher.go`  | Keyword-based matching, user command → skill matching                   |
| `injector.go` | Inject matching skills into the planner prompt                          |
| `*_test.go`   | Table-driven tests for each file                                        |

### v0.2.x Roadmap (Improvements Before v0.3 MCP)

> This plan was created jointly by Claude, Codex, and Gemini.

#### v0.2.1 — Standardization

- [x] Skill document alignment: clarify approval stage options as Approve/Reject; Modify will not be added (manual control is provided through /use)
- [x] Subcommand hierarchy: list subcommands when /config or /skill is typed
- [x] CI/CD: .github/workflows/ci.yml (go test, go vet, build)
- [x] .github/ISSUE_TEMPLATE/ (bug report, feature request)
- [x] .github/PULL_REQUEST_TEMPLATE.md
- [x] CONTRIBUTING.md, CODE_OF_CONDUCT.md, LICENSE
- [x] ASCII logo (terminal startup screen)
- [x] Deterministic /init command (.cowork/ structure, without LLM)

#### v0.2.2 — UX/Polish

- [x] ASCII-compatible spinner and colored log outputs (compatible with Windows ASCII limitations: [= ], [== ], |, /, -, \)
- [x] /mode plan and /mode build shortcut commands (UX-friendly shortcuts for existing approval modes)
- [ ] README demo animation (terminal recording with VHS or asciinema)

#### v0.2.3 — Safe Expansion

- [x] Real working directory support (with the /dir command) — Sandbox rules are preserved, automated tests do not touch real directories
- [x] Context trimming: summarization mechanism when the token limit is approached in long conversations (last 20 messages / 32K characters)
- [x] Global skill directory (~/.bolt-cowork/skills/) stabilization

#### v0.2.4 — Stabilization ✅

- [x] Comprehensive manual testing (all v0.2.x features)
- [x] Interface preparation for v0.3 MCP client (internal refactoring)
- [x] Final documentation update
- [x] Codex + Gemini cross-review, bug fix

#### v0.2.5 — Security + Quality Tests ✅

- [x] Secret redaction tests: Redactor struct, dedup, substring replacement (8 tests)
- [x] Protected path tests: read/write/delete denied, traversal and symlink blocked (7 tests)
- [x] Permission reason tests: delete, overwrite, outside sandbox, safe actions, format (5 tests)
- [x] Agent e2e scenario tests: simple create, read+write, dangerous approval/rejection, multi-step, invalid action, skill injection (7 tests)
- [x] Skill parser edge case tests: unicode, large body, multiple delimiters, whitespace, empty file, frontmatter-only, tabs, duplicate keys (8 tests)
- [x] MCP config validation tests: valid full/minimal, missing name/URL, invalid transport, duplicate name, empty list, unknown fields, invalid value type (9 tests)
- [x] .ssh/_, .gnupg/_, .config/bolt-cowork/\* added to protected paths

#### v0.2.6 — Stabilization + Documentation ✅

- [x] Protected path case-insensitive matching on Windows (F-005)
- [x] NTFS Alternate Data Stream blocking on Windows (F-014)
- [x] `isReservedFilename`: Windows reserved device names blocked (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
- [x] `maxWriteContentBytes`: 1 MB write size limit
- [x] Plan revision feedback prompt visible (F-012)
- [x] `/dir` resolves relative paths, tilde expansion, filepath.Clean normalization (F-008)
- [x] `--dir /nonexistent` exits with error (F-001)
- [x] Error messages: lowercase start, no trailing periods
- [x] Startup sequence: banner → status → warnings → help hint
- [x] Banner reverted to original Unicode BOLT logo
- [x] Go 1.25 → 1.26
- [x] Removed unused `colorRed`, `colorCyan`, `readREPLLine` functions
- [x] VHS demo tape (`demo.tape`) added
- [x] README, CHANGELOG, CLAUDE.md, AGENTS.md, GEMINI.md updated

#### Deferred Items

| Item                               | Deferral Reason                                        | Target Version |
| ---------------------------------- | ------------------------------------------------------ | -------------- |
| Skill registry/install (internet)  | Requires a security model; MCP must be completed first | v0.4+          |
| TUI framework (bubbletea)          | Set as a v0.6 target                                   | v0.6           |
| Installation wizard (MSI/Homebrew) | Product is still in CLI core stage                     | v0.5+          |
| Promotional website (EN/TR)        | When external users are targeted                       | v0.4+          |

---

### v0.3 — Foundation + MCP Client ← Next

v0.3 is split into two phases: foundation hardening (v0.3.0–v0.3.1) followed by MCP integration (v0.3.2–v0.3.7).

#### v0.3.0 — Foundation I: Skill + Real Directory

Skill registry redesign, matcher improvements, default skill updates
Exit sandbox: test path handling, permissions, and error management on real filesystem

Exit criterion: Skill system works correctly on real directories

#### v0.3.1 — Foundation II: Distribution + Contributing

Go cross-compilation for .exe / Linux / macOS binaries, automated GitHub Releases upload
Sustainable contributing guide, issue/PR template revision, dev environment setup guide

Exit criterion: User without Go installed can run the project via .exe

#### v0.3.2 — MCP Skeleton I: JSON-RPC + Transport ✅ Complete

JSON-RPC 2.0 core: request ID, pending requests, notification dispatch
Transport interface + stdio implementation: stdin/stdout framing, MCP server process launch

Status: Complete — 78 tests passing

Deliverables:

- `internal/mcp/jsonrpc.go`
- `internal/mcp/transport.go`
- `internal/mcp/stdio.go`
- `internal/mcp/process.go`

Completion note: chan struct{} semaphores for cancellable lock acquisition; context.AfterFunc for blocking I/O cancellation

Exit criterion: Can connect to a fake MCP server over stdio

#### v0.3.3 — MCP Skeleton II: Types + Registry ✅ Complete

Tool, ToolSchema, CallToolResult typed model + lifecycle (initialize, initialized, close, timeout)
MCP server registry + .mcp.json loader: ~/.bolt-cowork/mcp.json config file

Status: Complete — 174 tests passing

Deliverables:

- `internal/mcp/types.go` (updated with JSON tags + new wire-protocol and lifecycle types)
- `internal/mcp/loader.go`
- `internal/mcp/normalize.go`
- `internal/mcp/registry.go` (extended with LoadFromConfig, LoadFromFile)

Exit criterion: Multiple server definitions loaded from config into registry

#### v0.3.4 — Tool Discovery + Execution ✅ Complete

✅ tools/list support: fetch tool list from servers, add to registry
✅ CallMCPTool action type: Provider suggests → Agent produces Action → Approval gate decides → MCP client calls
✅ Security additions: registry validation before CallTool, sanitized JSON schema injection

Exit criterion: ✅ MCP tool call works with user approval end-to-end

#### v0.3.5 — CLI Integration + Approval ✅ Complete

**MCP Approval Gate**
Configurable gate that intercepts MCP tool calls before execution.
Modes: full / plan-only / dangerous-only / none.
Configured via --mcp-approval CLI flag or mcp_approval_mode in config file.
CLI flag takes priority. Default (unset) inherits global approval behavior.

**/mcp REPL Commands**
Two new slash commands for MCP inspection:
- /mcp list: shows all configured servers with live ConnectionStatus
- /mcp tools [server-name]: lists available tools grouped by server

Exit criterion: ✅ User can view MCP servers and tools from REPL

#### v0.3.6 — Security

Allowlist / denylist: permission profile controlling which MCP tools can be called
Protected config paths: agent cannot automatically modify ~/.bolt-cowork/mcp.json

Exit criterion: Risky MCP operations are controlled, config is protected

#### v0.3.7 — Stabilization + Tests

Fake MCP server e2e tests: mock stdio server for testing without real server dependency
resources/list, resources/read support, basic notification event model

Exit criterion: Solid MCP foundation ready for v0.4 sub-agent system

- Critical Design Decision (v0.3)
  MCP tool calls must NOT be wired directly to the provider. The flow must be:
  Provider suggests tool
  ↓
  Agent produces Action: CallMCPTool
  ↓
  Approval gate decides
  ↓
  MCP client calls tool
  ↓
  ActionResult → returned to provider context
  This keeps MCP within Bolt's own action system regardless of provider (OpenAI/Anthropic/Gemini).

- Recommended v0.3 Package Architecture
  internal/mcp/ → Public MCP client interface
  internal/mcp/jsonrpc/ → JSON-RPC 2.0 request/response core
  internal/mcp/transport/ → Transport interface + stdio implementation
  internal/mcp/config/ → .mcp.json parse/validate
  internal/mcp/registry/ → MCP server registry
  internal/mcp/types/ → Tool, Resource, Result, Error types
  internal/agent/actions/ → CallMCPToolAction, ReadMCPResourceAction
  cmd/bolt-cowork/ → /mcp commands

### v0.4 — Sub-agent Coordination _(Go + Shell)_

- Break complex tasks into pieces (task decomposition)
- Run parallel tasks with Go goroutines
- Dependency management between sub-agents
- Progress reporting and error management
- Shell: MCP server lifecycle management, environment setup scripts

### v0.5 — Own LLM Provider _(Go + Shell)_

- Support a custom-trained model wrapped with Python + FastAPI
- HTTP-based custom provider implementation
- Performance optimizations with Go:
  - Reading/parsing large files (>100MB) — with the `io.Reader` stream structure
  - Token counting and splitting (tokenization) — with Go libraries
- Model performance comparison (benchmark) tool
- Shell: model service start/stop, health-check scripts

### v0.6 — TUI + Desktop App _(Go + TypeScript)_

- **Primary option:** TUI — terminal user interface with charmbracelet/bubbletea
- **Alternative option:** Electron desktop application (TypeScript frontend + Go backend)
- Real-time task monitoring
- File browser and folder picker
- Skill and MCP server management panel
- Decision to be made after v0.5

---

## 5. Architectural Design

### 5.1 Folder Structure

```
bolt-cowork/
├── cmd/
│   └── bolt-cowork/
│       └── main.go                 # Entry point
├── internal/
│   ├── agent/
│   │   ├── agent.go                # Main agent loop
│   │   ├── planner.go              # Task planning
│   │   └── executor.go             # Task execution
│   ├── provider/
│   │   ├── provider.go             # LLM Provider interface definition
│   │   ├── openai.go               # OpenAI API provider
│   │   ├── anthropic.go            # Anthropic API provider
│   │   ├── custom.go               # Custom LLM provider (v0.5)
│   │   └── fallback.go             # Fallback chain management
│   ├── skill/
│   │   ├── loader.go               # Reading and parsing skill files
│   │   ├── matcher.go              # Task-skill matching
│   │   └── registry.go             # Managing loaded skills
│   ├── mcp/
│   │   ├── client.go               # MCP client implementation
│   │   ├── transport.go            # stdio / HTTP transport
│   │   └── registry.go             # MCP server management
│   ├── sandbox/
│   │   └── sandbox.go              # File access restriction
│   └── config/
│       └── config.go               # Configuration management
├── pkg/
│   └── types/
│       └── types.go                # Shared type definitions
├── testdata/                       # ⛔ Tests run ONLY here
│   ├── sample-dir/                 # Fake user folder (file operation tests)
│   │   ├── notes.txt
│   │   ├── report.pdf
│   │   └── photo.jpg
│   ├── fixtures/                   # Fixed test files (skill parse, config, etc.)
│   │   ├── sample-skill.md
│   │   └── sample-config.yaml
│   └── README.md                   # "This folder is for testing" warning
├── web/                            # Added in v0.6
│   ├── package.json
│   └── src/
│       ├── App.tsx
│       └── components/
├── scripts/                        # Shell scripts
│   ├── build.sh                    # Build automation
│   ├── test.sh                     # Test runner
│   ├── lint.sh                     # Lint check
│   └── mcp-start.sh                # MCP server startup (v0.4)
├── skills/                         # Default skills
│   ├── file-organizer/
│   │   └── SKILL.md
│   └── summarizer/
│       └── SKILL.md
├── go.mod
├── go.sum
├── CLAUDE.md                       # Claude Code project memory
├── AGENTS.md                       # OpenAI Codex project instructions
├── Makefile                        # Unified build for all languages
└── README.md
```

### 5.2 Core Interfaces

```go
// LLM Provider — abstracts which model will be talked to
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    StreamChat(ctx context.Context, messages []Message) (<-chan string, error)
    Name() string
    Available() bool  // Check whether the model limit is reached
}

// FallbackChain — switches to the next provider when the model limit is reached
type FallbackChain struct {
    providers []LLMProvider  // In priority order
    current   int
}

// Skill — represents a loaded skill
type Skill struct {
    Name        string
    Description string
    Content     string
    AutoTrigger bool
}

// MCPClient — abstracts communication with an MCP server
type MCPClient interface {
    ListTools() ([]Tool, error)
    CallTool(name string, args map[string]any) (Result, error)
    Close() error
}

// Agent — main agent loop
type Agent struct {
    chain      *FallbackChain
    skills     []Skill
    mcpClients []MCPClient
    sandbox    *Sandbox
}
```

### 5.3 Fallback Chain System

```
User command received
        │
        ▼
┌─ Provider #1: claude-opus-4-6 ──┐
│  Limit reached?                  │
│  ├─ No  → Use                    │
│  └─ Yes → Next provider          │
└──────────┬──────────────────────┘
           │
           ▼
┌─ Provider #2: claude-sonnet-4-6 ┐
│  Limit reached?                  │
│  ├─ No  → Use                    │
│  └─ Yes → Next provider          │
└──────────┬──────────────────────┘
           │
           ▼
┌─ Provider #3: gpt-4o ───────────┐
│  Limit reached?                  │
│  ├─ No  → Use                    │
│  └─ Yes → Error message          │
└─────────────────────────────────┘

The user is notified at every switch:
"⚠ Opus limit reached, switching to Sonnet."
```

### 5.4 Agent Loop

```
User command
       │
       ▼
┌─ Skill Matching ───────────┐
│ Is there a relevant skill?  │
│ ☑ USER APPROVAL             │
└──────┬──────────────────────┘
       │
       ▼
┌─ Planning ─────────────────┐
│ Send to LLM:                │
│ - User command              │
│ - Skill context             │
│ - Available tools (MCP)     │
│ - Folder contents           │
│                             │
│ LLM returns plan            │
│ ☑ USER APPROVAL             │
└──────┬──────────────────────┘
       │
       ▼
┌─ Execution ────────────────┐
│ Execute each step in order  │
│ ☑ USER APPROVAL (each step) │
│ - File operations           │
│ - MCP tool calls            │
│ - Sub-agent tasks           │
└──────┬──────────────────────┘
       │
       ▼
┌─ Result ───────────────────┐
│ Result report               │
│ ☑ USER APPROVAL (accept/reject) │
└─────────────────────────────┘
```

---

## 6. User Intervention Model

Because "approval at every step" is selected, Bolt Cowork stops at the following approval points:

### 6.1 Approval Gates

| #   | Stage               | Shown to the User                                  | Options                                                              |
| --- | ------------------- | -------------------------------------------------- | -------------------------------------------------------------------- |
| 1   | Skill matching      | "I plan to use these skills for this task: [list]" | ✅ Approve / ❌ Reject (No Modify — manual selection: `/use <name>`) |
| 2   | Plan creation       | "I will follow these steps: [step list]"           | ✅ Approve / ❌ Reject / ✏️ Revise                                   |
| 3   | Each execution step | "I am now going to do this: [move file X]"         | ✅ Continue / ⏭️ Approve all / ❌ Stop                               |
| 4   | Result              | "Task completed. What was done: [summary]"         | ✅ Accept / ↩️ Roll back                                             |

### 6.2 Speed Mode (Optional)

Approving every step is excellent for learning, but it may feel slow over time. For this, modes to be added later:

```bash
# Full control — stop at every step (default)
bolt-cowork --approval full

# Plan approval — stop only at the planning stage
bolt-cowork --approval plan-only

# Stop on dangerous operations — only for delete/overwrite-like operations
bolt-cowork --approval dangerous-only

# Fully automatic — never stop (for experienced users)
bolt-cowork --approval none
```

Initially, `full` mode is the default. You can change it as you get used to the project.

---

## 7. Development Workflow — "Approval at Every Step" Model

### 7.1 Roles

- **Human (Haluk):** Product manager + architect + approver. Decides what will be done, priorities, and architectural decisions. Reviews and approves every output.
- **Claude Code:** Primary developer. Writes ~80-90% of the code. But commits nothing without approval.
- **OpenAI Codex:** Code reviewer.
- **Gemini CLI:** Developer + reviewer. Can be used in both roles.

### 7.2 Development Cycle (Detailed)

```
 ┌─────────────────────────────────────────────────┐
 │  STAGE 1: IDEA (You)                            │
 │  Define a new feature or change                 │
 │  "Let's build the sandbox module for v0.1"      │
 │  ☑ YOU make the decision                        │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  STAGE 2: PLAN (Claude Code — Plan Mode)        │
 │  Claude Code presents the implementation plan   │
 │  "sandbox.go will include these interfaces..."  │
 │  ☑ YOU review, approve, or revise the plan      │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  STAGE 3: CODE WRITING (Claude Code)            │
 │  Claude Code writes the code                    │
 │  Stops when each file/function is completed     │
 │  ☑ YOU review, approve, or request fixes        │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  STAGE 4: TEST (Claude Code)                    │
 │  Claude Code writes and runs tests              │
 │  Shows you the test results                     │
 │  ☑ YOU approve the coverage and results         │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────────┐
 │  STAGE 5: REVIEW (OpenAI Codex + Gemini CLI)        │
 │  Codex and/or Gemini review the same code from      │
 │  a different perspective                            │
 │  Report alternative approaches and issues           │
 │  ☑ YOU evaluate the reviews                         │
 └──────────────────┬──────────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  STAGE 6: MERGE (You + Claude Code)             │
 │  You make the final decisions                   │
 │  Claude Code creates the commit and PR          │
 │  ☑ YOU approve the merge                        │
 └─────────────────────────────────────────────────┘
```

### 7.3 Review Chain Rules

1. **The tool that wrote the code cannot review the same code.** If Claude Code wrote it → Codex or Gemini reviews it.
2. **If the review result is "REQUEST CHANGES"** → the writing tool fixes it, and the same reviewer reviews it again.

### 7.4 Important Principle

Claude Code, Codex, and Gemini are tools — architectural decisions, prioritization, and product vision always belong to the human. Agents answer the "How" question; you answer the "What" and "Why" questions.

---

## 8. Provider Configuration

### ~/.bolt-cowork/config.yaml

```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    models:
      - claude-opus-4-6 # Primary — strongest
      - claude-sonnet-4-6 # Fallback — fast and economical

  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o # Primary
      - gpt-4o-mini # Fallback — low cost

  custom: # Becomes active in v0.5
    endpoint: http://localhost:8000/chat
    models:
      - bolt-local-v1

# Fallback order: tried from top to bottom.
# If a model returns a limit/error, the next one is tried.
# The user is notified at every switch.

fallback_chain:
  - provider: anthropic
    model: claude-opus-4-6
  - provider: anthropic
    model: claude-sonnet-4-6
  - provider: openai
    model: gpt-4o
  - provider: openai
    model: gpt-4o-mini

sandbox:
  allowed_dirs:
    - ./workspace # The user chooses this
  denied_patterns:
    - "*.env"
    - "*.key"
    - ".ssh/*"

# ⚠ THIS SETTING IS NOT USED DURING DEVELOPMENT/TESTING.
# Tests run only inside testdata/ and t.TempDir().
# This config applies only when the end user runs Bolt Cowork.

skills:
  dirs:
    - ~/.bolt-cowork/skills
    - ./bolt-skills

mcp:
  servers: [] # To be filled in v0.3

approval_mode: full # full | plan-only | dangerous-only | none
```

---

## 9. Development Rules

### 9.1 Go Coding Standards

- Use Go 1.25+
- In error handling, wrap with `fmt.Errorf("context: %w", err)`
- Write table-driven tests
- Comments should be in English
- Run lint checks with `golangci-lint`
- Package names should be short and descriptive

### 9.2 Test Isolation Rules ⛔

These rules cover both Claude Code's behavior during development and Bolt Cowork's test suite. **There are no exceptions.**

**Absolute Prohibitions:**

- Tests must NEVER use `~/Documents`, `~/Desktop`, `~/Downloads`, or any real user directory
- Tests must NEVER access real paths with `os.UserHomeDir()` or `os.Getenv("HOME")`
- Tests must NEVER write outside the project folder except `/tmp`
- During development, Claude Code must NEVER leave the `bolt-cowork/` folder

**Mandatory Rules:**

- All file operation tests run in the `testdata/` folder or in a temporary directory created with `t.TempDir()`
- `t.TempDir()` is a function provided by Go's test framework — it creates a unique temporary folder for each test and deletes it automatically when the test finishes
- `testdata/sample-dir/` is used as the fake user folder
- `testdata/fixtures/` is used for fixed test data (skill files, config samples, etc.)
- Test data is created before each test run and cleaned up afterward (setup/teardown)
- The sandbox module itself is also tested inside `testdata/` — to verify that it blocks access to real folders

**Example Test Structure:**

```go
func TestSandbox_BlocksOutsideAccess(t *testing.T) {
    // Create a temporary directory — automatically deleted when the test finishes
    dir := t.TempDir()

    sb := sandbox.New(dir)

    // Should work inside the allowed directory
    err := sb.WriteFile(filepath.Join(dir, "test.txt"), []byte("ok"))
    assert(t, err == nil)

    // Access outside the allowed directory MUST BE BLOCKED
    err = sb.WriteFile("/home/user/Documents/hack.txt", []byte("bad"))
    assert(t, err != nil)  // Should return an error
}
```

### 9.3 TypeScript Coding Standards (v0.6+)

- Use React 19+ and TypeScript 5+
- Check formatting with ESLint + Prettier
- Components must be functional (no class components)

### 9.4 Shell Script Standards

- Use Bash 5+, start with `#!/usr/bin/env bash`
- `set -euo pipefail` must be at the top of every script
- Run lint checks with ShellCheck

### 9.5 Commit Standards

- Use the Conventional Commits format
- Choose the scope by language: `feat(go/agent):`, `fix(ts/components):`, `chore(shell/build):`

### 9.6 Development Commands

```bash
# Unified commands through Makefile
make build          # Build Go binary → dist/bolt-cowork[.exe]
make release        # Cross-compile 5 binaries to dist/
make install        # Install to $GOPATH/bin
make test           # Run all tests
make lint           # Lint all languages
make dev-web        # Web frontend development server (v0.6+)

# Direct execution
./dist/bolt-cowork --dir ./workspace "Summarize the PDF files in this folder"
./dist/bolt-cowork --dir ./workspace --approval full "Separate files by type"
./dist/bolt-cowork --provider openai --dir ./workspace "Create README.md"
```

**CI/CD:** GitHub Actions run test + vet + build on every push/PR. Dependabot tracks Go module updates.

---

## 10. Dependencies (Planned)

### Go (v0.1+)

| Package                                  | Purpose                             |
| ---------------------------------------- | ----------------------------------- |
| `github.com/chzyer/readline`             | Readline (tab completion, history)  |
| `gopkg.in/yaml.v3`                       | YAML parsing (SKILL.md frontmatter) |
| `github.com/sashabaranov/go-openai`      | OpenAI API client _(v0.1.7)_        |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API client _(v0.1.7)_     |

### TypeScript (v0.6+)

| Package       | Purpose      |
| ------------- | ------------ |
| `react`       | UI framework |
| `typescript`  | Type safety  |
| `tailwindcss` | Styling      |

### Shell

| Tool         | Purpose          |
| ------------ | ---------------- |
| `shellcheck` | Lint             |
| `make`       | Build automation |

---

## 11. Risks and Open Questions

| #   | Topic                                    | Status                      | Resolution Plan                               |
| --- | ---------------------------------------- | --------------------------- | --------------------------------------------- |
| 1   | GUI preference: Web vs Electron vs TUI   | To be decided in v0.6       | Evaluate after v0.5                           |
| 2   | Size and capacity of your own LLM        | Depends on the course       | Will be clarified in v0.5                     |
| 3   | Maturity of MCP Go library               | To be researched            | Implement ourselves if needed                 |
| 4   | Token cost management                    | Reduced with fallback chain | Usage limit + cost reporting                  |
| 5   | Security: sandbox bypass risk            | Basic in v0.1               | Strengthen in every version                   |
| 6   | Go performance sufficiency (large files) | Expectation: sufficient     | Optimize with profiling if bottlenecks appear |

---

## 12. Success Criteria

### Definition of "Done" for v0.1:

- [x] `bolt-cowork --dir ./workspace "List the files in this folder"` works
- [x] `bolt-cowork --dir ./workspace "Summarize the README.md file"` works
- [x] `bolt-cowork --dir ./workspace "Sort files into folders by type"` works
- [x] Switching between `--provider openai` and `--provider anthropic` is possible
- [x] Fallback chain works (when the primary model errors, it switches to the second)
- [x] Access outside the sandbox is blocked
- [x] User approval is requested at every step (--approval full)
- [x] The "Approve all" option works
- [x] Basic error messages are understandable

---

### Definition of "Done" for v0.1.7:

- [x] REPL conversation history works (multi-turn context)
- [x] OpenAI API provider implementation works
- [x] Google Gemini API provider implementation works
- [x] /model command can switch between providers
- [x] Fallback chain works with the new providers
- [x] All tests pass

---

### Definition of "Done" for v0.1.8:

- [x] Ctrl+C cancels the command and does not kill the REPL
- [x] dangerous-only mode asks for approval on write
- [x] `..hidden` directories are accessible from inside the sandbox
- [x] 401/403 triggers provider fallback
- [x] Delete recursive behavior is defined with clear rules
- [x] Meta questions are answered from conversation history
- [x] ~ paths in config are resolved correctly
- [x] All tests pass

---

### Definition of "Done" for v0.2: _(Completed: April 25, 2026)_

- [x] SKILL.md files are read from the `~/.bolt-cowork/skills/` folder
- [x] Project-specific skills are read from the `./bolt-skills/` folder
- [x] YAML frontmatter (name, description) is parsed
- [x] Skills are automatically triggered according to the user command
- [x] Can be manually invoked with `/use <name>` (one-shot ForceSkills)
- [x] Skill content is injected into the LLM prompt as context (`<active_skills>` XML block)
- [x] Default skills (file-organizer, summarizer) work
- [x] All tests pass

---

### Definition of "Done" for v0.3:

#### v0.3.0 — Foundation I: Skill + Real Directory _(Completed: 2026-05-12)_

- [x] Skill registry is redesigned
- [x] Skill matcher is improved
- [x] Default skills are updated
- [x] Exit-sandbox scenarios are tested on the real filesystem with path handling, permissions, and error management
- [x] Skill system works correctly on real directories

#### v0.3.1 — Foundation II: Distribution + Contributing _(Completed: 2026-05-15)_

- [x] Cross-platform binary builds are supported (.exe / Linux / macOS)
- [x] Automated GitHub Releases binary upload is ready through CI/CD
- [x] Sustainable contributing guide is updated
- [x] Issue/PR template revision is completed
- [x] Dev environment setup guide is prepared
- [x] User without Go installed can run the project via .exe

#### v0.3.2 — MCP Skeleton I: JSON-RPC + Transport ✅ Complete

- [x] JSON-RPC 2.0 core implementation is complete (request ID, pending requests, notification dispatch)
- [x] Transport interface is defined
- [x] stdio transport implementation is complete (stdin/stdout framing, MCP server process launch)
- [x] Can connect to a fake MCP server over stdio
- [x] Deliverables: `internal/mcp/jsonrpc.go`, `internal/mcp/transport.go`, `internal/mcp/stdio.go`, `internal/mcp/process.go`
- [x] 78 tests passing
- [x] Completion note: chan struct{} semaphores for cancellable lock acquisition; context.AfterFunc for blocking I/O cancellation

#### v0.3.3 — MCP Skeleton II: Types + Registry ✅ Complete

- [x] MCP type model is ready (Tool, ToolSchema, CallToolResult)
- [x] MCP lifecycle is supported (initialize, initialized, close, timeout)
- [x] MCP server registry is added
- [x] `~/.bolt-cowork/mcp.json` config loader parses and validates config
- [x] Multiple server definitions can be loaded from config into the registry

#### v0.3.4 — Tool Discovery + Execution ✅ Complete

- [x] Tool lists can be fetched from servers through `tools/list` support
- [x] Discovered tools are added to the registry
- [x] `CallMCPTool` action type is added
- [x] Provider suggestion → Agent action → Approval gate → MCP client call flow works
- [x] MCP tool call works with user approval end-to-end
- [x] Security additions: registry validation before CallTool, sanitized JSON schema injection

Exit criterion: ✅ MCP tool call works with user approval end-to-end

#### v0.3.5 — CLI Integration + Approval ✅ Complete

- [x] MCP approval gate is compatible with the existing full/plan-only/dangerous-only/none system
- [x] `/mcp list` lists configured servers with live ConnectionStatus in the REPL
- [x] `/mcp tools [server-name]` shows the tool list, grouped by server, in the REPL
- [x] User can view MCP servers and tools from the REPL

#### v0.3.6 — Security

- [ ] Allowlist / denylist permission profile model is added
- [ ] Risky MCP tool calls are controlled by the permission profile
- [ ] Agent cannot automatically modify `~/.bolt-cowork/mcp.json`
- [ ] MCP config path is protected as a protected path

#### v0.3.7 — Stabilization + Tests

- [ ] Fake MCP server e2e tests are added
- [ ] Tests run with a mock stdio server without real server dependency
- [ ] `resources/list` support is added
- [ ] `resources/read` support is added
- [ ] Basic notification event model is ready
- [ ] Solid MCP foundation is ready for the v0.4 sub-agent system

---

_This document is a living document. It will be updated at every version transition._
_Last updated: May 19, 2026_
