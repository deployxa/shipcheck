# False-positive methodology

False positives are the failure mode that would make Shipcheck useless.
This document defines the severity/confidence model, states what each rule
class does and does not claim, and how false positives are measured.

## Severity × confidence × check nature

Every finding carries three axes:

| Axis | Values | Meaning |
| --- | --- | --- |
| `severity` | critical / high / medium / low / info | worst plausible impact if the evidence is real |
| `confidence` | confirmed / probable / possible | how strongly the analyzed files support the finding |
| `check` | static / validated / inconclusive | the nature of the analysis that produced it |

CI thresholds should key on severity; humans should read confidence first.

## Definitions the rules obey

Statements that hold by construction for every rule in the catalogue:

1. **Absence of a health check is not unhealthiness.** `SC-DCK-005` and
   `SC-HLT-001` are `info/possible` and explicitly say so.
2. **Not declared in a template ≠ missing at runtime.** `SC-ENV-001` is
   `medium/probable` with that caveat in its description; variables injected
   by platforms are invisible statically.
3. **A floating dependency is not a vulnerability.** `SC-DEP-001` and
   `SC-DCK-001` are `low/possible`, framed as reproducibility risk only.
4. **Running as root is not automatically exploitable.** `SC-DCK-002` is
   `medium/probable`, framed as hardening advice.
5. **No volume ≠ stateful bug.** `SC-FS-001` is `medium/possible` and fires
   only on recognized persistence evidence (SQLite/uploads) with no volumes
   anywhere.
6. **No evidence of graceful shutdown ≠ no graceful shutdown.** `SC-GRD-001`
   is `low/possible` with the evidence caveat in the description.
7. **privileged / host network / docker.sock** are severity-scaled to their
   blast radius but remain `probable` where legitimate use is conceivable —
   except the Docker socket mount (`SC-CMP-007`), which is `critical/
   confirmed` because container-escape via the mounted socket is
   unconditional.
8. **A secret-shaped string in docs is probably an example.** Confirmed-tier
   matches in documentation files are downgraded to `probable`.

## Measured false positives on the fixture corpus

The fixture corpus in `testdata/fixtures/` is the measurement base:

| Fixture | Role | Rules expected | Rules observed | FP |
| --- | --- | --- | --- | --- |
| `healthy-node` | negative control | 0 | 0 | 0 |
| `compose-stack` | negative control | 0 | 0 | 0 |
| `broken-node` | positive | 25 | 25 | 0 |
| `python-app` | mixed | 4 | 4 | 0 |
| `monorepo` | mixed | 2 | 2 | 0 |
| `fs-persist` | positive (FS-001) | 5 | 5 | 0 |
| `empty` | degenerate | 1 | 1 | 0 |

Denominator and method: false positives are counted as *rule × fixture*
pairs where a rule fires on a fixture whose documentation says it must not.
The negative-control fixtures are deliberately realistic (a clean
Node/Docker service, a clean compose stack) rather than minimal. The test
suite asserts the table above continuously (`internal/rules/rules_test.go`),
so a change that introduces a false positive fails CI.

**These numbers say nothing about real-world precision.** They measure the
implementation against its own specification. Real-world false-positive
rates depend on project conventions and are not claimed anywhere.

## How to dispute a false positive

1. Check the rule's `limitation` in `shipcheck rules --format json` — many
   are documented deliberately (e.g. `SC-CMP-002` treats external volumes as
   missing).
2. If the evidence is genuinely wrong, open an issue with the rule ID, the
   finding, and a minimal manifest. Fixes that weaken a rule must come with
   a fixture pair, exactly like new rules.
