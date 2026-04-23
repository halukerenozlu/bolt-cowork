# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.8...HEAD
[0.1.8]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/halukerenozlu/bolt-cowork/compare/v0.1.0...v0.1.4
[0.1.0]: https://github.com/halukerenozlu/bolt-cowork/releases/tag/v0.1.0
