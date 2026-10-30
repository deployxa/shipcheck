# Suppressions & Baselines

In real-world production engineering, legacy repositories contain known exceptions, and teams require escape hatches. Shipcheck provides three deterministic suppression mechanisms:

1. **Inline Suppressions:** Targeted comment annotations directly in source code, Dockerfiles, and Compose files.
2. **Baseline Mode (`--baseline`):** Omit historical findings to gate only on newly introduced defects in CI.
3. **Configuration Exclusions (`.shipcheck.yml`):** Project-wide rule disabling and directory exclusion.

---

## 1. Inline Suppressions

Shipcheck parses comment directives in any recognized file format (`#`, `//`, `/*`, `--`).

### Same-Line Suppression (`shipcheck:ignore`)

Place the comment on the exact line where the finding occurs:

```dockerfile
# Suppress a specific rule:
FROM alpine:3.19 # shipcheck:ignore SC-DCK-001

# Suppress multiple specific rules:
USER root # shipcheck:ignore SC-DCK-002, SC-SEC-009

# Blanket suppression for that line:
ENTRYPOINT ["nodemon", "server.js"] # shipcheck:ignore
```

In Docker Compose:
```yaml
services:
  web:
    # Accept host networking for this local utility service
    network_mode: host # shipcheck:ignore SC-CMP-006
```

In Application Code:
```javascript
// Variable injected by platform runner, undeclared in local template
const token = process.env.RUNNER_INTERNAL_TOKEN; // shipcheck:ignore SC-ENV-001
```

### Expirable Suppressions (`until=YYYY-MM-DD`)

To prevent ignore comments from rotting into permanent tech debt that reviewers stop trusting, Shipcheck supports optional expiration dates on all suppression directives:

```dockerfile
# Suppression is active until December 31, 2026; after that date it fails the build:
FROM alpine:3.19 # shipcheck:ignore SC-DCK-001 until=2026-12-31

# Expired suppression on a legacy service:
USER root # shipcheck:ignore SC-DCK-002 until=2026-06-30
```

When an `until` date has passed:
1. The suppression expires immediately.
2. Shipcheck reports the finding again, annotating the evidence: `(suppression expired on YYYY-MM-DD)`.
3. CI gates fail if the finding breaches the configured severity threshold.

### Next-Line Suppression (`shipcheck:ignore-next-line`)

Place the directive on the line immediately preceding the target line:

```dockerfile
# shipcheck:ignore-next-line SC-DCK-002
USER root
```

```yaml
# shipcheck:ignore-next-line SC-CMP-005
privileged: true
```

### File-Level Suppression (`shipcheck:ignore-file`)

Place anywhere in a file to ignore specific rules (or all rules) for that entire file:

```dockerfile
# shipcheck:ignore-file SC-DCK-001, SC-DCK-002
FROM node:latest
WORKDIR /app
COPY . .
CMD ["node", "app.js"]
```

Or blanket ignore for a third-party mock fixture:
```javascript
// shipcheck:ignore-file
const mockApiKey = "AKIAIOSFODNN7EXAMPLE";
```

---

## 2. Baseline Mode (`--baseline <report.json>`)

When adopting Shipcheck on a legacy codebase with dozens of existing issues, failing the build on historical debt blocks adoption. Baseline mode allows teams to freeze existing findings and fail CI only when **new** defects are introduced.

### Generating the Initial Baseline

Run a scan and output the report as JSON:

```bash
shipcheck ./my-app --format json --output .shipcheck-baseline.json
```

Commit `.shipcheck-baseline.json` to your repository.

### Gating CI with the Baseline

In subsequent CI runs or local scans, pass the `--baseline` flag:

```bash
shipcheck ./my-app --baseline .shipcheck-baseline.json --severity-threshold high
```

- **Resilient Fingerprinting:** Shipcheck matches baseline findings not only by exact line position, but by an ESLint-style semantic content fingerprint: `hash(RuleID, File, NormalizedEvidence)`.
- **Refactor Proof:** If code is refactored and line numbers shift, existing defects remain suppressed as long as the underlying finding evidence is unchanged.
- Only **newly introduced** findings will be reported and evaluated against `--severity-threshold`.
- If zero new findings exist, Shipcheck exits with code `0`.
- The number of filtered baseline findings is noted in `meta.notes`.

---

## 3. Project-Wide Rule Disabling (`.shipcheck.yml`)

To permanently disable a rule across the entire repository without cluttering source files with comments, declare it in `.shipcheck.yml`:

```yaml
# Disable specific rules by stable ID
disable_rules:
  - SC-DCK-002   # Intentionally running containers as root
  - SC-PORT-001  # Hardcoded host ports are permitted in this project
```
