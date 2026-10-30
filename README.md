<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/branding/shipcheck-header.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/branding/shipcheck-header.svg">
  <img alt="Shipcheck Banner" src="assets/branding/shipcheck-header.svg" width="100%">
</picture>

<br/>

**Open-source pre-deployment readiness and topology verification for containerized applications.**

[![CI Status](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/deployxa/shipcheck?color=00F59B)](https://github.com/deployxa/shipcheck/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

[Problem](#the-problem) • [How It Works](#how-it-works) • [Comparison](#candid-ecosystem-comparison) • [Features](#features) • [Installation](#installation) • [Quick Start](#quick-start) • [Suppressions & Baselines](#suppressions--baseline-mode) • [Configuration](#configuration) • [Architecture](#architecture)

</div>

---

## What is Shipcheck?

**Shipcheck** is an offline, local-first static analysis tool that verifies an application repository's readiness for production deployment before code leaves your workstation or passes through a CI runner.

It identifies common causes of deployment failures—broken Dockerfile build contexts, compose files referencing missing services or undeclared volumes, development servers configured as production entrypoints, undeclared environment variables, committed credentials, and missing lifecycle signal handlers.

Shipcheck inspects static files, requires no cloud accounts or API keys, executes no untrusted code, and makes zero network calls.

---

## The Problem

Compilers check type signatures. Syntax linters enforce style guidelines. Unit test suites verify internal business logic. **None of these tools check whether an application repository is topologically and operationally deployable.**

In practice, production rollouts fail for preventable configuration and structural reasons:

- **Cross-Manifest Inconsistencies:** Application source reads `process.env.API_KEY`, but the variable is missing from `.env.example`, compose environments, and Dockerfile `ENV` declarations.
- **Compose Topology Failures:** A `docker-compose.yml` service references a `depends_on` service or named volume that was never declared at the root level, causing orchestrators to fail during rollout.
- **Development Leakage:** A Dockerfile `CMD` runs `npm run dev`, `vite`, `flask run`, or `nodemon`, resulting in missing optimizations, unhandled memory growth, and exposed internal stack traces.
- **Uncontrolled Lifecycle Termination:** HTTP servers fail to register `SIGTERM` listeners, causing orchestrators to fall back to `SIGKILL` timeouts and drop active in-flight user requests during zero-downtime rolling deploys.
- **Credential Spillage:** Live keys, tokens, or private certificates are inadvertently committed to the repository.

Traditional linters inspect individual files in isolation. Shipcheck walks the repository as an **integrated deployment graph**, validating relationships across source code, manifests, Dockerfiles, Compose files, and environment definitions.

---

## Candid Ecosystem Comparison

Static analysis tools serve different layers of the delivery pipeline. Shipcheck focuses specifically on **pre-deployment application topology**:

| Dimension | Shipcheck | Hadolint | Checkov | Trivy | Kube-Linter |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Primary Scope** | Application Topology & Readiness | Dockerfile Syntax & Linting | Infrastructure as Code (IaC) | Vulnerability & CVE Scanning | Kubernetes YAML Policy |
| **Cross-Manifest Analysis** | **Yes** (Dockerfile ↔ Compose ↔ Code ↔ Env) | No (Dockerfile only) | Limited (within IaC resources) | No (Image/FS scan) | No (K8s manifests only) |
| **Env-Var Drift Detection** | **Yes** (Code usage vs templates) | No | No | No | No |
| **Compose Topology Integrity** | **Yes** (`depends_on`, volumes, env_file) | No | Yes (Security rules) | Misconfig check | No |
| **Zero Runtime Execution** | **Yes** (Pure static Go) | Yes | Yes (Python parser) | Yes | Yes |
| **Scan Latency (Medium Repo)** | **< 5 ms** (~300 scans/sec) | < 20 ms | 1.5 – 5 s | 2 – 8 s | < 50 ms |
| **Offline / Air-Gapped** | **100% Offline** | 100% Offline | Offline capable | Requires CVE DB sync | 100% Offline |
| **Inline Comment Suppressions**| **Yes** (`# shipcheck:ignore`) | Yes (`# hadolint ignore`) | Yes (`# checkov:skip`) | Via `.trivyignore` | Via annotations |
| **Baseline Mode** | **Yes** (`--baseline`) | No | Via CLI flags | No | No |

*See [docs/comparison.md](docs/comparison.md) for a detailed breakdown.*

---

## How It Works

Shipcheck analyzes repositories as untrusted input. It executes a single confined filesystem walk, extracts project topology, runs a pure deterministic rules engine, and formats findings:

```mermaid
flowchart TD
    subgraph Input["Untrusted Repository"]
        SRC["Application Source (.js, .py, .go, .rs)"]
        DCK["Dockerfiles & .dockerignore"]
        CMP["Docker Compose Specs"]
        ENV["Environment Templates (.env.*)"]
        PKG["Manifests (Node, Python, Go, Rust)"]
    end

    subgraph Core["Shipcheck Engine (Pure Go, Offline)"]
        FS["fsutil: Confined Walk<br/>(Bounds: 5MB size, 24 depth, symlink defense)"]
        DIS["discovery: Manifest Classifier & Monorepo Detector"]
        
        subgraph Parsers["Static Parsers (Zero Code Execution)"]
            P_DCK["Dockerfile AST"]
            P_CMP["Compose Decoder"]
            P_ENV["Env Var Extractor"]
            P_SEC["Secret Pattern Scanner"]
        end

        RULES["Deterministic Rule Registry (46 Rules + Custom Rules)"]
        SUP["Suppression Engine (Inline Comments & Baseline Diff)"]
    end

    subgraph Output["Diagnostic Reporting"]
        OUT_TXT["Terminal Output (ANSI, Stderr Diagnostics)"]
        OUT_JSON["Versioned JSON Report (v1 Schema)"]
        OUT_SARIF["GitHub SARIF 2.1.0 (Code Scanning)"]
    end

    SRC --> FS
    DCK --> FS
    CMP --> FS
    ENV --> FS
    PKG --> FS

    FS --> DIS
    DIS --> P_DCK
    DIS --> P_CMP
    DIS --> P_ENV
    DIS --> P_SEC

    P_DCK --> RULES
    P_CMP --> RULES
    P_ENV --> RULES
    P_SEC --> RULES

    RULES --> SUP
    SUP --> OUT_TXT
    SUP --> OUT_JSON
    SUP --> OUT_SARIF
```

---

## Output Contract

Every finding emitted by Shipcheck adheres to a structured, 3-axis contract:

```console
$ shipcheck ./my-app

Shipcheck 0.1.0-rc1 — open-source production-readiness checks
Project: /workspace/my-app
Detected projects: node:apps/api, node:apps/web
Scanned 64 files in 0.04s

[CRITICAL/confirmed] SC-BLD-001 apps/api/package.json:6 — Development server used as production entrypoint
    evidence: scripts["start"] = "nodemon server.js"
    explanation: Production entrypoint invokes nodemon; dev servers lack graceful drain and leak stack traces.
    fix: Change "start" to execute the production runtime: "node server.js".

[HIGH/confirmed] SC-CMP-001 docker-compose.yml:14 — depends_on references an undefined service
    evidence: service "web" depends on "cache", but "cache" is not declared in services
    explanation: Docker Compose will fail manifest validation during container deployment.
    fix: Define service "cache" in docker-compose.yml or remove the depends_on entry.

[HIGH/confirmed] SC-CMP-002 docker-compose.yml:22 — Named volume is not declared at top level
    evidence: service "api" mounts named volume "redis_data", but top-level volumes key is missing
    explanation: Modern Compose specifications reject undeclared top-level named volumes.
    fix: Add "volumes:\n  redis_data:" to top-level docker-compose.yml.

[MEDIUM/probable] SC-ENV-001 apps/api/src/db.js:12 — Referenced environment variable is not declared in any template
    evidence: DATABASE_URL is read in 2 location(s) but declared in no template (.env.example, compose, Dockerfile)
    explanation: Missing environment variables produce silent fallbacks or immediate runtime crashes upon deployment.
    fix: Add DATABASE_URL to .env.example or declare it in the compose environment block.

Summary: 1 critical, 2 high, 1 medium, 0 low, 0 info
Exit condition: 3 finding(s) at or above high threshold (exit 1)
```

### The 3 Evaluation Axes

1. **Severity (Operational Blast Radius):**
   - `CRITICAL`: Guaranteed deployment failure, root container compromise, or confirmed credential leak (e.g., Docker socket passthrough).
   - `HIGH`: Broken configuration that will prevent orchestrator startup (e.g., unresolvable `depends_on`, undeclared volume).
   - `MEDIUM`: Anti-pattern or configuration drift (e.g., container running as `root`, undeclared environment variables).
   - `LOW`: Best-practice advisory with low operational risk (e.g., missing graceful shutdown handler, unpinned tags).
   - `INFO`: Informational diagnostic observation (e.g., optional healthcheck omitted).

2. **Confidence (Evidential Certainty):**
   - `confirmed`: Unambiguous, structural fact (e.g., invalid YAML syntax, missing referenced file).
   - `probable`: Strong evidential correlation where external context might exist (e.g., variable read in source code but not declared in templates).
   - `possible`: Heuristic observation requiring human review (e.g., generic password assignment string).

3. **Check Nature:**
   - `static`: Pure static inspection of file content and directory trees.
   - `validated`: Verified via opt-in local subsystem validation (`--docker-validate`).
   - `inconclusive`: Check could not definitively prove condition due to safe bounds (e.g., oversized file, skipped depth).

---

## Features

### Implemented Capabilities

- **46 Static Readiness Rules across 14 Domains:** Covers startup scripts, port collisions, Dockerfile hygiene, Compose topologies, build contexts, graceful shutdown, environment variable drift, filesystem persistence, and secret leaks.
- **Polyglot Topology Discovery:** Automatically detects projects and monorepos across **Node.js** (`package.json`, lockfiles), **Python** (`pyproject.toml`, `requirements*.txt`, lockfiles), **Go** (`go.mod`, `go.sum`), and **Rust** (`Cargo.toml`, `Cargo.lock`).
- **Inline Suppressions:** Granular comment-based ignore annotations (`# shipcheck:ignore`, `# shipcheck:ignore-next-line`, `# shipcheck:ignore-file`).
- **Baseline Mode (`--baseline`):** Omit historical findings to gate only on newly introduced defects in legacy codebases.
- **Custom Declarative Rules:** Define organization-specific regex rules and file targets directly in `.shipcheck.yml`.
- **Environment Variable Analysis:** Distinguishes referenced, declared, required, optional, and unresolved variables across source code and deployment templates.
- **Secret Detection & Sanitization:** Detects AWS keys, GitHub tokens, Slack tokens, Stripe live keys, Google API keys, NPM tokens, and private keys. All matched values are permanently masked at extraction time.
- **Machine-Readable Outputs:** Versioned JSON report conforming to the published [`shipcheck-report.v1` schema](docs/schema/shipcheck-report.v1.schema.json) and SARIF 2.1.0 for GitHub Code Scanning.
- **Defensive Confinement:** Confines all filesystem walking within the scanned root, prevents symlink escapes, rejects path traversal (`../`), and caps memory usage (5 MiB file size limit, 24 directory depth limit, 20,000 file limit).
- **Extreme Performance:** Benchmarked at **~2 to 3 milliseconds per scan** (~300 scans per second).
- **Zero Runtime Execution:** Never runs build scripts, package managers (`npm`, `pip`), or container runtimes.

### Experimental Capabilities

- **Subprocess Compose Validation (`--docker-validate`):** Optionally invokes `docker compose config` in a sandboxed subprocess with a bounded 45-second timeout to validate compose syntax against the local Docker engine. Reports inconclusive if Docker is absent.

### Planned Capabilities

- **Auto-Remediation Mode (`--fix`):** Interactive generation of missing `.env.example` templates and Dockerfile `USER` directives.
- **Native WebAssembly Playground:** Interactive in-browser scanner running via WebAssembly.

---

## Installation

### Option 1: Quick Install Script (Linux & macOS)

```bash
curl -sSL https://raw.githubusercontent.com/deployxa/shipcheck/main/scripts/install.sh | sh
```

### Option 2: Homebrew (macOS & Linux)

```bash
brew install deployxa/tap/shipcheck
```

### Option 3: Official GitHub Action

Add to any GitHub Actions workflow (`.github/workflows/production-readiness.yml`):

```yaml
- name: Shipcheck Readiness Scan
  uses: deployxa/shipcheck@v0.1.0-rc1
```

### Option 4: Official Container Image

```bash
docker run --rm -v "$(pwd):/app" ghcr.io/deployxa/shipcheck:latest /app
```

### Option 5: Using `go install` (Go 1.22+)

```bash
go install github.com/deployxa/shipcheck/cmd/shipcheck@v0.1.0-rc1
```

### Option 6: Prebuilt Static Binaries

Download pre-compiled binaries from the [GitHub Releases](https://github.com/deployxa/shipcheck/releases) page:

```bash
# Example: Download for macOS (Apple Silicon / ARM64)
curl -sSL https://github.com/deployxa/shipcheck/releases/download/v0.1.0-rc1/shipcheck-0.1.0-rc1-darwin-arm64.tar.gz | tar -xz
sudo mv shipcheck /usr/local/bin/

# Verify installation
shipcheck version
```

---

## Quick Start

### 1. Scan a Local Repository

Run a static inspection against your application directory:

```bash
shipcheck ./my-app
```

### 2. Configure Severity Threshold for CI

Fail pipeline runs only when findings meet or exceed a specific severity level (default: `high`):

```bash
shipcheck ./my-app --severity-threshold critical
```

### 3. Generate Machine-Readable Reports

Generate pure JSON or SARIF reports:

```bash
# JSON output conforming to shipcheck-report.v1 schema
shipcheck ./my-app --format json --output report.json

# SARIF output for security dashboards
shipcheck ./my-app --format sarif --output shipcheck.sarif
```

### 4. Gate Only on New Findings with Baseline Mode

```bash
# 1. Freeze existing findings into a baseline report:
shipcheck ./my-app --format json --output .shipcheck-baseline.json

# 2. Gate CI against the baseline (only new findings fail the build):
shipcheck ./my-app --baseline .shipcheck-baseline.json --severity-threshold high
```

---

## Suppressions & Baseline Mode

### Inline Suppressions

Teams require escape hatches for intentional decisions without disabling rules project-wide. Shipcheck supports inline comment annotations:

```dockerfile
# Suppress a specific rule on the current line:
FROM alpine:3.19 # shipcheck:ignore SC-DCK-001

# Suppress on the next line:
# shipcheck:ignore-next-line SC-DCK-002
USER root
```

In Docker Compose:
```yaml
services:
  web:
    # Accept host networking for this local tool
    network_mode: host # shipcheck:ignore SC-CMP-006
```

In Application Code:
```javascript
// Variable injected by platform runner, undeclared in local template
const token = process.env.RUNNER_INTERNAL_TOKEN; // shipcheck:ignore SC-ENV-001
```

*See [docs/suppressions.md](docs/suppressions.md) for full suppression documentation.*

---

## Configuration (`.shipcheck.yml`)

Shipcheck operates with zero configuration by default. You can adjust behavior and define **custom declarative rules** by adding a `.shipcheck.yml` in your project root:

```yaml
# Severity threshold that causes non-zero exit: critical | high | medium | low | info (Default: high)
severity_threshold: high

# Additional directory names to skip during discovery
exclude:
  - legacy-fixtures
  - vendor
  - playground

# Disable specific rules by their stable contract ID
disable_rules:
  - SC-DCK-002    # Explicitly permit running as root in this repository
  - SC-PORT-001   # Explicitly permit hardcoded host port binding

# Skip real .env files when collecting variable names (Default: true)
skip_real_dotenv: true

# Custom declarative organization rules
custom_rules:
  - id: "CUSTOM-001"
    title: "No plaintext database passwords in compose"
    severity: "high"
    confidence: "confirmed"
    files: ["*compose*.yml", "*compose*.yaml"]
    pattern: "POSTGRES_PASSWORD:\\s*['\"]?[a-zA-Z0-9]+"
    remediation: "Use variable interpolation (${DB_PASSWORD}) or Docker secrets."
```

> **Strict Validation:** Unrecognized configuration keys are immediately rejected with exit code `2`. This ensures that configuration typos cannot silently disable checks.

---

## Exit Codes Contract

Shipcheck conforms to a deterministic Unix exit code specification suitable for automated pipelines:

| Code | Status | Meaning |
| :---: | :--- | :--- |
| **`0`** | **Clean** | Scan completed; no findings meet or exceed the active severity threshold. |
| **`1`** | **Findings** | Scan completed; findings at or above the severity threshold exist (CI gate fails). |
| **`2`** | **Usage Error** | Invalid invocation (unrecognized flag, invalid project path, malformed configuration). |
| **`3`** | **Internal Error** | Unexpected runtime crash or internal invariant violation. |
| **`4`** | **Incomplete** | Scan completed, but analysis was incomplete (e.g., oversized files or symlink escapes skipped). All collected findings are still output. |

---

## CI/CD Integration

### Turnkey GitHub Action (Zero Setup + Auto SARIF Annotations)

Add to `.github/workflows/production-readiness.yml`:

```yaml
name: Production Readiness Gate

on:
  push:
    branches: [main]
  pull_request:

jobs:
  shipcheck:
    name: Shipcheck Readiness Scan
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write # Required for automated SARIF PR annotations

    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Run Shipcheck
        uses: deployxa/shipcheck@v0.1.0-rc1
        with:
          severity-threshold: 'high'
```

*The action automatically installs the prebuilt binary in ~2 seconds, executes the scan, and uploads the SARIF report to GitHub Code Scanning so findings appear directly as inline Pull Request review annotations.*

### Pre-Commit Hook

Add Shipcheck to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/deployxa/shipcheck
    rev: v0.1.0-rc1
    hooks:
      - id: shipcheck
```

---

## Architecture

Shipcheck is designed around a single-pass, read-only pipeline with isolated package responsibilities:

| Package | Responsibility | May Access Disk? |
| :--- | :--- | :---: |
| `cmd/shipcheck` | CLI process entrypoint | No |
| `internal/cli` | Argument parsing, baseline diffing, exit code orchestration | No |
| `internal/fsutil` | Confined walking, defensive caps, symlink containment | **Yes (Only package)** |
| `internal/discovery` | Manifest recognition (Node, Python, Go, Rust), monorepo classification | Via `fsutil` |
| `internal/parse` | Pure decoders for Dockerfiles, Compose YAML, package manifests | No |
| `internal/envvar` | Cross-referencing variable usage against declarations | No |
| `internal/secrets` | Conservative token pattern matching and masking | No |
| `internal/rules` | Pure analysis engine (46 rules + custom rules + inline suppressions) | No |
| `internal/config` | Strict `.shipcheck.yml` loading and custom rule validation | Root config only |
| `internal/validate` | Opt-in Docker Compose CLI validation subprocess | No (Shells out) |
| `internal/output` | Formatted human terminal, JSON v1, and SARIF 2.1.0 renderers | No |

---

## Roadmap

### Completed (v0.1.0-rc1)
- [x] Recursive project discovery for Node.js, Python, Go (`go.mod`), Rust (`Cargo.toml`), Docker, Compose, and Procfiles.
- [x] Core rules engine with 46 rules across 14 operational categories.
- [x] Inline comment suppressions with expiration dates (`# shipcheck:ignore SC-DCK-002 until=2026-12-31`).
- [x] Baseline scanning mode (`--baseline`) with resilient semantic fingerprinting.
- [x] Declarative custom regex rules in `.shipcheck.yml`.
- [x] Machine-readable versioned JSON (`v1`) and SARIF 2.1.0 output.
- [x] Turnkey GitHub Action (`action.yml`) with automated SARIF upload and fast binary caching.
- [x] Pre-commit hook (`.pre-commit-hooks.yaml`), official container image, and one-liner installer.
- [x] Native parser fuzzing suite and sub-5ms scan benchmarks.

### In Progress
- [ ] Watch mode (`shipcheck --watch`) for real-time terminal feedback on file changes.
- [ ] Editor-level LSP extension.

### Proposed
- [ ] Native WebAssembly playground for live browser scanning.
- [ ] Auto-remediation flag (`--fix`) for standard template and Dockerfile defaults.

---

## Contributing

Contributions are welcome! Please review [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, development guidelines, and [CONTRIBUTING-rules.md](CONTRIBUTING-rules.md) for a 30-minute guide to authoring new rules.

### Local Development Gate

Before submitting a pull request, ensure all tests and formatters pass:

```bash
make check      # Runs gofmt, go vet, and all package tests
make docs       # Re-generates docs/rules.md from the rule registry
```

To report security vulnerabilities, please consult our [Security Policy](SECURITY.md).

---

## Documentation Index

| Resource | Contents |
| :--- | :--- |
| [CLI Reference](docs/cli.md) | Command syntax, flags, exit codes, and output streams |
| [Rule Catalogue](docs/rules.md) | Detailed documentation for all 46 rules and remediations |
| [Suppressions & Baselines](docs/suppressions.md) | Inline comment directives, baseline workflows, and escaping legacy debt |
| [Ecosystem Comparison](docs/comparison.md) | Candid technical comparison vs Hadolint, Checkov, Trivy, and Kube-Linter |
| [Rule Authoring Guide](CONTRIBUTING-rules.md) | 30-minute step-by-step tutorial on adding a new detection rule |
| [Configuration Reference](docs/configuration.md) | Complete `.shipcheck.yml` specification and custom rule schema |
| [JSON Schema Specification](docs/schema/shipcheck-report.v1.schema.json) | Formal schema for JSON report integration |
| [Architecture Specification](docs/architecture.md) | Internal subsystem designs and package invariants |
| [Threat Model & Security](docs/threat-model.md) | Defensive bounds, untrusted input policies, and sandboxing |
| [False-Positive Methodology](docs/false-positives.md) | Evidence grading, confidence tiers, and accuracy calibration |
| [Test Fixtures Specification](docs/fixtures.md) | Synthetic test fixtures used for deterministic regression testing |

---

## License

Shipcheck is licensed under the [MIT License](LICENSE).  
Copyright © 2026 [Deployxa](https://deployxa.com).
