# Contributing to Shipcheck

Thanks for helping! Shipcheck's value is *trustworthy* findings, so the bar
for changes is: deterministic, evidence-based, documented, and tested in
both directions (a rule that fires where it shouldn't is a bug, and so is a
rule that stays silent where its documentation says it must fire).

## Development workflow

Prerequisites: Go 1.22+.

```console
make build      # compile to bin/shipcheck
make test       # go test ./...
make check      # gofmt + go vet + tests (what CI runs)
make docs       # regenerate docs/rules.md from the registry
make fixtures   # recreate test fixtures (including symlinks)
```

Before opening a PR:

1. `make check` passes.
2. New behaviour has tests.
3. `docs/rules.md` is regenerated if the registry changed.
4. The CHANGELOG has an entry under *Unreleased*.

## Project rules

- **Static analysis is pure.** Rule bodies must not touch the clock, the
  network, or the filesystem outside `rules.Input`. Anything that must read
  files goes through `discovery`/`fsutil` results.
- **No execution of project code.** No exceptions, including "just to test
  if the config parses". Parsers are static.
- **No full credential values anywhere.** If you touch secret detection,
  the masking test is not optional.
- **Rule IDs are forever.** Never rename or repurpose an `SC-xxx-nnn` ID.
  Retired IDs stay in the registry marked as such.
- **Severity/confidence have definitions.** Changes to a rule's defaults
  need a sentence in the PR explaining which entry of
  `docs/false-positives.md` applies.

## Adding a rule

1. **Register it** in `internal/rules/registry.go`:

   ```go
   {
       ID: "SC-DCK-006", Title: "…", Category: CatDocker,
       Severity: model.SevMedium, Confidence: model.ConfProbable,
       Description: "what is wrong and why it matters",
       Remediation: "how to fix it",
       Limitation:  "what this rule cannot see",
   },
   ```

   Pick the category; never invent an ID shape outside `SC-XXX-999`.

2. **Implement the check** in the rules package (usually a small function
   called from the appropriate `run*Rules` stage). Emit via
   `e.emit("SC-DCK-006", func(f *model.Finding) { … })` so the finding
   inherits the registered defaults and can be disabled by config.

3. **Add fixtures.** A rule without a positive *and* a negative fixture does
   not pass review:
   - trigger the rule in an existing positive fixture or a new one,
   - prove absence in a negative fixture (e.g. `healthy-node` must stay at
     zero findings — if your rule fires there, the rule is wrong).

4. **Assert both directions** in `internal/rules/rules_test.go`:

   ```go
   assertHas(t, ids, "SC-DCK-006")
   // and in the negative-control test:
   assertNotHas(t, ids, "SC-DCK-006")
   ```

5. **Document it**: `make docs` regenerates `docs/rules.md` from the
   registry; add a row to the fixture table in `docs/fixtures.md` if you
   introduced a fixture; update the counts in `docs/false-positives.md`.

## Adding a parser or stack

Parsers live in `internal/parse` and are format-only (no rule logic).
Discovery lives in `internal/discovery`. A new stack means: recognize its
manifests in discovery (with ambiguity notes where intent is unclear), model
the minimal subset in parse, then write rules against the model. Malformed
files must degrade to explicit errors — a panic in a parser is a release
blocker.

## Security-sensitive changes

Anything touching `fsutil`, `secrets`, `validate`, or subprocess/exec code
requires a review from a maintainer and updates to
`docs/threat-model.md`. When in doubt, open an issue first.

## Reporting bugs

Open a GitHub issue with: the command, the exact exit code, the JSON output
(`--format json`), and the minimal fixture that reproduces it. Determinism
means we can reproduce everything you can.
