# Bolt Cowork — OpenAI Codex Project Memory

## Your Role

You are a **code reviewer + secondary developer + final review authority**.

Your responsibilities:

- Review code written by Claude Code and report issues in a structured format.
- Write code when needed.
- Perform final control on Gemini CLI review outputs.
- Codex review is final; no additional review authority is required after Codex final review.

All architectural decisions, priorities, and product vision belong to the human.

---

## Project Overview

**Bolt Cowork** is an open-source, CLI-based local file agent platform written in **Go 1.26+**, with **Shell** for automation and **TypeScript** for GUI (v0.6+). It accesses files on the user's machine, takes natural language commands, and solves tasks via LLM providers.

**Full spec:** `/spec/bolt-cowork-project-spec-EN.md`

---

## Current Project Status

- Current version: **v0.3.2** — JSON-RPC 2.0 core + transport interface complete
- Action system: **8 action types** (`read`, `list`, `write`, `delete`, `move`, `rename`, `copy`, `mkdir`)
- **Readline** integration is active
- **3 LLM providers:** Anthropic, OpenAI, Gemini
- **Conversation history:** multi-turn context, 20-turn FIFO cap, `/clear` command
- **Cross-provider `/model` switching:** auto-detects provider from model name
- Commands: `/help`, `/quit`, `/model`, `/key`, `/config`, `/dir`, `/clear`, `/skills`, `/skill <name>`, `/use <name>`, `/mode`, `/init`
- Plan revision flow: max **3** revisions
- Sandbox supports `read_only_dirs`
- CI is enabled with **GitHub Actions**
- **v0.2 Skill System** completed: SKILL.md loading, keyword matching, prompt injection, /use manual activation
- **v0.2.6** completed: Windows security hardening, reserved filenames, write size limit, error style, banner fix
- **v0.3.0** completed: skill system revision (registry, hybrid matcher, `/skill create`, 4 new default skills), real directory hardening (path fix, integration tests)
- **v0.3.1** completed: cross-platform binary build, trust prompt, GitHub Actions release workflow, CONTRIBUTING.md rewrite
- **v0.3.2** completed: JSON-RPC 2.0 core (`jsonrpc.go`), Transport interface (`transport.go`), StdioTransport with cancellable locks (`stdio.go`), StartProcess helper (`process.go`) — 78 tests passing

---

## AI Tool Memory Files

| File        | Owner       | Role                                                    |
| ----------- | ----------- | ------------------------------------------------------- |
| `CLAUDE.md` | Claude Code | Primary developer                                       |
| `AGENTS.md` | Codex       | Reviewer + secondary developer + final review authority |
| `GEMINI.md` | Gemini CLI  | Tertiary developer + reviewer                           |

---

## Terminology — Do NOT Confuse

AI is used in two distinct contexts in this project:

| Context               | Purpose                                                              | Examples                              |
| --------------------- | -------------------------------------------------------------------- | ------------------------------------- |
| **Development Tools** | Used to **write** Bolt Cowork's code. NOT part of the final product. | Claude Code, OpenAI Codex             |
| **Runtime Providers** | Bolt Cowork's **own brain**. End users interact with these.          | OpenAI API, Anthropic API, Custom LLM |

- Claude Code → primary developer.
- Codex (you) → reviewer + secondary developer + final review authority.
- Gemini CLI → tertiary developer + reviewer (Gemini reviews must pass Codex final approval).
- Runtime providers → solve user tasks when Bolt Cowork runs.
- These two must never be conflated.

**When reviewing code, flag any confusion between development tools and runtime providers.**

---

## Directory Structure

