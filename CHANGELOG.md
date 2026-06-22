# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.4.4] - 2026-06-22

### Added

- Safe `stat` and streaming SHA-256 `hash` file actions for size and duplicate discovery without printing file contents
- Mouse-draggable chat scrollbar
- Persistent project sessions stored in `~/.bolt-cowork/sessions/<project-key>/` (one directory per workspace, keyed by a hash of its absolute path) with searchable switching, rename, and delete actions
- Today/Yesterday/Older grouping and timestamps in the Switch Session modal
- Automatic one-time migration of legacy `<workspace>/.cowork/sessions/` data into the new global location on first run
- `run_command` plan action: lets the planner invoke a small allowlist of local CLI tools (`git`, `pandoc`, `soffice`, `libreoffice`) so the bundled `pdf-converter` and `git-helper` skills can actually run real conversions/git commands instead of failing with "mcp not configured". Always treated as a dangerous step requiring approval; only bare executable names (no path) are accepted, traversal-looking arguments are rejected, and the process runs with the workspace root as its working directory under a 2-minute timeout
- `merge_pdf` and `split_pdf` plan actions: native PDF merge/split powered by `pdfcpu` (pure Go, compiled into the binary), so the `pdf-converter` skill can combine/split PDFs without pandoc, libreoffice, or any tool installed on the user's machine. `pdf-converter` skill rewritten to use these for PDF-to-PDF operations and reserve `run_command` (pandoc/libreoffice) for actual format conversion (Markdown/DOCX <-> PDF)

**Part 1 — Provider Correctness**

- `ProviderState` enum (`NotConfigured`, `Configured`, `Connected`, `Error`) with `String()` method for tracking provider connection status
- `Verifier` interface and `VerifyProvider()` helper: optional credential verification via lightweight API calls (GET `/models` for OpenAI/Gemini/Custom, minimal POST for Anthropic)
- `Verify()` implemented on all four providers: `OpenAIProvider`, `AnthropicProvider`, `GeminiProvider`, `CustomProvider`
- `FallbackChain.LastActive()`: tracks which provider actually handled the most recent request
- Fallback visibility: provider fallback events surfaced through Bubble Tea messages (`ProviderFallbackEvent`, `ProviderActiveEvent`) instead of suppressed stderr; right panel shows active provider and fallback reason
- `fallbackReason()` helper: classifies errors into human-readable reasons (auth error, rate limited, server error, timeout, etc.) instead of generic "unavailable"
- `OnFallback` callback now receives a `reason string` parameter with the actual error classification
- Provider verification wired into connect-provider flow: `VerifyProvider` runs async (10s timeout) before committing a provider switch; on failure the switch is rejected with an error message
- `AgentRunner.VerifyProvider` field: allows TUI session to verify providers without importing the provider package directly

**Part 2 — CustomProvider Activation + Hosted Presets**

- `HostedPresets` map with 4 OpenAI-compatible providers: OpenRouter, DeepSeek, Mistral, Groq (with default endpoints and env var names)
- `RequiresAPIKey` field on `ProviderPreset`: hosted services are marked as requiring an API key; `CustomProvider.Available()` checks this
- `createProvider` refactored: unknown provider names are matched against `HostedPresets` to create `CustomProvider` instances with correct endpoints
- `DefaultModels` extended with default models for all 4 new providers
- Real SSE streaming for `CustomProvider`: `text/event-stream` parsing with `data: [DONE]` sentinel (replaces fake single-chunk stream)
- SSE scanner buffer increased to 512 KiB (from default 64 KiB); `scanner.Err()` surfaced as `[stream error: ...]` message
- Grouped provider modal: "Native" and "OpenAI Compatible" section headers using disabled `ModalItem`s; group headers preserved during search filtering
- Real provider state hints in modal: `● current`, `configured`, `no API key`, `not configured`

**Part 3 — Connection Wizard, Model Discovery, and Local Providers**

- Four-step provider connection wizard: authentication method, masked key entry, credential verification, and model selection
- Environment-variable credential detection for all hosted provider presets
- OpenAI-compatible model discovery through the provider `/models` endpoint
- Automatic Ollama and LM Studio detection with installed-model discovery
- Local provider group in the Connect Provider modal with `detected` and `not detected` state hints
- `AgentRunner` callbacks for live provider configuration, verified-key persistence, provider verification, and model discovery
- Cross-platform **Open docs** command that opens the official Bolt Cowork documentation site
- Initial setup expanded to Anthropic, OpenAI, Gemini, OpenRouter, DeepSeek, Mistral, and Groq with grouped provider display
- Runtime `Redactor.AddSecret()` support for credentials added after application startup

### Fixed

