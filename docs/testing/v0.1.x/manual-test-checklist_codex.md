# bolt-cowork v0.1.7 Manual Test Checklist (Codex Working Copy)

Source of truth remains: `manual-test-checklist.md` (unchanged).

## Legend
- `[ ]` Test not executed yet
- `[x]` Test passed
- `[!]` Test failed (detail required in report)
- Owner tags:
  - `[CODEx]` Codex can execute without your direct intervention
  - `[JOINT]` We should execute together (interactive/decision points)
  - `[MANUAL]` Best done by you manually on real terminal UX

---

## A) Codex-Owned Items (Automatable)

### Setup & Startup
- [x] [CODEx] `make install` succeeds

### REPL Basics
- [x] [CODEx] `/quit` exits cleanly
- [x] [CODEx] Unknown command typo suggestion: `/modle` -> "Did you mean '/model'?"
- [x] [CODEx] Completely unknown command: `/xyz123` -> generic help message

### Model & Key / Config / Dir
- [x] [CODEx] `/model` shows current model
- [x] [CODEx] `/model sonnet` switches model/provider correctly
- [x] [CODEx] `/model gpt-4o` switches model/provider correctly
- [x] [CODEx] `/model gemini-2.5-pro` switches model/provider correctly
- [x] [CODEx] `/model nonexistent` shows error
- [x] [CODEx] `/config` masks keys
- [x] [CODEx] `/config path` shows config path
- [x] [CODEx] `/config reload` handles valid/invalid updates cleanly
- [x] [CODEx] `/dir` shows absolute path
- [x] [CODEx] `/dir ./workspace` changes dir if allowed
- [x] [CODEx] `/dir /etc` (or outside allowed dir on Windows) fails

### Sandbox / Security / Read-Only
- [x] [CODEx] Outside allowed dirs: read blocked
- [x] [CODEx] Outside allowed dirs: write blocked
- [x] [CODEx] Outside allowed dirs: delete blocked
- [x] [CODEx] Denied patterns block `.env` read
- [x] [CODEx] Path traversal blocked (`../../../...`)
- [x] [CODEx] `..hidden` legit name is not blocked
- [x] [CODEx] Read-only dir: read/list allowed
- [x] [CODEx] Read-only dir: write/delete/move-to/mkdir blocked
- [x] [CODEx] Read-only dir: copy-from allowed
- [x] [CODEx] Symlink escape blocked

### Approval / Errors / Fallback
- [x] [CODEx] `approval_mode: none` runs without asking
- [x] [CODEx] Unsupported action type -> clear error
- [x] [CODEx] Empty/invalid fallback chain -> clear user-facing error

### Edge Cases / Regression
- [x] [CODEx] Init wizard invalid choice handling
- [x] [CODEx] Corrupted `config.yaml` reload safety
- [x] [CODEx] Windows path variants behave consistently
- [!] [CODEx] Special/edge filename errors are user-friendly
- [x] [CODEx] Conversation history cap (20+ turns trim)

### Advanced & Stress (Automatable subset)
- [x] [CODEx] Hidden file list/read behavior
- [x] [CODEx] Path depth / long filename handling

---

## B) Joint Items (Run Together)

### Setup / Provider / Workflow
- [x] [JOINT] REPL banner version/path/provider/approval correctness
- [!] [JOINT] Conversation memory + `/clear` behavior with realistic prompts
- [x] [JOINT] Basic file operations via natural-language commands (read/list/write)
- [!] [JOINT] New action types via natural-language commands (mkdir/copy/move/delete)

### Approval / Revision / Fallback
- [x] [JOINT] `approval_mode: full` flow (PLAN/EXECUTE/RESULT)
- [!] [JOINT] `approval_mode: dangerous-only` flow (`[auto]` for safe ops)
- [x] [JOINT] `approval_mode: plan-only` flow
- [x] [JOINT] Plan revision flow (`v`, max 3 revisions)
- [!] [JOINT] Provider fallback with real keys (primary invalid, secondary valid)

### Reliability / Interruptions
- [!] [JOINT] Ctrl+C during in-flight provider call cancels cleanly
- [!] [JOINT] Ctrl+C during write/move does not leave bad state (best-effort)
- [!] [JOINT] Invalid API key + network timeout user-facing errors

### LLM Robustness
- [!] [JOINT] Ambiguous command handling
- [!] [JOINT] Contradictory instruction handling
- [x] [JOINT] Prompt injection attempt remains sandboxed

---

## C) Manual-Only Items (You Run)

### REPL UX / Terminal Ergonomics
- [x] [MANUAL] Ctrl+C once warns, twice exits (true interactive feel)
- [x] [MANUAL] Tab completion for slash commands
- [x] [MANUAL] Tab completion for `/model` subcommands
- [x] [MANUAL] Up/down history navigation quality
- [!] [MANUAL] History persistence across restart in your terminal

### Windows Specific
- [x] [MANUAL] Home/End/backspace/arrow editing feel
- [!] [MANUAL] No mojibake in terminal output
- [x] [MANUAL] Terminal theme readability (light/dark)

---

## D) Progress Summary
- Total CODEx items: 35
- Total JOINT items: 16
- Total MANUAL items: 8

Update these totals if we add/remove scope.
