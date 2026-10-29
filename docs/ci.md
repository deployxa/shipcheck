# CI integration guide

Shipcheck is designed for CI: deterministic, offline, no credentials needed.

## GitHub Actions

```yaml
name: production-readiness
on: [push, pull_request]

jobs:
  shipcheck:
    runs-on: ubuntu-latest
    permissions:
      security-events: write   # only needed for SARIF upload
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go install github.com/deployxa/shipcheck/cmd/shipcheck@v0.1.0-rc1
      - run: shipcheck . --severity-threshold high --format sarif --output shipcheck.sarif
      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: shipcheck.sarif
```

The run exits `1` when findings meet the threshold, failing the job. SARIF
results appear under *Security → Code scanning* with rule IDs, confidence and
remediation.

## GitLab CI

```yaml
shipcheck:
  stage: test
  image: golang:1.22
  script:
    - go install github.com/deployxa/shipcheck/cmd/shipcheck@v0.1.0-rc1
    - shipcheck . --format json --output shipcheck.json --severity-threshold high
  artifacts:
    when: always
    paths: [shipcheck.json]
```

## Pre-release gate (recommended)

```console
shipcheck . --severity-threshold high || exit 1
shipcheck . --docker-validate || true   # informational; requires Docker
```

Run `--docker-validate` as a separate, non-blocking step: it needs a Docker
daemon and reports `exit 4` (incomplete) where Docker is unavailable, which
you usually do not want to fail PR builds from runners without Docker.

## Interpreting results in CI

- Exit `1` — findings at/above threshold: gate failed.
- Exit `4` — analysis incomplete (skipped/unreadable files, Docker missing):
  findings still reported in the JSON under `meta.errors`. Decide per project
  whether that blocks.
- Rule IDs are stable; you can annotate or exempt known findings in review.
- Reports are deterministic — caching by content hash of the scanned tree is
  safe.