- Binary files and terminal control sequences no longer corrupt the TUI
- Chat input and command palette remain stable after binary-file operations
- Switch-model modal refreshes its `current` marker after a model change
- Completed plan results remain visible when a new command starts
- Runtime model changes are authoritative for the active and newly created sessions
- New Session titles are no longer sent to the LLM as user prompts
- Chat input is now focused immediately when creating a blank session (Ctrl+P → New Session → Create) or reopening a saved session, instead of staying inert until the command palette was opened and closed again
- Provider credentials entered in the connection wizard are available to verification immediately and are persisted only after successful verification
- Invalid provider credentials no longer pollute the system keyring
- Keyring failures fall back to session-only credentials instead of blocking provider connection
- Environment-variable credentials are restored when configuration is reloaded
- Dynamically discovered hosted and local models are persisted in `ProviderConfig.Models`, keeping configuration valid after restart
- Fallback configuration now uses the exact model selected by the user
- Ollama and LM Studio no longer reuse the previously active hosted model when local discovery returns no models
- Short environment credential values no longer panic during masked display
- Model discovery handles `/chat/completions/` endpoints with a trailing slash
- Provider modal refreshes preserve the active search query and selected row after local detection completes
- Browser launcher child processes are reaped asynchronously
- Runtime-added credentials are included in output redaction

### Changed

- `OnFallback` signature changed from `func(from, to LLMProvider)` to `func(from, to LLMProvider, reason string)`
- Right panel PROVIDER section: single merged Status line (`⚡ ○ Idle (fallback)`) instead of duplicate Status lines
- `defaultProviderOrder` expanded from 3 to 9 entries (anthropic, openai, gemini, openrouter, deepseek, mistral, groq, ollama, lmstudio)
- `CustomProvider.Available()` now requires API key when `requiresAPIKey` is set (prevents unauthorized HTTP requests to hosted services)
- Initial setup and REPL provider commands now use the central `config.DefaultModels` catalog instead of a separate three-provider model map

### Tests

- `verify_test.go`: table-driven tests for `ProviderState.String()`, `Verify()` on all 4 providers (success + failure cases), `VerifyProvider()` helper
- `custom_test.go`: table-driven SSE stream parsing tests (normal, empty content, malformed JSON, non-data lines) + `Available()` with `RequiresAPIKey` combinations
- `fallback_test.go`: table-driven `LastActive()` (Chat + Stream), `OnFallback` callback with reason, `fallbackReason()` classifier
- `config_test.go`: `HostedPresets` endpoint/native/model coverage and environment credential reload tests
- `discovery_test.go`: table-driven model endpoint, model discovery, HTTP failure, malformed response, and local server detection tests
- `wizard_test.go`: credential verification/persistence, keyring fallback, safe key masking, local model selection, and empty-discovery tests
- `browser_test.go`: platform command selection for Windows, macOS, and Linux
- `session_test.go`: grouped modal layout, API key state hints, provider panel width, dynamic model persistence, and local provider configuration tests
- `modal_test.go`: search and selection preservation during asynchronous provider-list refresh
- `redact_test.go`: runtime secret registration tests
- All tests pass (`go test ./...` — 18 packages, 0 failures)

## [v0.4.3] - 2026-05-25

### Added

- `bubbles/spinner` integration in right panel PROVIDER section: animated spinner replaces static "● Running" while agent is active; reverts to "○ Idle" when done
- Streaming cursor: blinking `▌` character appended to the last assistant message while chunks are arriving (500ms blink via `cursorBlinkMsg` / `tea.Tick`); disappears when streaming ends
- Token progress bar in PROVIDER section: `[████░░░░░░] X.X%` rendered dynamically against the model's context window (claude→200k, gpt-4o→128k, gemini→1M, default→128k)
- Cost indicator in PROVIDER section: `Cost : $X.XXXX` updated after each streaming chunk using per-model input/output pricing table (anthropic: 3 models; openai: 9 models; gemini: 4 models)
- Plan step spinner animation: active step shows `[⠋]` (synced with session spinner) instead of `[ ]`; completed shows `[✓]`; failed shows `[✗]` — `SetActiveStep`/`SetSpinnerFrame` methods added to `PlanWidget`
- Skills paginator: `bubbles/paginator` shows max 8 skills per page in the Skills modal; `← →` navigation when `>8` skills loaded; page indicator in modal footer
- Mouse support: `tea.EnableMouseCellMotion()` enabled in `Session.Init()`; left-click outside modal or palette closes it; mouse wheel scrolls the chat viewport
- `cursorBlinkCmd()` helper: schedules the next 500ms blink tick and is rescheduled on every `cursorBlinkMsg`
- `contextWindowForModel(provider, model string)` helper: returns context window size by model name pattern
- `estimateChunkCost(provider, model string, tokens int)` helper: output-token cost estimate per chunk
- `formatCost(cost float64)` helper: `$X.XXXX` formatter
- `pricingTable` map: per-model input/output pricing in USD/1M tokens for all listed anthropic/openai/gemini models
- `skillsPerPage = 8` constant; `skillModalItems` now accepts a `page int` argument
- `streaming bool` and `cursorShow bool` fields added to `Session` struct
- `sessionCost float64` and `tokenByteCount int` fields added to `Session` struct
- `skillPaginator paginator.Model` field added to `Session` struct; initialized in `NewSession`
- TUI setup wizard (`internal/ui/views/setup.go`): step 1 provider selection, step 2 masked API key entry → stored to system keyring via `zalando/go-keyring`; launches automatically when `config.yaml` is absent
- "Trust this directory?" TUI modal on first run in a new workspace (`internal/config/trust.go`); `trusted_dirs` exact-match (sub-directories are not automatically trusted)

