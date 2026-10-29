# CLI reference

## `shipcheck <project-root> [flags]`

Runs a production-readiness scan. `<project-root>` must be a readable
directory; it is the only required argument.

### Flags

| Flag | Values | Default | Meaning |
| --- | --- | --- | --- |
| `--format` | `text`, `json`, `sarif` | `text` | Report format. `text` is human output; `json` and `sarif` are machine-readable. |
| `--output` | path | *(stdout)* | Write the machine-readable report to a file instead of stdout. |
| `--severity-threshold` | `critical`, `high`, `medium`, `low`, `info` | `high` | Exit `1` when at least one finding is at or above this severity. |
| `--docker-validate` | boolean | off | Additionally run `docker compose config` on every discovered compose file. Requires the Docker CLI and daemon; never builds or runs your containers; reports honestly (inconclusive) when Docker is unavailable. |
| `--config` | path | `.shipcheck.yml` in the project root | Explicit config file path. Must stay inside the project root. |

Flags may appear before or after the project root.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Scan completed; no findings at/above threshold. |
| `1` | Scan completed; findings at/above threshold exist. |
| `2` | Invalid invocation: unknown flag, unreadable project root, malformed config, unknown format/threshold. |
| `3` | Unexpected internal error (scan crashed). |
| `4` | Scan completed but analysis is **incomplete**: some files were unreadable, skipped (binary/oversized/deep), or Docker validation could not run. All collected findings are still reported. |

Precedence rule: when findings breach the threshold **and** the analysis is
incomplete, Shipcheck exits `1` so CI gates fail closed; the incompleteness is
visible in `meta.incomplete`/`meta.errors` of the JSON report and in the text
summary. CI pipelines that must distinguish the two should check
`meta.incomplete` explicitly.

### Diagnostics

Human diagnostics (warnings, `report written to …`) go to **stderr**.
Machine-readable output goes to **stdout** (or `--output`), so
`shipcheck ./app --format json > report.json` is always pure JSON.

## `shipcheck rules [--format text|json|markdown]`

Prints the rule catalogue: ID, default severity, confidence, category, title,
description, remediation and documented limitation for every rule. Rule IDs
are stable contract identifiers — see [rules.md](rules.md).

## `shipcheck version`

Prints the version, commit and attribution line.

## `shipcheck help`

Prints usage.
