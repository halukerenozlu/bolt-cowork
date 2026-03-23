# testdata/

This directory is for **testing purposes only**.

All file operation tests MUST run exclusively within this directory or inside `t.TempDir()`.

## Structure

- `sample-dir/` — Fake user directory for file operation tests
- `fixtures/` — Fixed test data (skill files, config samples, etc.)

## Rules

- NEVER use real user directories (`~/Documents`, `~/Desktop`, etc.) in tests
- NEVER access real paths via `os.UserHomeDir()` or `os.Getenv("HOME")`
- NEVER write outside the project directory (except `/tmp`)