### Changed

- API keys removed from `config.yaml`; stored in system keyring (Windows Credential Manager / Mac Keychain / Linux Secret Service) via `zalando/go-keyring`; `yaml:"-"` tag ensures they are never written to disk
- Multi-provider config: `anthropic`, `openai`, `gemini` all defined in `config.yaml` with no API key fields
- `sandbox.denied_patterns` expanded from 3 to 28 security patterns
- `theme` field added to config (default: `dark`)
- `skills.dirs` updated: includes `cmd/bolt-cowork/skills` as bundled path
- `trusted_dirs` is written at runtime (first trust confirmation); hardcoded defaults removed
- `statusContent` PROVIDER section now renders spinner frame instead of static "● Running"
- `/clear` command now also resets `tokenByteCount` and `sessionCost`
- `buildChatBody` iterates messages with index to identify last assistant message for cursor placement

### Fixed

- Right panel status indicator no longer shows stale "● Running" after agent completes; spinner stops cleanly on `agentMsg{done: true}`
- Skills modal left/right arrow keys handled before forwarding to `modal.Update` to avoid keypress leak
- Mouse click outside modal no longer panics when `approvalCh` is nil

### Tests

- `session_test.go`: 14 new tests covering spinner status (running/idle), streaming cursor (on/off), `contextWindowForModel`, `estimateChunkCost`, `formatCost`, token progress bar content, cost content, skills pagination (with/without paginator), mouse click closes modal, mouse click closes palette, cursor blink toggle
- `go test ./...` passes

## [v0.4.2] - 2026-05-22

### Added

**Part 1 — Command Palette Overlay Redesign**

- Palette rewritten as a true ANSI-aware overlay: background session panels remain visible beneath the modal (`overlayCenter` + `overlayLine` using `charmbracelet/x/ansi` Truncate/TruncateLeft)
- Grouped command layout: four categories (Suggested, Session, Prompt, System) with category headers; Label + Shortcut columns
- Search filters on both `Name` and `Label` fields; 15 built-in commands, `DefaultCommands` exported flat list
- Status bar (`renderStatusBar`): `workspace:branch` left, `version` right; lipgloss background "237"
- `ctrl+x` chord system: `chordActive bool` in Session; second key (l/m/e/n/h/s/t) dispatches palette command directly
- `fetchGitBranch(workspace)`: `git symbolic-ref --short HEAD` primary, `git rev-parse --abbrev-ref HEAD` fallback
- `charmbracelet/x/ansi v0.11.6` promoted from indirect to direct dependency in `go.mod`
- `Chord` key binding added to `keys/keymap.go`
- `StepStartEvent{Index, Action, Desc}` UIEvent fires before each executor step
- `PermWarnEvent{Warning}` UIEvent fires on dangerous auto-approval in TUI mode

**Part 2 — Right Panel Live Sections + Git Dirty Indicator**

- Right panel redesigned with 5 live sections: PROVIDER, AGENT (step counter + active action), MCP (last tool call), PERMISSIONS (auto-approval warnings, shown only when triggered), SKILLS (loaded skill names)
- Git dirty indicator: `branch*` in status bar when `git status --porcelain` is non-empty; re-fetched async after each agent run via `fetchGitDirtyCmd`
- Narrow terminal collapse: below 80 cols the right panel is hidden, `[»]` shown in status bar instead
- `StepStartCallback` added to `internal/agent/agent.go` — `SetStepStartCallback(fn)` fires before each executor step so the TUI shows the active action in real time
- `onStepDone` signature extended to include `action string` (now 4-arg); `stepInfo()` helper prefixes MCP results with `server/tool:` for TUI identification
- `AgentRunner.LoadedSkills []string` wired from `store.GetAll()` in `buildTUIRunner` → displayed in right panel SKILLS section
- `parseMCPTool(info string)` extracts server/tool identifier; `firstLine(s string)` utility
- `tuiApprover.onPermWarn` callback fires `PermWarnEvent` to right panel PERMISSIONS section
- `clippedStatusContent(lines, w int)` replaces old `statusContent` for width-bounded right panel rendering
- New Session fields: `gitDirty bool`, `activeAction/activeTarget string`, `currentStep int`, `lastMCPTool/Status/Output string`, `lastPermWarn string`, `loadedSkills []string`

