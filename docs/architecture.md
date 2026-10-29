# Architecture

## Overview

```
                ┌──────────────────────────────────────────────┐
                │                     CLI                      │
                │  cmd/shipcheck → internal/cli                │
                └──────┬───────────────────────────────┬───────┘
                       │                               │
             ┌─────────▼─────────┐          ┌──────────▼──────────┐
             │    discovery      │          │      validate       │
             │ (walk + classify) │          │ (--docker-validate) │
             └─────────┬─────────┘          └──────────┬──────────┘
                       │ Result                        │
             ┌─────────▼─────────┐          ┌──────────▼──────────┐
             │      rules        │──────────►   model.Finding    │
             │  (pure analysis)  │          └──────────┬──────────┘
             └──┬──────┬──────┬──┘                     │
                │      │      │               ┌────────▼────────┐
         ┌──────▼─┐ ┌──▼───┐ ┌▼─────────┐     │     output      │
         │ envvar │ │parse │ │ secrets  │     │ text/json/sarif │
         └────────┘ └──────┘ └──────────┘     └─────────────────┘
                       ▲
              ┌────────┴────────┐
              │ fsutil (walk,   │   Every package reads the analyzed
              │ limits, ignore) │   repository ONLY through fsutil.
              └─────────────────┘
```

## Package responsibilities

| Package | Responsibility | May read disk? |
| --- | --- | --- |
| `cmd/shipcheck` | process entrypoint | no |
| `internal/cli` | flag parsing, exit-code contract, orchestration | no |
| `internal/discovery` | one safe walk; classifies manifests, projects, ambiguities | via fsutil |
| `internal/fsutil` | confined walking, limits, symlink policy, .gitignore subset, safe reads | yes (only here) |
| `internal/parse` | format decoding (JSON/YAML/TOML/Dockerfile/compose/env) | no |
| `internal/envvar` | referenced-vs-declared env-var names | no |
| `internal/secrets` | conservative credential patterns, masking | no |
| `internal/rules` | registry + deterministic rule engine | no |
| `internal/model` | finding/report types, severity/confidence vocabularies | no |
| `internal/config` | strict config loading | root config only |
| `internal/validate` | explicit docker validation mode | no (shells out) |
| `internal/output` | text/JSON/SARIF rendering | no |

## Key invariants

1. **One walk.** `discovery.Discover` performs the only filesystem walk.
   Every other package consumes its result. Rules never touch the disk
   outside the walked files (content is cached by `engine.contentOf`).
2. **Rules are pure functions.** Given the same `rules.Input`, the findings
   are identical. No clocks, no randomness, no I/O inside rule bodies.
3. **Registry is the contract.** `rules.Registry` defines every rule's ID,
   defaults and documentation; `shipcheck rules` renders from it, and the
   SARIF exporter derives rule definitions from it. A finding may never
   reference an ID outside the registry (enforced by `emit` and asserted in
   tests).
4. **Findings are self-contained.** Each finding carries ID, severity,
   confidence, check nature, location, evidence and remediation — machine
   consumers need no second lookup.
5. **Static/validated separation.** `internal/validate` is only reachable via
   the explicit `--docker-validate` flag, shells out to the Docker CLI with a
   bounded timeout, and marks its findings `check: validated` (or
   `inconclusive` when Docker is unavailable). Static findings are always
   `check: static`.

## Adding a rule

See [CONTRIBUTING.md](../CONTRIBUTING.md#adding-a-rule). Summary: register a
`Spec` in `rules.Registry` (ID, category, defaults, description, remediation,
**limitation**), implement the deterministic check in the rules package,
add positive and negative fixtures plus a test asserting both, regenerate
`docs/rules.md` with `make docs`. The failing-test-first workflow keeps the
catalogue honest: a rule without both fixtures does not pass CI.

## Determinism guarantees

- Walk order is sorted; rule evaluation order is fixed; findings are
  re-sorted by (file, line, rule ID) before rendering.
- JSON reports are byte-identical across runs except `meta.duration_seconds`.
- No map-iteration order leaks into output (all iterations over maps sort
  keys first).
