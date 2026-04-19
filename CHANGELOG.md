# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.5] - 2026-04-19

### Added
- Read-only directory support in sandbox config (`sandbox.read_only_dirs`) with enforcement across write/delete/move/rename/copy/mkdir operations.
- New planner/executor actions: `copy` and `mkdir`.
- Delete target disambiguation flow:
  - optional `PathSelector` interface and `PathSelectionRequest` model
  - interactive CLI target selection before execute approval when delete path is missing
- New regression tests for:
  - delete target selection (ambiguous/single-candidate/no-candidate)
  - content-delete intent validation
  - mkdir "already exists" behavior

### Changed
- Plan intent validation in `planStage` now retries planning when generated steps do not match user intent (delete/copy/move/mkdir/content semantics).
- "Delete file content" style commands are treated as content intent (not hard-required to include an `ActionDelete` step).
- Not-found UX improved with friendlier user output and suggestion listing.
- Windows REPL input handling now preserves native cooked-mode line editing behavior (arrow keys, Home/End, paste shortcuts).

### Fixed
- Removed unsafe single-candidate auto-delete target rewriting; explicit path selection is now required for unresolved delete targets.
- `DeleteFile` and `DeletePath` now fail closed on `filepath.Abs` / `resolvePath` errors instead of ignoring them before read-only checks.

## [0.1.4] - 2026-04-14

### Added
- Table-driven tests for `resolvePath` covering join semantics, path traversal rejection, and edge cases like `..hidden` directories.
- `TestExecutor_ReadTruncation` — verifies files longer than 200 lines are truncated with a `[truncated]` marker.
- `TestExecutor_WriteEmptyContent` — verifies write actions with empty content are rejected and no file is created.
- `TestExecutor_UnsupportedAction` — verifies unsupported action types return a descriptive error instead of panicking or silently skipping.
- Levenshtein distance utility (`internal/agent/levenshtein.go`) with table-driven tests.
- Slash command typo suggestions ("Did you mean '/model'?") for unknown commands with edit distance ≤ 2.
- `[auto]` label in plan display for read-only actions under `dangerous-only` approval mode.
- Startup banner now shows the resolved absolute working directory instead of the raw flag value.

### Changed
- **`resolvePath` rewritten** — removed prefix-stripping logic entirely. Paths from the LLM are now joined to the sandbox root as-is, with path traversal protection via a narrowed check (`rel == ".."` or `rel` starting with `".." + separator`). Duplicate-prefix avoidance is now handled in the planner system prompt rather than in path resolution.
- Approval modes clarified:
  - `full` (default) — every step requires approval, including reads and lists.
  - `dangerous-only` — read/list actions auto-approve, write/execute require approval.
  - `none` — no approvals requested.
- Read action results now include actual file contents (up to 200 lines, remainder marked `[truncated]`) instead of only byte counts.
- Write actions now persist the `content` field from the plan to disk (previously produced 0-byte files).
- All user-facing system messages translated from Turkish to English (`Plan rejected.`, `Execution stopped.`, `Result rejected.`, etc.).
- Non-ASCII em-dash characters in executor output replaced with ASCII hyphens to avoid Windows terminal mojibake.
- Header/banner no longer reprinted on every command — shown only once at REPL startup or before single-command execution.
- Unsupported action errors now return `unsupported action type: <type>` instead of generic "unknown action" text.

### Fixed
- **Path duplication bug** — planner occasionally produced paths like `workspace/workspace/file.txt`; resolved at the prompt level with an explicit instruction not to repeat the working directory name.
- `Ctrl+C` / EOF during plan or execute approval now prints `Command cancelled.` and returns cleanly to the REPL instead of surfacing a raw `read input: EOF` error. Non-EOF I/O errors are wrapped and returned rather than silently mapped to rejection.
- `..hidden/file.txt` and similar legitimate paths starting with `..` are no longer falsely rejected as path traversal.
- `filepath.Abs` errors in the startup banner are now handled — falls back to the resolved path with a warning instead of silently printing an empty string.

## [0.1.0] - 2026-04-03

### Added
- Initial release: sandbox, config (YAML + env var expansion), LLM provider interface with fallback chain, agent loop with approval gates, CLI, Anthropic provider.
- 64+ tests across all packages.

[Unreleased]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.0...v0.1.4
[0.1.0]: https://github.com/halukerenozlu/bolt-cowork/releases/tag/v0.1.0