### Fixed

- `renderStatusBar` overflow guard: when version string exceeds terminal width, truncates to available space instead of overflowing
- `firstLine()` uses `strings.SplitSeq` (Go 1.24+) to avoid allocating a full slice

### Tests

- `palette_test.go` rewritten: uses `p.allCmds` / `p.filteredFlat`; added `TestPalette_filterByLabel`, `TestPalette_groupsPresent`
- `session_test.go`: `TestSession_RenderStatusBarClampsToWidth` guards narrow + version-wider-than-terminal cases; `TestSession_ReadMCPResourceEventTracksResourceIdentifier` added; `TestSession_StatusContentIncludesProviderName` added

## [v0.4.1] - 2026-05-22

### Added
- Agent integration: `AgentRunner` wired into `views/session.go` via `buildTUIRunner`; streaming output chunks via buffered channel + `waitNext(ch)` tea.Cmd pattern
- Spinner shown while agent runs without a plan (bubbles/spinner, styled with `theme.TitleStyle`)
- Plan viewer widget (`widgets/plan.go`): live `[ ]` → `[✓]` / `[✗]` step checkboxes driven by `PlanReadyEvent` / `StepDoneEvent` callbacks
- Execution log: one line per completed step, `✓`/`✗` prefixed, accumulated below the plan widget
- Right panel live info: provider, model, cumulative token estimate (len(chunk)/4), `● Active` / `○ Idle` status, workspace directory
- Command palette (`Ctrl+P`): `widgets/palette.go` overlay widget with prefix filtering, arrow-key navigation, Enter to execute, Esc to dismiss
- `PaletteSelectMsg{Command}` / `PaletteCloseMsg{}` bubbletea message types for palette→session communication
- `AgentRunner.ApprovalMode` field wired from `cfg.ApprovalMode` in `buildTUIRunner`
- REPL commands migrated to palette: `/clear`, `/model`, `/dir`, `/approval`, `/help`, `/quit`
- `internal/ui/widgets/palette_test.go`: 8 table-driven tests (defaults, filtering, navigation, Enter/Esc messages, View non-empty)

### Fixed
- Agent state correctly reset on each new run (`planActive`, `planSteps`, `stepDone`, `stepErrors`, `execLog`, `tokenCount`)
- Right panel content anchored to top via `AlignVertical(lipgloss.Top)`; no overflow past fixed panel height
- `chatContent` scroll window anchors the `> input` row to the bottom regardless of message count
- `tokenCount` resets to 0 at the start of each new run

## [v0.4.0] - 2026-05-22

### Added
- charmbracelet/bubbletea, lipgloss, bubbles, glamour dependencies
- `internal/ui/` package structure:
  - `app.go`: root App model, view switching (Welcome → Session)
  - `keys/keymap.go`: quit and palette key bindings
  - `theme/theme.go`: centralized lipgloss color and style definitions
  - `views/welcome.go`: welcome screen — centered title, text input, git branch + version status bar
  - `views/session.go`: split layout placeholder (70% chat / 30% status)
  - `panels/`: chat, status, input (bubbles/textinput), statusbar
  - `widgets/`: spinner (bubbles/spinner), plan (glamour fallback), approval, palette
- Window size propagation: App stores tea.WindowSizeMsg and seeds Session on view switch
- Git branch detection scoped to configured workspace directory

### Changed
- `cmd/bolt/main.go`: REPL startup replaced with `ui.New(cfg, version).Run()`
- `getGitBranch` now accepts `workDir` parameter, reads branch for the correct repository

### Removed
- `github.com/chzyer/readline` dependency
- All readline references from codebase and documentation

### Fixed
- Session.View() blank on first frame (window size seeded before first render)
- glamour renderer errors now fall back to raw plain text instead of empty string
- Stale readline comment in main.go

## [0.3.5] - 2026-05-19

### Added
- MCP approval gate with configurable mode (full / plan-only / dangerous-only / none)
- `--mcp-approval` CLI flag and `mcp_approval_mode` config file field
- Runtime ConnectionStatus tracking for MCP servers (connected / disconnected / error)
- `/mcp list` REPL command: shows all configured servers with live status
- `/mcp tools` REPL command: lists tools grouped by server

### Fixed
- `/mcp list` no longer shows empty results when servers are configured
- "connected" status now reflects actual connection result, not the Enabled flag

## [v0.3.4] - 2026-05-18

