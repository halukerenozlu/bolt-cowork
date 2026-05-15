# Contributing to bolt-cowork

Thanks for your interest in contributing. This project is in early development (pre-1.0), so the process is intentionally lightweight — but the quality bar is not.

## 1. Quick Start

```bash
git clone https://github.com/halukerenozlu/bolt-cowork.git
cd bolt-cowork
make build          # → dist/bolt-cowork[.exe]
make test           # all packages, race detector
```

Open a branch, make your change, run tests, open a PR against `master`.

## 2. Development Setup

| Tool | Version | Notes |
|---|---|---|
| Go | 1.26+ | `go version` to check |
| make | any | GnuWin32 on Windows, or Git Bash make |
| git | any | needed for version injection via `git describe` |
| golangci-lint | latest | `make lint`; install from golangci-lint.run |

**Windows:** `make build` and `make release` use `scripts/build.go` (pure Go, no bash required). Shell scripts under `scripts/*.sh` still need Git Bash if run directly.

## 3. Project Structure

```
cmd/bolt-cowork/     CLI entry point — main.go, REPL, init wizard
internal/
  agent/             Agent loop, plan/execute/approve pipeline
  config/            YAML config loader, trusted dirs, validation
  mcp/               MCP client and transport
  prompt/            Prompt templates and builders
  provider/          LLM providers (OpenAI, Anthropic, Gemini) + fallback chain
  sandbox/           File access boundary enforcement
  skill/             SKILL.md loader, matcher, injector
  tool/              Tool definitions and helpers
scripts/
  build.go           Cross-platform build helper (go run ./scripts/build.go)
  build.sh           Legacy bash build script
spec/                Detailed project specification (EN + TR)
testdata/            ALL test fixtures live here — never real user directories
.github/             CI workflow, release workflow, issue templates
```

## 4. Code Style

- Format with `gofmt` (`make lint` checks formatting)
- Wrap errors: `fmt.Errorf("context: %w", err)` — never swallow errors silently
- Write table-driven tests (`for _, tc := range tests { ... }`)
- Comments in English; package names short and lowercase
- No `init()` functions; avoid global mutable state

Run before pushing:

```bash
make test    # go test -v -race ./...
make lint    # gofmt check, go vet, golangci-lint
```

## 5. Commit Messages

[Conventional Commits](https://www.conventionalcommits.org/) with language/scope:

```
feat(go/agent): add plan revision step
fix(go/sandbox): correct path boundary detection on Windows
docs(spec): update v0.3.1 release notes
test(go/skill): add table-driven tests for frontmatter parser
chore(shell/build): remove legacy build.sh from Makefile
```

Scopes follow the pattern `language/package`:
- `go/agent`, `go/provider`, `go/skill`, `go/sandbox`, `go/config` …
- `shell/build`, `shell/test`
- `ts/components` (v0.6+)

## 6. Branch Naming

| Type | Pattern | Example |
|---|---|---|
| Feature | `feat/short-description` | `feat/mcp-skeleton` |
| Bug fix | `fix/short-description` | `fix/sandbox-windows-path` |
| Docs | `docs/short-description` | `docs/contributing-guide` |
| Chore | `chore/short-description` | `chore/update-dependabot` |

## 7. Pull Request Process

1. Fork the repo and create your branch from `master`.
2. Make your changes — keep PRs focused on one concern.
3. Run `make test` and `make lint` — both must pass.
4. Open a PR against `master` with a clear title and description.
5. A maintainer will review; address feedback in new commits (no force-pushes to open PRs).
6. PRs are merged by the maintainer after approval.

## 8. Testing

All tests must be isolated — **no access to real user directories**.

```bash
make test                # unit tests with race detector (required)
make test-integration    # integration tests (required for agent/sandbox changes)
go test ./... -count=1   # same as make test but without -v/-race, for quick checks
```

Rules enforced in this project:
- All file I/O tests run inside `testdata/` or `t.TempDir()`
- Never call `os.UserHomeDir()` or read `~/Documents`, `~/Desktop`, etc. in tests
- Each test creates and cleans up its own fixtures (setup/teardown)

New behaviour must come with tests. Bug fixes should include a regression test.

## 9. Release Process

Releases are fully automated via GitHub Actions:

1. A maintainer creates and pushes a version tag:
   ```bash
   git tag v0.3.1
   git push origin v0.3.1
   ```
2. `.github/workflows/release.yml` triggers automatically:
   - Runs `go test ./... -count=1`
   - Builds 5 platform binaries via `go run ./scripts/build.go release`
   - Creates a GitHub Release with auto-generated notes and all binaries attached

Contributors do not need to manage releases. If you believe a fix warrants a patch release, note it in your PR.

## Security

Do not report security issues through public issues or PRs. See [SECURITY.md](SECURITY.md) for the private disclosure process.
