# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.6...HEAD
[0.1.6]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.0...v0.1.4
[0.1.0]: https://github.com/halukerenozlu/bolt-cowork/releases/tag/v0.1.0