### Added
- MCP client: ListTools, CallTool, DiscoverTools methods
- ToolRegistry with composite serverName/toolName key
- Deep copy (cloneTool) for InputSchema.Properties and Required slice
- ReplaceServerTools for atomic tool refresh on re-discovery
- CallMCPToolAction: integrates into planner, executor, approval gate
- Executor registry validation: unregistered tools rejected before MCP call
- Agent.SetMCPToolRegistry public API for schema injection
- Tool schemas injected as sanitized JSON block in system prompt

### Fixed
- PendingRegistry blocking bug: CloseAll() on Disconnect/Connect/readLoop
- Race condition: Register() rejects new channels after CloseAll()
- Stale tools removed on connection replacement and re-discovery

### Tests
- 174 -> 210+ tests, go test ./... PASS

## [0.3.3] - 2026-05-18

### Added
- `internal/mcp/types.go`: JSON tags on all existing types; new wire-protocol types
  (Tool, ToolSchema, ToolProperty, CallToolResult, ToolResultContent), lifecycle/handshake
  types (InitializeParams, InitializeResult, ClientInfo, ServerInfo, ServerCapabilities,
  ToolsCapability, ResourcesCapability)
- `internal/mcp/loader.go`: LoadConfig (tilde expansion, missing-file tolerance),
  DefaultConfigPath, expandTilde; `var userHomeDir` injectable for test isolation
- `internal/mcp/normalize.go`: NormalizeConfig — trim whitespace, lowercase transport,
  transport-aware validation (sse requires URL, stdio/unspecified requires Command),
  unknown-transport rejection, deduplication by name
- `internal/mcp/registry.go`: LoadFromConfig and LoadFromFile convenience methods

### Tests
- `internal/mcp/types_test.go`: 7 table-driven test functions covering JSON serialization,
  omitempty, and round-trip fidelity for all new and updated types
- `internal/mcp/loader_test.go`: 5 table-driven test functions, 33 subtests covering
  LoadConfig, DefaultConfigPath, NormalizeConfig, Registry.LoadFromConfig,
  Registry.LoadFromFile

## [0.3.2] - 2026-05-17

### Added
- `internal/mcp/jsonrpc.go`: JSON-RPC 2.0 types (Request, Response,
  Notification, RPCError), typed ID struct with unicode-safe Key(),
  IDGenerator (atomic, never reuses), PendingRegistry (chan semaphore,
  duplicate detection), NotificationDispatcher (RWMutex, re-entrant)
- `internal/mcp/transport.go`: Transport interface (Send/Receive/Close)
  with context cancellation contract
- `internal/mcp/stdio.go`: StdioTransport with Content-Length framing,
  `chan struct{}` semaphores for cancellable lock acquisition,
  `context.AfterFunc` for cancellable blocking I/O
- `internal/mcp/process.go`: StartProcess helper via exec.CommandContext

### Tests
- 78 tests passing (60 JSON-RPC + 18 stdio), go vet clean

## [0.3.1] - 2026-05-15

### Added
- Trust prompt mechanism: first-run directory trust check
- `trusted_dirs` config field with subdirectory inheritance
- `/dir` command trust gate (blocks switching to untrusted dirs)
- `AddTrustedDir` respects `--config` flag
- Cross-platform binary build via `scripts/build.go`
- `make release`: 5 platform binaries (windows/amd64, linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64)
- GitHub Actions release workflow (tag push triggers build + upload)
- `make lint` now checks gofmt formatting
- CONTRIBUTING.md full rewrite (9 sections)
- Issue templates: config snippet field, target version field
- PR template: updated checklist, how-to-test section

### Fixed
- Makefile shell dependency removed (Go build script replaces POSIX commands)
- Version injection works on Windows (moved from Makefile to `scripts/build.go`)
- `TestDirCommand_TildeExpansion` test isolation (fake home dir)

## [0.3.0] - 2026-05-12

### Fixed
- Path boundary detection: `strings.HasPrefix` replaced with `filepath.Rel`-based `IsUnderDir`
  in `loader.go` and `repl.go` scope detection — prevents false positives where `/home/me2`
  incorrectly matched home prefix `/home/me`

### Added
- `SkillMetadata`: `version` and `category` frontmatter fields
- Bundled skills updated with tags, category, version
- Hybrid skill matcher: tag-aware scoring, LLM disambiguation fallback
- `MatchResult` type and `LLMDisambiguator` interface
- Skill registry: tag search, category listing/filtering, general-purpose search
- Default skills: code-reviewer, git-helper, project-scaffolder, pdf-converter
- `/skill create` interactive command for custom skill generation
- `sandbox.IsUnderDir(parent, child string) bool` — exported helper for correct path containment
  checks across packages; uses `filepath.Rel` to avoid prefix collisions
- `sandbox.WrapFSError(op, path string, err error) error` — user-friendly filesystem error
  messages for permission denied, file not found, file locked, and other OS errors
- Integration tests with realistic Go project fixture (`testdata/fixtures/sample-go-project`);
  run with `go test ./internal/sandbox/ -tags=integration -v`
