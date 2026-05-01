# bolt-cowork v0.1 Gemini-Assisted Test Checklist (Finalized)

This file tracks the tests performed autonomously by Gemini or in collaboration with the user.
Manual [M] tasks have been removed as they were handled separately.

İşaretleme Anahtarı:
- [A] - Autonomous (Gemini tarafından otomatik test edildi)
- [C] - Collaborative (Beraber test edildi)

## 1. Setup & Startup
- [A] `make install` succeeds
- [C] `bolt-cowork` starts REPL, banner shows correct version (v0.1.7)
- [A] Banner shows resolved absolute path for dir
- [A] Banner shows correct provider and approval mode
- [C] First run setup wizard

## 2. REPL Basics
- [C] `/help` shows all commands
- [A] Tab completion (simulated/inspected)
- [A] History persists after restart
- [C] Unknown command typo suggestion

## 3. Model & Key Management
- [A] `/model` shows current model
- [C] Switching models (sonnet, gpt-4o, gemini)
- [A] `/key` shows masked API key
- [C] `/key set` prompts (input masking check)

## 4. Config & Dir Commands
- [A] `/config` shows full config masked
- [A] `/config path`
- [A] `/config reload`
- [A] `/dir` shows CWD
- [A] `/dir ./workspace` change check
- [A] `/dir /etc` failure check (sandbox)

## 5. Conversation History
- [A] Context retention
- [A] `/clear` resets history

## 6. Basic File Operations
- [A] Read file contents
- [A] Read large file (truncation check)
- [A] List files in workspace
- [A] Write to file (content verification)
- [A] Reject empty write

## 7. New Action Types
- [A] Mkdir (idempotent check)
- [A] Copy (destination exists check)
- [A] Move (source deletion check)
- [A] Delete (recursive/non-empty check)

## 8. Sandbox Security
- [A] Read/Write/Delete outside allowed_dirs
- [A] Denied patterns (.env, .git)
- [A] Path traversal (../../../etc/passwd)
- [A] Legitimate names check (..hidden)

## 9. Read-Only Directories
- [A] Read/List allowed
- [A] Write/Delete/Mkdir blocked
- [A] Move FROM blocked, Copy FROM allowed

## 10. Approval Modes
- [C] Full mode behavior (Stage visibility)
- [C] Dangerous-only mode (Auto-approve check)
- [C] Plan-only mode
- [C] None mode

## 11. Plan Revision
- [C] Press `v` for revise and verify new plan
- [A] Max 3 revisions reached check

## 12. Error Handling
- [C] Ctrl+C during approval
- [A] Invalid API key error
- [A] Unsupported action error

## 13. Provider Fallback
- [A] Primary fail -> Secondary success flow
- [A] Fallback message visibility

## 16. Edge Cases & Regression
- [A] Corrupted config.yaml handling
- [A] Approval scope local to command
- [A] 20+ turns history trim

## 17. Advanced & Stress Tests
- [C] Ambiguous command (Clarification check)
- [A] Windows critical file access (C:\Windows\...)
- [A] 1000+ files performance (2s limit)
- [A] Permission errors (OS level access)
- [A] Malformed skill handling (no crash)
