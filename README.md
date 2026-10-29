# Shipcheck

**Open-source production-readiness checks for your application.**

Built and maintained by Deployxa.

Shipcheck is a CLI tool that finds common production-readiness problems in an
application repository *before* you deploy: broken Dockerfiles, compose files
that reference missing files, development servers used as production
entrypoints, undeclared environment variables, committed credentials, missing
health checks and more. It runs entirely on your machine, needs no account,
no API key, no network access, and never executes your project's code.

```console
$ shipcheck ./my-app
[MEDIUM/probable] SC-ENV-001 server.js:2 — Referenced environment variable is not declared in any template
    evidence: API_KEY is read in 1 source location(s) but declared in no template
    ...
Exit condition: 2 finding(s) at or above high threshold (exit 1)
```

## What Shipcheck is not

- **Not a security audit.** Secret detection is conservative and pattern-based;
  a clean scan does not mean the repository is free of credentials.
- **Not a deploy guarantee.** Findings are evidence-based indicators, not
  proof that a deployment will succeed or fail.
- **Not a runtime checker.** Static mode never runs your application, its
  scripts, its package managers, or any container.

## Design principles

1. **Local-first and offline.** No accounts, no telemetry, no network calls in
   static mode. Suitable for CI and for analyzing untrusted repositories.
2. **Evidence over guessing.** Every finding carries rule ID, severity,
   confidence, the nature of the check (static/validated/inconclusive) and the
   evidence it is based on. Where evidence is insufficient, Shipcheck says so
   instead of failing the build.
3. **Deterministic.** Two scans of the same unchanged project produce
   byte-identical JSON (modulo the duration field).
4. **Safe by construction.** The analyzed repository is treated as untrusted
   input: path traversal, symlink escapes, oversized files, deep nesting,
   binary and unreadable files are all handled explicitly. See
   [docs/threat-model.md](docs/threat-model.md).

## Install

From source (Go 1.22+):

```console
git clone https://github.com/deployxa/shipcheck
cd shipcheck && make build
./bin/shipcheck --help
```

Prebuilt release archives for Linux, macOS and Windows (amd64/arm64) are
published with SHA256 checksums on the releases page.

## Quick start

```console
shipcheck ./my-app                          # static scan, human output
shipcheck ./my-app --format json            # machine-readable report
shipcheck ./my-app --format sarif           # for GitHub code scanning
shipcheck ./my-app --severity-threshold medium
shipcheck ./my-app --docker-validate        # also run `docker compose config`
shipcheck rules                             # print the rule catalogue
```

Exit codes: `0` clean · `1` findings at/above threshold · `2` invalid
invocation · `3` internal error · `4` analysis incomplete (findings still
reported). See [docs/cli.md](docs/cli.md).

## Supported stacks

| Stack   | Manifests recognized                                             |
| ------- | ---------------------------------------------------------------- |
| Node.js | package.json, package-lock.json, yarn.lock, pnpm-lock.yaml, bun.lockb |
| Python  | pyproject.toml, requirements*.txt, poetry.lock, uv.lock, Pipfile.lock |
| Docker  | Dockerfile, .dockerignore                                        |
| Compose | docker-compose.yml/.yaml, compose.yml/.yaml                      |
| Procfile, .env.example-style templates, GitHub Actions / GitLab CI / CircleCI configs |

Monorepos are supported: every directory containing an application manifest is
treated as a deployable unit, and ambiguity is reported rather than guessed.

## Documentation

- [CLI reference](docs/cli.md)
- [Rule catalogue](docs/rules.md) — also live via `shipcheck rules`
- [Configuration reference](docs/configuration.md)
- [JSON schema & example report](docs/schema/)
- [CI integration guide](docs/ci.md)
- [Architecture](docs/architecture.md)
- [Threat model & security boundaries](docs/threat-model.md)
- [False-positive methodology](docs/false-positives.md)
- [Reproducible fixtures](docs/fixtures.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and a
step-by-step guide for adding a rule. Security issues: see
[SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) — Copyright (c) 2026 Deployxa.