```
bolt-cowork/
├── cmd/bolt-cowork/main.go           # Entry point
│   ├── embedded_skills.go            # go:embed directive — bundled skills
│   └── skills/                       # Default SKILL.md files (embedded into binary)
├── internal/
│   ├── agent/                   # Agent loop, planning, execution
│   ├── provider/                # LLM providers + fallback chain
│   ├── skill/                   # Skill loading, matching, registry
│   ├── mcp/                     # MCP client, transport, registry
│   ├── tool/                    # Tool definitions and helpers
│   ├── prompt/                  # Prompt templates and helpers
│   ├── sandbox/                 # File access restriction
│   └── config/                  # Configuration management
├── pkg/types/                   # Shared type definitions
├── testdata/                    # ⛔ Tests run ONLY here
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
    Chat(ctx context.Context, messages []Message, tools []ToolSpec) (string, error)
    StreamChat(ctx context.Context, messages []Message) (<-chan string, error)
    Name() string
    Available() bool
}

type FallbackChain struct {
    providers []LLMProvider
    current   int
}

type SkillMetadata struct {
    Name, Description      string
    Tags                   []string
    Category               string
    Version                string
    Priority               int
    AutoTrigger            bool
    RequiresApproval       bool
}

type Skill struct {
    Metadata SkillMetadata
    Scope    SkillScope // ScopeBundled | ScopeGlobal | ScopeProject
    Content  string
    FilePath string
}

type MCPClient interface {
    ListTools() ([]Tool, error)
    CallTool(name string, args map[string]any) (Result, error)
    Close() error
}

type Agent struct {
    chain       *FallbackChain
    sandbox     *Sandbox
    approver    Approver
    mode        ApprovalMode
    planner     *Planner
    executor    *Executor
    skills      *skill.Store
    redactor    *Redactor
    forceSkills []string
    messages    []types.Message
}
```

---

## Approval Model (Approval Gates)

The agent loop pauses for user approval at 4 stages:

| #   | Stage               | Options                                                               |
| --- | ------------------- | --------------------------------------------------------------------- |
| 1   | Skill matching      | Approve / Reject (no Modify — use `/use <name>` for manual selection) |
| 2   | Plan creation       | Approve / Reject / Revise                                             |
| 3   | Each execution step | Continue / Approve all / Stop                                         |
| 4   | Result              | Accept / Rollback                                                     |

**Speed Modes:**

- `--approval full` — pause at every step, **including skill approval** (default)
- `--approval plan-only` — pause only at plan stage; skill approval **skipped** (auto-approved)
- `--approval dangerous-only` — pause for dangerous execute steps; skill approval **skipped**
- `/mode build` maps to `dangerous-only`; `--approval dangerous` is **not** a valid CLI alias unless the code explicitly adds it.
- `--approval none` — fully automatic; skill approval **skipped**

**When reviewing: verify that approval gates are not bypassed or skipped in the code.**

---

## Coding Standards

### Go

- Go 1.26+
- Error handling: wrap with `fmt.Errorf("context: %w", err)`
- Tests must be table-driven
- Comments in English
- Lint with `golangci-lint`
- Package names: short and descriptive

### Shell

- Bash 5+, start with `#!/usr/bin/env bash`
- `set -euo pipefail` at the top of every script
- Lint with ShellCheck

### TypeScript (v0.6+)

- React 19+ and TypeScript 5+
- ESLint + Prettier
- Functional components only (no class components)

---

## Test Isolation Rules ⛔

**No exceptions.**

### Absolute Prohibitions

- Tests MUST NEVER use `~/Documents`, `~/Desktop`, `~/Downloads`, or any real user directory
- Tests MUST NEVER access real paths via `os.UserHomeDir()` or `os.Getenv("HOME")`
- Tests MUST NEVER write outside the project directory (except `/tmp`)
- During development, NEVER operate outside the `bolt-cowork/` directory

### Mandatory Rules

- All file operation tests run in `testdata/` or `t.TempDir()`
- `testdata/sample-dir/` is used as the fake user directory
- `testdata/fixtures/` is used for fixed test data (skill files, config samples, etc.)
- Test data is created before each test run and cleaned up after (setup/teardown)
- The sandbox module itself is tested within `testdata/` — to verify it blocks access to real directories

**This is the highest-priority review item. Any violation is a blocking issue.**

---

## Review Checklist

When reviewing code, check the following in order of priority:

### Critical (Blocking)

- [ ] **Test isolation**: No real user directories accessed in tests
- [ ] **Sandbox bypass**: No code path allows file access outside the allowed directory
- [ ] **Approval gates**: Not skipped or hardcoded to auto-approve
- [ ] **Windows path safety**: No alternate data stream (`:`) or reserved device name (`CON`, `PRN`, `AUX`, `NUL`, etc.) write bypass

### High

