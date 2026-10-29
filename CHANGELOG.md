# Changelog

All notable changes to Shipcheck are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0-rc1] — 2026-10-30

First release candidate. Milestones 1–3 of the initial roadmap are complete:
functional MVP, trust & security hardening, release candidate packaging.

### Added

- **Discovery** — recursive project discovery for Node.js (package.json +
  lockfiles), Python (pyproject.toml, requirements*.txt + lockfiles),
  Dockerfiles, compose files, Procfiles, env templates and CI configs;
  monorepo awareness; `.gitignore`-aware walking; ambiguity reporting.
- **Rules engine** with 46 registered rules across 14 categories, each with
  stable `SC-xxx-nnn` IDs, default severity/confidence, actionable
  description, remediation and a documented limitation. Categories: startup,
  ports, build-context, Dockerfile, package scripts, compose, environment
  variables, filesystem persistence, healthchecks, graceful shutdown,
  dependency pinning, secrets, discovery, docker validation.
- **Environment-variable analysis** — referenced vs declared vs required vs
  optional vs unresolved names. Values are never read from real `.env` files
  by default and never emitted anywhere.
- **Secret detection** — conservative known-token patterns plus generic
  credential assignments; confirmed/probable/needs-review tiers; every value
  masked in all output.
- **Machine-readable output** — versioned JSON report (`shipcheck-report.v1`
  schema, with a JSON Schema document and a generated example) and SARIF
  2.1.0; diagnostics separated from machine-readable stdout.
- **`--docker-validate`** — explicit opt-in compose validation via
  `docker compose config` with a bounded timeout; honest inconclusive
  reporting when Docker is unavailable; never builds or runs project
  containers; never mounts the Docker socket.
- **Configuration** — optional strict `.shipcheck.yml` (unknown keys
  rejected): severity threshold, excludes, rule disabling,
  `skip_real_dotenv` opt-in.
- **Security hardening** — path confinement for all content-derived paths,
  symlink escape detection, 5 MiB file cap, 24-level depth cap, 20k file
  cap, binary sniffing, irregular-file refusal, malformed-manifest
  handling, no shell interpolation anywhere, bounded subprocess only in
  validate mode.
- **Test suite** — synthetic fixtures for healthy/broken/Python/compose/
  monorepo/persistence/empty/adversarial projects; positive and negative
  fixtures per rule; determinism test; JSON schema-shaped assertions;
  SARIF validity assertions; exit-code contract tests; and the canary test
  proving static mode executes no project code.
- **Documentation** — CLI reference, rule catalogue (generated),
  configuration reference, CI integration guide, architecture, threat
  model, false-positive methodology, reproducible fixtures contract.
- **CI workflow** — lint/vet/test gate, cross-platform build matrix,
  checksum-producing release job.

### Deliberately not included

- No auto-fixing or file modification: first versions never mutate projects.
- No real-world accuracy claims: measured behavior is documented only
  against the fixture corpus (see docs/false-positives.md).
- No plugin framework: adding rules is a documented code change with
  mandatory fixtures, not a runtime API.

[0.1.0-rc1]: https://github.com/deployxa/shipcheck/releases/tag/v0.1.0-rc1
