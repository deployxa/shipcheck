# Security Policy

Shipcheck is a security tool; its own security matters doubly. This policy
covers vulnerabilities **in Shipcheck itself** (the binary, its parsers, its
subprocess handling) — not findings it reports about your project.

## Supported versions

| Version | Supported |
| --- | --- |
| 0.1.0-rc1 | yes |
| < 0.1.0 | n/a (pre-release) |

## Reporting a vulnerability

**Do not open a public issue.** Use GitHub's *Report a vulnerability*
(private vulnerability reporting) or contact `security@deployxa.dev`.

Include: the Shipcheck version (`shipcheck version`), the command line, a
minimal fixture that triggers the problem, and the observed vs expected
behaviour. We aim to acknowledge reports within 72 hours.

## Scope — what we treat as a vulnerability

- Code execution through analyzed project content: any path by which a
  crafted repository causes Shipcheck to run commands, load code, or spawn
  interpreters.
- Path escape: any path by which Shipcheck reads or writes outside the
  project root (or the explicit output path) during static analysis.
- Secret leakage: any path by which an unmasked credential value appears in
  output, logs, or reports.
- Resource exhaustion: crafted inputs (depth, size, count, subprocess
  runaway) that make Shipcheck hang or crash ungracefully.
- Docker-validation boundary breaches: building/running project containers
  without the explicit flag, mounting the Docker socket, unbounded
  subprocesses.

## What is explicitly *not* a vulnerability

- Findings about *your* project (that is the product working).
- The documented limitations in `docs/threat-model.md` (e.g. `.gitignore`
  subset support, archives not being unpacked).
- A scan that exits `4` (incomplete) on hostile input — that is the
  designed fail-visible behaviour, as long as it does not crash.

## Design invariants you can hold us to

Static mode never executes project code, never imports project modules,
never runs package managers, never reads outside the project root, never
touches the network, and never emits an unmasked credential. The canary test
in `internal/cli` (`TestStaticModeNeverExecutesProjectCode`) enforces the
first of these in CI on every commit.
