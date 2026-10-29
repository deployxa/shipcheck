## Description

<!-- Describe the changes introduced by this pull request. If this adds or modifies a rule, reference the rule ID. -->

## Changes Checklist

- [ ] `make check` passes cleanly locally (`gofmt`, `go vet`, and `go test`).
- [ ] If detection logic was added or modified:
  - [ ] Both positive test cases (fires when condition is present) and negative test cases (stays silent on clean projects) are included.
  - [ ] Rule ID conforms to `SC-XXX-999` and is registered in `internal/rules/registry.go`.
  - [ ] Severity and confidence comply with definitions in `docs/false-positives.md`.
  - [ ] `make docs` was run to regenerate `docs/rules.md`.
- [ ] Static safety invariants are preserved:
  - [ ] Zero execution of project code or lifecycle scripts.
  - [ ] All filesystem access is bounded within the project root via `fsutil.Resolve`.
  - [ ] Credential values are masked (`internal/secrets`) before being included in findings.
- [ ] Added an entry to `CHANGELOG.md` under `[Unreleased]`.
