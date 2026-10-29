<div align="center">

# 🚢 Shipcheck

**Open-source, local-first production-readiness checks for modern applications.**

[![CI Status](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/deployxa/shipcheck?color=blue)](https://github.com/deployxa/shipcheck/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

*Catch broken deployments at commit time, not in production at 2 AM.*

[Features](#-key-features) • [Installation](#-installation) • [Quick Start](#-quick-start) • [Rule Catalogue](#-rule-categories) • [CI/CD Integration](#-cicd-integration) • [Configuration](#-configuration)

</div>

---

## 💡 What is Shipcheck?

**Shipcheck** is a lightweight, blazing-fast CLI tool that audits your application repository for production-readiness defects *before* deployment.

While unit tests check logic and traditional linters check syntax, deployments often fail due to cross-cutting integration bugs:
- A `docker-compose.yml` mounts a named volume that was never declared at the top level.
- An environment variable is read in application code (`process.env.API_KEY`) but omitted from deployment templates and Dockerfile `ENV` specs.
- A production `Dockerfile` runs `npm run dev` or `vite` instead of a compiled production server.
- A container lacks a `USER` directive and runs as `root`, or lacks a `HEALTHCHECK` for orchestrators.
- A service ignores `SIGTERM`, causing forced `SIGKILL` timeouts and dropped in-flight requests during rolling deploys.
- Live secrets or credentials were inadvertently committed into the repository.

Shipcheck analyzes your Dockerfiles, Compose files, package manifests, environment templates, and source files **as a coherent topology** to surface these issues in milliseconds.

```console
$ shipcheck ./my-app

Shipcheck 0.1.0-rc1 — open-source production-readiness checks
Project: /workspace/my-app
Detected projects: node:.
Scanned 42 files in 0.02s

[CRITICAL/confirmed] SC-BLD-001 package.json:5 — Development server used as production entrypoint
    evidence: "start": "nodemon server.js"
    fix: Run the production build and serve it with the framework's production server.

[HIGH/confirmed] SC-CMP-001 docker-compose.yml:8 — depends_on references an undefined service
    evidence: service "web" depends on "cache", but "cache" is not defined
    fix: Define the missing service or remove the dependency entry.

[MEDIUM/probable] SC-ENV-001 src/server.js:14 — Referenced environment variable is not declared in any template
    evidence: DATABASE_URL is read in 2 source location(s) but declared in no template
    fix: Add the variable to deployment templates (.env.example, compose, Dockerfile ENV).

Summary: 1 critical, 1 high, 1 medium, 0 low, 0 info
Exit condition: 2 finding(s) at or above high threshold (exit 1)
```

---

## ✨ Key Features

- **⚡ Blazing Fast (<50ms):** Single static Go binary with zero external runtime dependencies. Runs instantly in pre-commit hooks and local terminals.
- **🔒 Safe by Construction:** Evaluates repositories as untrusted input. Path traversal, symlink escapes, oversized files, and binary blobs are safely bounded.
- **🚫 Zero Runtime Execution:** Never compiles or executes untrusted project code, package scripts, or containers.
- **🌐 100% Offline & Local-First:** No accounts, no API keys, and zero network calls. Air-gapped safe for enterprise and CI environments.
- **🎯 Evidence-Based & Deterministic:** Every finding includes stable rule IDs, exact source locations, severity levels, confidence ratings, and actionable remediation steps.
- **🏢 Monorepo Aware:** Automatically detects multi-service repositories, discovering multiple Node.js, Python, and Docker projects cleanly.
- **📊 Standard Output Formats:** Formatted human-readable CLI output, schema-validated JSON (`--format json`), and GitHub Code Scanning SARIF (`--format sarif`).

---

## 📦 Supported Stacks & Manifests

| Category | Supported Files & Manifests |
| :--- | :--- |
| **Node.js** | `package.json`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lockb` |
| **Python** | `pyproject.toml`, `requirements*.txt`, `poetry.lock`, `uv.lock`, `Pipfile.lock` |
| **Containers** | `Dockerfile`, `.dockerignore` |
| **Orchestration**| `docker-compose.yml`, `docker-compose.yaml`, `compose.yml`, `compose.yaml` |
| **Configuration**| `.env.*` templates, `Procfile`, GitHub Actions, GitLab CI, CircleCI configs |

---

## 🚀 Installation

### Using Go (Go 1.22+)

```bash
go install github.com/deployxa/shipcheck/cmd/shipcheck@latest
```

### From Prebuilt Release Binaries

Download pre-compiled binaries for Linux, macOS, and Windows from the [GitHub Releases](https://github.com/deployxa/shipcheck/releases) page.

```bash
# Example for macOS (Apple Silicon)
curl -sSL https://github.com/deployxa/shipcheck/releases/download/v0.1.0-rc1/shipcheck-0.1.0-rc1-darwin-arm64.tar.gz | tar -xz
sudo mv shipcheck /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/deployxa/shipcheck.git
cd shipcheck
make build
./bin/shipcheck --help
```

---

## 🛠️ Quick Start

```bash
# Basic scan with default human-friendly terminal report
shipcheck ./my-app

# Fail CI only on critical issues (default threshold: high)
shipcheck ./my-app --severity-threshold critical

# Generate machine-readable JSON report
shipcheck ./my-app --format json --output report.json

# Generate SARIF for GitHub Code Scanning
shipcheck ./my-app --format sarif --output shipcheck.sarif

# Validate compose files against local Docker engine (if available)
shipcheck ./my-app --docker-validate

# View the built-in rule catalogue
shipcheck rules
```

### Exit Codes Contract

Shipcheck conforms to a strict Unix exit code contract suitable for CI gates:

| Code | Status | Meaning |
| :---: | :--- | :--- |
| **`0`** | **Clean** | Scan completed successfully; no findings at or above the configured severity threshold. |
| **`1`** | **Findings** | Scan completed; at least one finding meets or exceeds the severity threshold (fails CI). |
| **`2`** | **Usage Error** | Invalid invocation (unrecognized flag, inaccessible path, malformed configuration). |
| **`3`** | **Internal Error**| Unexpected internal failure or crash. |
| **`4`** | **Incomplete** | Scan completed, but analysis was incomplete (e.g., binary files or deep nesting skipped). |

---

## 🔍 Rule Categories

Shipcheck ships with an extensible catalogue of battle-tested rules across 8 operational domains:

- **🐳 Docker & Container Hardening (`SC-DCK-*`)**: Detects root user entrypoints, unpinned `latest` base images, shell-form `CMD`/`ENTRYPOINT` preventing signal propagation, missing `HEALTHCHECK`, and insecure `curl \| sh` install scripts.
- **🐙 Docker Compose Integrity (`SC-CMP-*`)**: Detects missing `depends_on` targets, undeclared named volumes, missing `env_file` paths, dangerous `privileged: true`, host networking mode, and Docker socket passthrough mounts (`/var/run/docker.sock`).
- **⚙️ Environment Variable Drift (`SC-ENV-*`)**: Detects variables referenced in source code but undeclared in templates/Compose, variables declared in templates but never referenced, and missing production env files.
- **🔑 Secret & Token Leakage (`SC-SEC-*`)**: Detects committed AWS access keys, GitHub tokens, Slack tokens, Stripe live keys, Google API keys, private keys, and hardcoded credential assignments.
- **🛑 Process & Graceful Shutdown (`SC-GRD-*`)**: Detects web servers running without `SIGTERM`/`SIGINT` handlers, preventing graceful connection draining during deployments.
- **⚡ Startup & Build Verification (`SC-BLD-*`, `SC-SCR-*`)**: Catches development servers (`nodemon`, `vite`, `next dev`, `flask run`) used as production entrypoints, and missing npm lifecycle scripts.
- **🔌 Network & Port Mapping (`SC-PORT-*`)**: Detects dangerous hardcoded host port collisions and mismatches between Dockerfile `EXPOSE` and Compose published ports.
- **💾 Ephemeral Filesystem Persistence (`SC-FS-*`)**: Identifies stateful file writes occurring in ephemeral containers without attached volumes.

View all active rules and remediation steps with:
```bash
shipcheck rules --format markdown
```
Or explore the [Full Rule Catalogue](docs/rules.md).

---

## 🤖 CI/CD Integration

### GitHub Actions (with Code Scanning SARIF)

Add Shipcheck to `.github/workflows/production-readiness.yml`:

```yaml
name: Production Readiness Gate

on:
  push:
    branches: [main]
  pull_request:

jobs:
  shipcheck:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write # Required for GitHub Code Scanning upload

    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install Shipcheck
        run: go install github.com/deployxa/shipcheck/cmd/shipcheck@latest

      - name: Run Shipcheck Scan
        run: |
          shipcheck . \
            --severity-threshold high \
            --format sarif \
            --output shipcheck.sarif

      - name: Upload to GitHub Code Scanning
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: shipcheck.sarif
```

Findings are automatically presented directly in Pull Request diffs and under the repository's **Security → Code scanning** tab.

### GitLab CI

```yaml
stages:
  - test

shipcheck:
  stage: test
  image: golang:1.22
  script:
    - go install github.com/deployxa/shipcheck/cmd/shipcheck@latest
    - shipcheck . --severity-threshold high --format json --output shipcheck-report.json
  artifacts:
    when: always
    paths:
      - shipcheck-report.json
```

---

## ⚙️ Configuration

Shipcheck is zero-config by default. You can customize behavior by placing an optional `.shipcheck.yml` in your project root:

```yaml
# Set failure threshold: critical | high | medium | low | info (Default: high)
severity_threshold: high

# Additional directory names to exclude from analysis
exclude:
  - legacy
  - e2e-fixtures
  - vendor

# Disable specific rules by ID if intentionally accepted
disable_rules:
  - SC-DCK-002 # Accept running as root in this specific container
  - SC-PORT-001 # Accept specific host port binding

# Skip reading real .env files for variable name discovery (Default: true)
skip_real_dotenv: true
```

*Note: Typo protection is enabled—unrecognized configuration keys are rejected with an exit code of `2`.*

---

## 📖 Detailed Documentation

- [CLI Reference](docs/cli.md) — Complete flag reference and exit code semantics
- [Rule Catalogue](docs/rules.md) — Comprehensive guide to every check and its remediation
- [Configuration Reference](docs/configuration.md) — `.shipcheck.yml` options and schema
- [JSON Schema Specification](docs/schema/) — Machine-readable output contract
- [Threat Model & Security](docs/threat-model.md) — Untrusted code boundary guarantees
- [False-Positive Philosophy](docs/false-positives.md) — Evidence vs heuristic grading

---

## 🤝 Contributing

We welcome contributions of new rules, manifest parsers, and ecosystem integrations!
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for local development setup and how to author new rules.

---

## 🛡️ Security

For security vulnerability reports, please review our [Security Policy](SECURITY.md).

---

## 📄 License

Shipcheck is licensed under the [MIT License](LICENSE).  
Copyright © 2026 Deployxa.
