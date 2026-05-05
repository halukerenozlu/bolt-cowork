# Contributing to bolt-cowork

Thanks for your interest in contributing. This project is in early development (pre-1.0), so the contribution process is intentionally minimal.

## Quick Start

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<halukerenozlu>/bolt-cowork.git
   cd bolt-cowork
   ```
3. **Create a branch** for your change:
   ```bash
   git checkout -b fix/short-description
   ```
4. **Make your changes.**
5. **Run the tests** — everything must pass:
   ```bash
   go test ./...
   ```
6. **Build** to confirm the project still compiles:
   ```bash
   make build
   ```
7. **Commit** your changes with a clear message.
8. **Push** to your fork and **open a Pull Request** against `main`.

## Requirements

- Go 1.25 or newer
- All tests must pass (`go test ./...`)
- New behavior should come with tests
- Keep changes focused — one concern per PR

## Reporting Bugs

Open a GitHub issue with:

- What you expected to happen
- What actually happened
- Steps to reproduce
- Your OS and Go version

## Security

Do not report security issues through public issues or pull requests. See [SECURITY.md](SECURITY.md) for the private disclosure process.
