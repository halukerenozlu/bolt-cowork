# TUI Binary Output, Scrollbar, and Model State Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent file contents from corrupting the terminal, add safe file inspection capabilities, make chat scrolling visibly mouse-draggable, and keep model selection state truthful.

**Architecture:** Treat filesystem bytes as untrusted at the executor boundary: textual reads receive bounded previews and binary reads receive metadata rather than raw bytes. Add explicit metadata/hash actions so discovery tasks do not require reading full files. Keep the viewport as the scroll owner, but translate scrollbar mouse coordinates into viewport offsets and rebuild model modal items from current session state whenever opened.

**Tech Stack:** Go 1.26+, Bubble Tea, bubbles/viewport, lipgloss/x/ansi, standard-library MIME/UTF-8/hash packages, table-driven Go tests.

---

### Task 1: Safe file output boundary

**Files:**
- Modify: `internal/agent/executor.go`
- Test: `internal/agent/executor_test.go`
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/views/session_test.go`

- [ ] Add table-driven executor tests proving binary files and terminal control sequences are never returned as executable terminal content.
- [ ] Run the focused tests and confirm they fail against the current raw `string(data)` behavior.
- [ ] Implement binary detection, bounded text previews, and control-character sanitization while preserving newline and tab formatting.
- [ ] Add a TUI rendering regression test containing escape, carriage-return, backspace, and invalid UTF-8 bytes.
- [ ] Run `go test ./internal/agent ./internal/ui/views -count=1`.

### Task 2: File metadata and duplicate-friendly hashing

**Files:**
- Modify: `internal/agent/planner.go`
- Modify: `internal/agent/executor.go`
- Test: `internal/agent/executor_test.go`
- Test: `internal/agent/planner_test.go`

- [ ] Add failing table-driven tests for file metadata and SHA-256 actions using only `t.TempDir()`.
- [ ] Add action constants/schema validation for metadata and hash operations.
- [ ] Implement sandboxed metadata and streaming SHA-256 execution without loading whole files into chat output.
- [ ] Update planner instructions so size searches use metadata and duplicate searches compare size before hashes.
- [ ] Run `go test ./internal/agent -count=1`.

### Task 3: Stable viewport and draggable scrollbar

**Files:**
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/views/session_test.go`

- [ ] Add failing tests proving the input remains on the final panel row with hostile content.
- [ ] Add failing tests mapping scrollbar click/drag positions to top, middle, and bottom viewport offsets.
- [ ] Track scrollbar drag state and translate left-button press/motion/release events only when the pointer is inside the scrollbar column.
- [ ] Keep ordinary clicks available for modal/palette closing and retain wheel/PageUp/PageDown behavior.
- [ ] Run `go test ./internal/ui/views -count=1`.

### Task 4: Truthful model selection

**Files:**
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/views/session_test.go`
- Test: `cmd/bolt-cowork/main_test.go`

- [ ] Add a failing test that selects Haiku, reopens Switch model, and expects Haiku—not Opus—to carry `current`.
- [ ] Rebuild model modal items from the current runner/config every time the modal opens.
- [ ] Add a runner-level test proving the next provider chain is constructed with the newly selected model.
- [ ] Run `go test ./internal/ui/views ./cmd/bolt-cowork -count=1`.

### Task 5: Full verification and integration

**Files:**
- Modify: `README.md`, `AGENTS.md`, `CHANGELOG.md`

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./... -count=1`.
- [ ] Run `go test -race ./internal/agent/... ./internal/ui/...`.
- [ ] Run the repository lint command if available.
- [ ] Verify the user's pre-existing `README.md` roadmap-heading edit remains intact and is not included in the fix commit.
- [ ] Commit with Conventional Commits.
- [ ] Switch to `master`, merge the fix branch, and rerun focused smoke tests.
