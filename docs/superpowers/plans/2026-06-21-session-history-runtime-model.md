# Session History and Runtime Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve every conversation turn in the TUI, make the selected runtime model authoritative across new sessions, and provide persistent searchable session switching.

**Architecture:** Introduce a project-scoped session store under `.cowork/sessions/` with atomic JSON writes and validated opaque IDs. Move session ownership to the root UI App so it can save, list, open, rename, and delete sessions while sharing one mutable runtime provider/model state. Represent rendered conversations as completed turns plus one active run so earlier plans, execution logs, and answers remain visible.

**Tech Stack:** Go 1.26+, Bubble Tea, standard-library JSON/filesystem/time APIs, table-driven tests using `t.TempDir()`.

---

### Task 1: Completed conversation turns remain visible

**Files:**
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/views/session_test.go`

- [x] Add a failing test with two completed plan runs that expects both plans, execution logs, and final answers in `buildChatBody`.
- [x] Freeze completed plan state into an assistant display message containing the plan, completion state, execution log, and final answer.
- [x] Archive the active run into a completed turn before starting the next user command.
- [x] Render completed turns in order and render the active turn last.
- [x] Run `go test ./internal/ui/views -count=1`.

### Task 2: Runtime model is one authoritative state

**Files:**
- Modify: `internal/ui/views/runner.go`
- Modify: `internal/ui/views/session.go`
- Modify: `internal/ui/app.go`
- Modify: `cmd/bolt-cowork/main.go`
- Test: `internal/ui/views/session_test.go`
- Test: `internal/ui/app_test.go`
- Test: `cmd/bolt-cowork/main_test.go`

- [x] Add a failing App-level test: select Haiku, create a new session, and expect the new Session/status to use Haiku.
- [x] Add a runtime model-change message carrying provider and model from Session to App.
- [x] Store provider/model in App and rebuild AgentRunner metadata before each new/opened session.
- [x] Ensure agent execution reads the selected model from current config and report persistence errors to chat.
- [x] Run `go test ./internal/ui ./internal/ui/views ./cmd/bolt-cowork -count=1`.

### Task 3: Secure persistent session store

**Files:**
- Create: `internal/session/store.go`
- Create: `internal/session/store_test.go`

- [x] Add failing table-driven tests for create/save/load/list/rename/delete under `t.TempDir()`.
- [x] Define versioned JSON records with ID, title, timestamps, provider, model, history, and display turns.
- [x] Generate IDs from `crypto/rand`; validate IDs before path construction.
- [x] Write through a same-directory temporary file and atomic rename with `0600` file permissions.
- [x] Ignore unrelated/corrupt files during list while returning contextual errors for direct load.
- [x] Run `go test ./internal/session -count=1`.

### Task 4: App-managed session lifecycle

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/views/runner.go`
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/app_test.go`
- Test: `internal/ui/views/session_test.go`

- [x] Add failing tests for saving the active session before new-session creation and restoring a selected session.
- [x] Add App messages for create, switch, rename, and delete operations.
- [x] Save active session after every completed agent run and before switching away.
- [x] Restore messages, completed turns, history, counters, provider, and model into a Session.
- [x] Keep session titles derived from the first prompt unless explicitly renamed.
- [x] Run `go test ./internal/ui ./internal/ui/views -count=1`.

### Task 5: Searchable Switch Session UX

**Files:**
- Modify: `internal/ui/widgets/modal.go`
- Modify: `internal/ui/views/session.go`
- Test: `internal/ui/widgets/modal_test.go`
- Test: `internal/ui/views/session_test.go`

- [x] Add failing tests for session rows containing title, time, and active marker.
- [x] Populate Switch Session from store summaries instead of the current-session placeholder.
- [x] Group summaries by Today, Yesterday, and Older through disabled heading rows.
- [x] Support filtered search and Enter-to-open without selecting headings.
- [x] Add keyboard actions for rename and delete with confirmation.
- [x] Run `go test ./internal/ui/widgets ./internal/ui/views -count=1`.

### Task 6: Verification and merge

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `AGENTS.md`

- [x] Document persistent sessions, turn history, and authoritative runtime model behavior.
- [x] Run `gofmt` on changed Go files.
- [x] Run `go test ./... -count=1`.
- [x] Run `go vet ./...`.
- [x] Build `./cmd/bolt-cowork`.
- [ ] Commit with Conventional Commits.
- [ ] Merge the branch into `master` and rerun `go test ./... -count=1`.
