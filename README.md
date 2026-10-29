# Shipcheck

**Open-source pre-deployment readiness and topology verification for containerized applications.**  
Built and maintained by [Deployxa](https://deployxa.com). Distributed under the MIT License.

[![CI Status](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/deployxa/shipcheck/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.22+-blue.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/deployxa/shipcheck?color=blue)](https://github.com/deployxa/shipcheck/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Shipcheck is a local-first static analysis tool that detects deployment-halting defects before code leaves your workstation or enters a CI runner: broken Dockerfile build contexts, compose files referencing non-existent services or undeclared volumes, development servers used as production entrypoints, undeclared environment variables, committed credentials, and missing lifecycle signal handlers.

It operates strictly on static files, requires no account, executes no untrusted code, and makes zero network calls.

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

---

## Why

Compilers verify types. Linters enforce code style. Unit test suites test business logic. **None of them test whether an application repository is structurally deployable.**

In practice, production rollouts fail for mundane topological reasons:
1. **Cross-manifest drift:** The Dockerfile expects an environment variable that was renamed in `.env.example`, or `.dockerignore` excludes a folder that `npm run build` needs.
2. **Compose topology errors:** Services reference `depends_on` targets, `env_file` paths, or named volumes that do not exist in the repository.
3. **Development leakages:** Container entrypoints execute `vite`, `nodemon`, `flask run`, or `manage.py runserver`, exposing verbose error pages and exhausting memory.
4. **Lifecycle blindspots:** HTTP servers do not register `SIGTERM` listeners, resulting in hard kernel termination (`SIGKILL`) and dropped client connections during rolling updates.
5. **Security regressions:** Sensitive keys (AWS access keys, Stripe live keys, Slack tokens) are committed into source code or unignored local configurations.

Linters inspect individual files in isolation. Shipcheck walks the repository as an integrated deployment graph—correlating source code against manifests, Dockerfiles, compose stacks, and environment templates.

---

## The Finding Contract

Every finding emitted by Shipcheck adheres to a documented, deterministic contract across three distinct axes:

```
[SEVERITY / CONFIDENCE] RULE-ID file:line — Title
    evidence:    <sanitized extract; secrets always masked>
    explanation: <what this condition implies in production>
    fix:         <concrete remediation steps>
    limitation:  <documented boundaries of the static check>
```

### 1. Severity (Worst-case operational impact)
- **`CRITICAL`**: Guaranteed deployment failure, root container compromise, or confirmed credential leak (e.g., Docker socket mounts, committed live keys).
- **`HIGH`**: Broken deployment configuration that will fail orchestrator startup (e.g., unresolvable `depends_on`, missing named volume declarations, missing compose files).
- **`MEDIUM`**: Production anti-patterns and probable drift (e.g., running as `root`, undeclared environment variables, floating dependencies).
- **`LOW`**: Best-practice advisories with low blast radius (e.g., missing graceful shutdown handlers, unpinned tags).
- **`INFO`**: Informational observations (e.g., optional healthchecks).

### 2. Confidence (Degree of evidential certainty)
- **`confirmed`**: Direct, unambiguous factual match (e.g., invalid YAML syntax, missing file reference, regex match against known token structure).
- **`probable`**: High evidential correlation where external context might exist (e.g., variable read in code not found in any template, but possibly injected by an uncommitted cloud runner).
- **`possible`**: Heuristic observation requiring human review (e.g., generic password assignment pattern).

### 3. Check Nature
- **`static`**: Pure static inspection of file content and directory trees.
- **`validated`**: Verified via opt-in local subsystem validation (`--docker-validate`).
- **`inconclusive`**: Check could not definitively prove condition due to safe bounds (e.g., oversized file, skipped depth).

---

## Core Principles & Safety Model

Shipcheck treats the analyzed repository as **untrusted input**. It enforces strict defensive limits designed for safe local usage and sandboxed CI execution:

| Risk Vector | Built-in Mitigation |
| :--- | :--- |
| **Code Execution** | **Zero execution.** Never executes `npm`, `pip`, `make`, build scripts, or project entrypoints. |
| **Path Traversal** | All paths derived from manifests (`COPY`, `volumes`, `env_file`) are checked with `fsutil.Resolve` and confined to the repository root. Paths attempting escape (`../../etc/passwd`) are rejected. |
| **Symlink Attacks** | Walking identifies symlinks leaving the root or forming cycles; targets outside the project boundary are safely skipped and recorded as errors. |
| **Resource Exhaustion** | Strict internal caps: files > 5 MiB are skipped; directory recursion is capped at 24 levels; total files scanned capped at 20,000. |
| **Secret Redaction** | Credential values matched by `SC-SEC-*` patterns are sanitized immediately at extraction (`AKIA********`, `ghp_********`). Full secrets never appear in terminal output, logs, or JSON/SARIF reports. |
| **Network Isolation** | 100% offline. Zero telemetry, zero analytics, zero external API dependencies. |

---

## Supported Stacks & Discovery Topology

Shipcheck automatically detects project structures without requiring manual stack flags. In monorepos, every directory containing an application manifest is analyzed as an independent component.

| Ecosystem | Manifests Recognized | Lockfiles Verified |
| :--- | :--- | :--- |
| **Node.js / JS / TS** | `package.json` | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lockb` |
| **Python** | `pyproject.toml`, `requirements*.txt` | `poetry.lock`, `uv.lock`, `Pipfile.lock` |
| **Docker** | `Dockerfile`, `.dockerignore` | Multi-stage targets, `FROM`, `USER`, `EXPOSE`, `HEALTHCHECK`, `CMD` |
| **Compose** | `docker-compose.yml`, `compose.yaml` | Services, networks, volumes, `env_file`, ports, capabilities |
| **Environment** | `.env.*` templates, `.env` | Referenced vs. declared variables across code and manifests |
| **Process Managers** | `Procfile` | Web / worker process entrypoints |

---

## Installation

### From Source (Go 1.22+)

```console
git clone https://github.com/deployxa/shipcheck.git
cd shipcheck
make build
./bin/shipcheck --help
```

### Prebuilt Release Binaries

Static binaries for Linux, macOS (Darwin), and Windows (`amd64` and `arm64`) are published with SHA256 checksums on the [Releases](https://github.com/deployxa/shipcheck/releases) page.

```console
# Download for macOS Apple Silicon
curl -sSL https://github.com/deployxa/shipcheck/releases/download/v0.1.0-rc1/shipcheck-0.1.0-rc1-darwin-arm64.tar.gz | tar -xz
sudo mv shipcheck /usr/local/bin/

# Verify installation
shipcheck version
```

---

## CLI Usage

```console
# Standard static scan (human-readable output to stdout)
shipcheck ./my-project

# Control severity threshold for CI exit code gating
shipcheck ./my-project --severity-threshold high    # Exit 1 only on HIGH and CRITICAL

# Output pure, schema-validated JSON
shipcheck ./my-project --format json --output report.json

# Output SARIF 2.1.0 for GitHub Code Scanning integration
shipcheck ./my-project --format sarif --output shipcheck.sarif

# Opt-in Docker Compose engine validation (runs docker compose config cleanly)
shipcheck ./my-project --docker-validate

# Dump the full active rule catalogue
shipcheck rules --format markdown
```

### Exit Codes Contract

Shipcheck conforms to a deterministic exit contract:

| Exit Code | Classification | Description |
| :---: | :--- | :--- |
| **`0`** | **Clean** | Scan completed; no findings meet or exceed the active severity threshold. |
| **`1`** | **Findings** | Scan completed; one or more findings meet or exceed the threshold (pipeline gate failure). |
| **`2`** | **Usage Error** | Invalid flags, non-existent project path, malformed configuration file. |
| **`3`** | **Internal Error** | Unexpected runtime crash. |
| **`4`** | **Incomplete** | Scan completed, but input constraints forced items to be skipped (e.g., oversized files, symlink escapes). Findings are still output. |

---

## Configuration (`.shipcheck.yml`)

Shipcheck requires zero configuration by default. To persist exclusions or threshold rules across a team, place a `.shipcheck.yml` in your project root:

```yaml
# Severity threshold that triggers non-zero exit (critical | high | medium | low | info)
severity_threshold: high

# Additional directory names to exclude from directory walk
exclude:
  - legacy-fixtures
  - vendor
  - temp-assets

# Disable individual rules by stable contract ID
disable_rules:
  - SC-DCK-002    # Explicitly permit running as root in this repository
  - SC-PORT-001   # Permit binding specific host port

# Skip reading real .env files (default: true)
# When set to false, variable NAMES are collected for cross-referencing, but values are never stored or logged.
skip_real_dotenv: true
```

*Note: Shipcheck enforces strict configuration parsing. Unrecognized keys fail with exit code `2` to prevent silent misconfigurations from typos.*

---

## CI/CD Integration

### GitHub Actions (with Code Scanning SARIF Upload)

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
      security-events: write

    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install Shipcheck
        run: go install github.com/deployxa/shipcheck/cmd/shipcheck@latest

      - name: Run Scan
        run: |
          shipcheck . \
            --severity-threshold high \
            --format sarif \
            --output shipcheck.sarif

      - name: Upload SARIF to GitHub Security Tab
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: shipcheck.sarif
```

Findings are surfaced directly inside Pull Request code review annotations and under your repository's **Security → Code scanning** dashboard.

---

## Documentation Index

| Document | Purpose |
| :--- | :--- |
| [CLI Reference](docs/cli.md) | Comprehensive specification of all CLI arguments, flags, and diagnostic streams. |
| [Rule Catalogue](docs/rules.md) | Exhaustive list of rules, rationale, severity defaults, and exact remediations. |
| [Configuration Reference](docs/configuration.md) | Configuration keys, inheritance rules, and validation behavior. |
| [JSON Schema](docs/schema/shipcheck-report.v1.schema.json) | Formal JSON Schema definition for machine integration. |
| [Architecture](docs/architecture.md) | Internal pipeline: fsutil → discovery → parse → envvar → rules → output. |
| [Threat Model](docs/threat-model.md) | Security boundaries, symlink policies, and handling untrusted repositories. |
| [False-Positive Methodology](docs/false-positives.md) | Calibration benchmarks against the synthetic test corpus. |
| [Reproducible Fixtures](docs/fixtures.md) | Specification of the fixture corpus used for deterministic testing. |

---

## Contributing

Please review [CONTRIBUTING.md](CONTRIBUTING.md) for instructions on local development, running the test gate (`make check`), and authoring new detection rules.

For security reports, refer to [SECURITY.md](SECURITY.md).

---

## License

[MIT](LICENSE) — Copyright © 2026 Deployxa.