- `make test-integration` target for running integration test suite

## [0.2.6] - 2026-05-05

### Security
- Protected path case-insensitive matching on Windows (F-005)
- NTFS Alternate Data Stream blocking on Windows (F-014)
- Unified `resolveAndCheckProtected` helper covering all actions with symlink resolution
- `ApproveAll` respects `dangerous-only` mode — dangerous steps always prompt

### Added
- `--version` flag
- `isReservedFilename`: block Windows reserved names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
- `maxWriteContentBytes`: 1 MB limit for single write actions
- E2E tests: plan rejection, result approval, approve-all full mode, skill rejection
- VHS demo tape (`demo.tape`) and `demo.gif` for README demo animation

### Fixed
- Plan revision feedback prompt now visible (F-012)
- `/dir` resolves relative to workspace, tilde expansion, `filepath.Clean` normalization (F-008)
- `--dir /nonexistent` exits with error instead of opening REPL (F-001)
- Error messages: lowercase start, no trailing periods
- Startup sequence: banner → status → warnings → help hint (Info lines moved below status line)

### Changed
- Go 1.25 → 1.26
- Banner reverted to original Unicode BOLT logo
- Startup sequence: banner → status → warnings → help hint
- Removed unused `colorRed`, `colorCyan`, `readREPLLine` functions
- `initSkillStore` returns warnings instead of printing them directly
- Roadmap v0.6 updated: GUI (Web UI) → TUI (charmbracelet/bubbletea) + Electron Desktop App

## [0.2.5] - 2026-05-01

