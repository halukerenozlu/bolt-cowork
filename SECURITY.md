# Security Policy

## Supported Versions

bolt-cowork is currently in pre-1.0 development. Only the latest tagged release receives security updates.

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅        |
| < 0.1   | ❌        |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, pull requests, or discussions.**

Instead, please use GitHub's private vulnerability reporting feature:

1. Go to the [Security tab](https://github.com/halukerenozlu/bolt-cowork/security) of this repository.
2. Click **"Report a vulnerability"**.
3. Fill out the form with as much detail as possible.

### What to Include

Please include the following in your report:

- A description of the vulnerability and its potential impact
- Steps to reproduce the issue
- Affected version(s) or commit hash
- Any proof-of-concept code or screenshots (if applicable)
- Your suggested fix, if you have one

### Response Timeline

- **Acknowledgment:** Within 7 days of your report.
- **Initial assessment:** Within 14 days.
- **Fix and disclosure:** Timeline depends on severity and complexity. You will be kept informed throughout.

## Scope

Security issues we care about include but are not limited to:

- **Sandbox escapes** — any way for the agent to read or write files outside the configured sandbox root
- **Command injection** — through crafted prompts, config files, or skill files
- **Path traversal** — bypasses of the `resolvePath` protections
- **Credential leakage** — exposure of API keys through logs, error messages, or crash dumps
- **Approval gate bypasses** — execution of write or execute actions without required user approval

## Out of Scope

- Vulnerabilities in third-party dependencies (report those upstream)
- Issues requiring physical access to the user's machine
- Social engineering of users into running malicious configs