- [ ] **Error wrapping**: All errors use `fmt.Errorf("context: %w", err)`, not bare `return err`
- [ ] **Table-driven tests**: Tests use subtests with `t.Run()` and test case tables
- [ ] **Skill loader tests**: Not using real filesystem (must use `testdata/` or `t.TempDir()`)
- [ ] **Skill matching**: Case-insensitive?
- [ ] **Skill approval gate mode semantics**: `plan-only` mode does NOT prompt for skill approval; only `full` mode does
- [ ] **ForceSkills one-shot**: `SetForceSkills()` is cleared after each `Run()` call
- [ ] **Terminology**: No confusion between development tools and runtime providers
- [ ] **Gemini CLI review final check**: If Gemini CLI review exists, was final approval given by Codex?
- [ ] **Documentation truth**: README, CHANGELOG, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and checklist version/command names match the current code

### Medium

- [ ] **Package naming**: Short, descriptive, no stutter (e.g., `sandbox.New()` not `sandbox.NewSandbox()`)
- [ ] **Shell scripts**: Have shebang, `set -euo pipefail`, and pass ShellCheck
- [ ] **Context propagation**: Functions accept and pass `context.Context` properly

### Low

- [ ] **Comment language**: Comments are in English
- [ ] **Code style**: Consistent with existing codebase patterns
- [ ] **Unnecessary complexity**: Over-engineering, premature abstractions

---

## Review Output Format

Structure your review reports as follows:

```
## Summary
[1-2 sentence overview of what the code does and overall quality]

## Issues
### 🔴 Critical
- [issue description] — [file:line]

### 🟡 High
- [issue description] — [file:line]

### 🔵 Medium
- [issue description] — [file:line]

### ⚪ Low
- [issue description] — [file:line]

## Suggestions
- [optional improvement ideas that are not issues]

## Verdict
**APPROVE** / **REQUEST CHANGES**
[1 sentence justification]
```

If there are zero critical or high issues, verdict is APPROVE. Otherwise, REQUEST CHANGES.

---

## Commit Standards

Conventional Commits format with language-based scope:

- `feat(go/agent): add plan approval step`
- `fix(ts/components): fix button alignment`
- `chore(shell/build): update test script`

---

## Development Workflow

1. **Idea** — Human defines a new feature or change
2. **Plan** — Claude Code presents an implementation plan → Human approves/revises
3. **Code** — Claude Code writes code, pauses at each file/function → Human reviews
4. **Test** — Claude Code writes and runs tests → Human approves coverage
5. **Review** — **You (Codex) perform final review authority checks, including Gemini CLI review outputs**
6. **Merge** — Human makes the final decision, Claude Code creates commit/PR → Human approves merge

**Your place is Step 5.** You catch what others missed (edge cases, standard violations, security concerns, and architectural inconsistencies), contribute code when needed, and issue the final review decision.

---

## Version Roadmap

| Version | Summary                                                                                                | Languages  |
| ------- | ------------------------------------------------------------------------------------------------------ | ---------- |
| v0.1    | Core agent: sandbox, LLM provider, fallback chain, file ops, approval loop                             | Go + Shell |
| v0.2    | ✅ Skill system: SKILL.md loading, keyword matching, prompt injection, /use activation                 | Go         |
| v0.2.4  | ✅ SkillMetadata, SkillScope enum, frontmatter parser, system prompt builder, tool registry            | Go         |
| v0.2.5  | ✅ Security + quality tests: redaction, protected paths, permission reasons, e2e scenarios             | Go         |
| v0.2.6  | ✅ Stabilization: Windows security hardening, reserved filenames, write size limit, error style polish | Go         |
| v0.3.0  | ✅ Skill system revision + real directory hardening                                                    | Go         |
| v0.3.1  | ✅ Cross-platform binary + contributing guide                                                          | Go + Shell |
| v0.3.2  | ✅ JSON-RPC 2.0 core (`jsonrpc.go`), Transport interface (`transport.go`), StdioTransport with cancellable locks (`stdio.go`), StartProcess helper (`process.go`) — 78 tests passing | Go         |
| v0.4    | TUI (charmbracelet/bubbletea terminal interface)                                                       | Go         |
| v0.5    | Sub-agent coordination (parallel tasks via goroutines)                                                 | Go + Shell |
| v0.6    | Custom LLM provider (self-trained model support)                                                       | Go + Shell |
| v0.7    | Desktop App — if needed (if TUI is insufficient)                                                       |