### Added
- Secret redaction tests: Redactor struct with dedup and substring replacement (8 tests)
- Protected path tests: read/write/delete denied, traversal and symlink blocked (7 tests)
- Permission reason tests: delete, overwrite, outside sandbox, safe actions, format (5 tests)
- Agent e2e scenario tests: simple create, read+write, dangerous approval/rejection, multi-step, invalid action, skill injection (7 tests)
- Skill parser edge case tests: unicode, large body, multiple delimiters, whitespace, empty file, frontmatter-only, tabs, duplicate keys (8 tests)
- MCP config validation tests: valid full/minimal, missing name/URL, invalid transport, duplicate name, empty list, unknown fields, invalid value type (9 tests)
- Added .ssh/*, .gnupg/*, .config/bolt-cowork/* to protected paths

### Fixed
- nil context in executor_test.go replaced with context.Background() (SA1012)
- TestPermissionReason_ShellCommand skipped — ActionShell not yet implemented, deferred to v0.3+

## [0.2.4] - 2026-05-01

### Added
- Tool Interface: ToolDef() method, tool registry for LLM function calling
- Typed Action / ActionResult model: structured action dispatch with type safety
- Command Registry pattern: slash command registration and dispatch
- MCP skeleton: types, config, registry for Model Context Protocol integration
- AppState struct: centralized application state management
- System Prompt Builder: dynamic system prompt assembly with skill context
- Provider interface tools parameter: LLM providers accept tool definitions
- SkillMetadata struct: name, description, tags, priority, requires_approval
- SkillScope enum: Bundled, Global, Project with override order
- parseFrontMatter: YAML frontmatter + Markdown body extraction
- descriptionFallback: first paragraph truncation at 512 chars
- nameFromPath: filename to skill name derivation
- CRLF normalization in frontmatter parser (P1 fix)
- LoadAll scope assignment with project > global > bundled override

### Fixed
- Binary path skill directory resolution bug
- Protected path list validation
- Codex review suggestions applied

## [0.2.3] - 2026-04-29

### Added
- `/dir` command: show or change workspace directory with `/dir [path|-]`
- Context trimming: LLM calls limited to last 20 messages / 32K chars
- `dangerReason()` helper: `[DANGEROUS]` now shows why (e.g., "permanently removes file")
- `displayPath()` helper: user-facing paths shown as workspace-relative
- `friendlyError()` helper: sandbox errors converted to actionable messages
- Revise flow regression test (`TestAgent_ReviseFlow`)
- Global skill directory warnings for missing dirs, bad YAML, name conflicts, empty files

### Fixed
- Unsupported action types return error (not nil), preventing false success
- `dangerReason` validates paths inside sandbox before `os.Stat` (P0 security fix)
- `/dir -` validates against `allowed_dirs` before switching (P0 security fix)
- Unicode em dash replaced with ASCII hyphen in dangerous prompt
- `trimHistory` reserves slot for summary message (stays within `MaxContextMessages`)
- `SkillStore` interface updated to match new `LoadAll []string` signature
- Single-command mode uses new `LoadAll` warning contract
- `TestInitProject` setup closures use subtest `t`

### Changed
- Executor error messages wrapped with `friendlyError` for all action types
- `LoadAll` returns `[]string` warnings instead of `error`

## [0.2.2] - 2026-04-27

### Added
- `/mode` command: `/mode plan|build|strict|none` shortcuts for approval modes
- Auto-create provider on `/key set` when provider doesn't exist in config
- Default provider auto-set when first provider is added via `/key set`
- `spinner.go`: ASCII spinner with TTY detection
- `color.go`: ANSI color helpers respecting `NO_COLOR` and `TERM=dumb`
- "bolt cowork" (missing hyphen) typo guard with actionable message
- Single-char / all-digit input guard
- `isAllDigits()` helper near `suggestSlashCommand`

### Fixed
- Zero-step conversational responses now displayed instead of generic warning
- Banner `T` character alignment
- `/mode build` description accurately reflects `dangerous-only` semantics
- `/init` rejects extra arguments (`Usage: /init [force]`)
- `/config` unknown subcommand now suggests `/config` for help
- `captureBoth` test helper now handles pipe errors

### Changed
- `handleModelCommand`: terse provider warning replaced with 2-step instructions
- `project_init_test.go` refactored to table-driven subtests
- "bolt-cowork" text removed from REPL startup (banner already shows it)

## [0.2.1] - 2026-04-27

### Added
- Deterministic `/init` command — creates `.cowork/` structure (config.json, keyset.json, sessions/) without LLM
- `/init force` to reinitialize (overwrite existing files)
- Bare `init` and `bolt-cowork init` intercepted in REPL before reaching agent
- Subcommand hierarchy: `/config`, `/skill`, `/key` print subcommand list on bare enter
- Grouped `/help` output (General, Config, Skills, Provider & Model, Workspace)
- ASCII banner logo at REPL startup
- GitHub issue templates (bug report, feature request) and PR template

### Fixed
- Bundled skills now embedded in binary via `go:embed`
- REPL loads embedded skills on startup
- Banner double "v" prefix when version string already starts with `v`
- Banner typo: "Platfom" → "Platform"

### Changed
- `skills/` moved to `cmd/bolt-cowork/skills/` for `go:embed` compatibility
- Skill docs aligned: approval stage options documented as Approve/Reject only; load order (bundled → global → project-local) documented

## [0.2.0] - 2026-04-25

### Added
- Skill system: SKILL.md loading with YAML frontmatter parser
- 3-tier skill loading: bundled (`skills/`) > global (`~/.bolt-cowork/skills/`) > project-local (`./bolt-skills/`)
- Keyword-based skill matching with stop words filter
- Skill context injection into planner system prompt (`<active_skills>` XML block)
- Skill approval gate in full approval mode
- `/skills` command: list all loaded skills
- `/skill <name>` command: show skill details
- `/use <name>` command: one-shot manual skill activation
- Default skills: file-organizer, summarizer

### Fixed
- `plan-only` mode no longer prompts for skill approval (skill stage is excluded)
- Unknown forced skill names logged as warnings instead of silently ignored

## [0.1.8] - 2026-04-24

### Fixed
- **Ctrl+C killing REPL** -- signal canceller added to both readline and fallback REPL paths; Ctrl+C now cancels the running command and returns to prompt.
- **dangerous-only mode not requiring write approval** -- `isDangerous()` now treats all non-read actions (write, delete, move, rename, copy, mkdir) as dangerous.
- **`..hidden` sandbox bypass** -- `isWithinAllowed()` and `handleDirCommand` now correctly allow directories whose names start with `..` (e.g. `..hidden`) while still blocking actual traversal.
- **Provider fallback not triggering on 401/403** -- `Retryable()` now includes `http.StatusUnauthorized` and `http.StatusForbidden`; invalid API keys cause automatic fallback to the next provider.
- **Delete intent recursive ambiguity** -- planner system prompt now includes explicit rules: `recursive` defaults to `false` unless the user explicitly requests recursive or directory deletion.
- **Conversation memory not working on meta-queries** -- planner returns empty `steps` array for meta-questions; agent skips execute/result stages and replies directly from description.
- **Tilde not expanding in config paths** -- `expandTilde()` added to `config.go`; `LoadFile()` now expands `~` in `allowed_dirs`, `read_only_dirs`, and `skills.dirs`.

### Changed
- Test checklists and review reports moved to `docs/testing/`.
- Removed temp directories `.codex_tmp_manual` and `.gemini_test_env`.

## [0.1.7] - 2026-04-21

### Added
- **OpenAI provider** -- real HTTP implementation replacing the stub. Supports `gpt-4o`, `gpt-4o-mini`, `o3-mini` via Chat Completions API.
- **Gemini provider** -- new `internal/provider/gemini.go`. Supports `gemini-2.5-pro`, `gemini-2.5-flash` via generateContent API with `systemInstruction` field.
- **Conversation history** -- multi-turn context in REPL. Agent accumulates user/assistant messages with FIFO cap at 20 turns (40 messages).
- `/clear` command -- resets conversation history in REPL.
- **Cross-provider `/model` switching** -- `detectProvider()` infers provider from model name prefix (`claude-`/`gpt-`/`gemini-`). No need to specify provider manually.
- Init wizard option 3 (Google Gemini) and option 4 (All providers).
- Tab completion for new models (`gpt-4o`, `gpt-4o-mini`, `gemini-2.5-pro`, `gemini-2.5-flash`) and `/clear`.
- 30+ new tests: OpenAI HTTP, Gemini role mapping, conversation history, provider detection, three-provider fallback.

### Changed
- `CreatePlan()` accepts conversation history parameter for multi-turn context.
- `run()` returns updated `[]types.Message` history for REPL persistence.
- `/help` output updated with new providers and `/clear`.
- `/key set` tab completion includes `gemini`.

## [0.1.6] - 2026-04-19

### Added
- **Readline integration** via `chzyer/readline` -- tab completion for slash commands and model names, persistent command history (`~/.bolt-cowork/history`), line editing (Home/End, Ctrl+A/E, Ctrl+W).
- `/config` command -- displays current config with API keys masked.
- `/config path` -- shows config file path.
- `/config reload` -- reloads config from disk without restarting REPL.
- `/dir` command -- shows current working directory (resolved absolute path).
- `/dir <path>` -- changes working directory at runtime (must be within allowed dirs).
- `RevisionPrompter` interface -- enables plan revision to collect user feedback.
- Maximum 3 revision attempts per command to prevent infinite loops.
- Fallback REPL mode for piped stdin or when readline init fails.
- Tests for revision feedback injection and maxRevisions boundary.

### Changed
- REPL input switched from `bufio.Scanner` to `readline.Instance` -- all user input reads through a single source to avoid stdin conflicts.
- `/key set` uses readline password mode (masked input) when readline is active.
- `/help` output updated with new commands.
- Tab completer tree covers subcommands (e.g., `/model haiku|sonnet|opus`, `/config path|reload`).

## [0.1.5] - 2026-04-19

### Added
- **New action types:** `delete` (with recursive flag), `move`, `copy`, `mkdir`.
- **Read-only directories** -- `read_only_dirs` config field.
- `DeletePath(path, recursive)`, `CopyFile(src, dst)`, `MkdirAll(path)` sandbox methods.
- Intent verification in plan stage with retry mechanism.
- Interactive path selection for ambiguous delete targets.
- User-friendly "not found" error messages with path suggestions.
- Windows REPL line-editing fix via terminal cooked mode.
- Build-time version injection via `git describe` + ldflags in Makefile.
- GitHub Actions CI workflow -- test, vet, build on push/PR.
- Dependabot config for Go module updates.

### Changed
- `ActionDelete` now uses `DeletePath` with recursive support.
- Planner system prompt updated with all action types and JSON format examples.
- `denied_patterns` now enforced across all action types.

### Fixed
- `DeleteFile` and `DeletePath` fail-open removed -- errors now returned instead of silently ignored.
- `RenameFile` read-only error message corrected.

## [0.1.4] - 2026-04-14

### Added
- Table-driven tests for `resolvePath` covering join semantics, path traversal rejection, and `..hidden` edge cases.
- `TestExecutor_ReadTruncation`, `TestExecutor_WriteEmptyContent`, `TestExecutor_UnsupportedAction`.
- Levenshtein distance utility with table-driven tests.
- Slash command typo suggestions for unknown commands with edit distance <= 2.
- `[auto]` label in plan display for read-only actions under `dangerous-only` mode.
- Startup banner shows resolved absolute working directory.

### Changed
- **`resolvePath` rewritten** -- removed prefix-stripping, narrow traversal check.
- Approval modes clarified: `full` (default), `dangerous-only`, `none`.
- Read actions return file contents (200-line truncation).
- Write actions validate non-empty content.
- All system messages in English.
- ASCII-only terminal output.

### Fixed
- Path duplication bug resolved at prompt level.
- `Ctrl+C` / EOF prints `Command cancelled.` and returns to REPL.
- `..hidden` paths no longer falsely rejected.
- `filepath.Abs` errors handled in startup banner.

## [0.1.0] - 2026-04-03

### Added
- Initial release: sandbox, config, LLM provider interface with fallback chain, agent loop with approval gates, CLI, Anthropic provider.
- 64+ tests across all packages.

[v0.4.3]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.7...v0.4.0
[0.3.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.4...v0.3.5
[v0.3.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.6...v0.3.0
[0.2.6]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.5...v0.2.6
[0.2.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.8...v0.2.0
[0.1.8]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.0...v0.1.4
[0.1.0]: https://github.com/halukerenozlu/bolt-cowork/releases/tag/v0.1.0
