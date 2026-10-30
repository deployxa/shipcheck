# Authoring a New Rule for Shipcheck (30-Minute Guide)

Adding a new rule to Shipcheck is the most direct way to contribute. Rules in Shipcheck are **pure, self-contained, and deterministic**. You do not need deep knowledge of the entire architecture to implement one.

Follow these 4 steps:

---

## Step 1: Register the Rule Specification

Open [`internal/rules/registry.go`](internal/rules/registry.go). Add your new rule entry into the `Specs` slice using a stable contract ID:

```go
{
    ID:          "SC-DCK-006",
    Title:       "Missing multi-stage final artifact copy",
    Category:    CatDockerfile,
    Severity:    model.SevMedium,
    Confidence:  model.ConfProbable,
    Description: "A multi-stage build creates a build stage but does not copy artifacts into the final stage.",
    Remediation: "Add COPY --from=<stage> to transfer build output into the production runtime stage.",
    Limitation:  "Stages with custom aliases or external image references are not checked.",
},
```

### Selecting Category, Severity, and Confidence

- **ID Convention:** Must follow `SC-<CAT>-<NUM>` (e.g., `SC-DCK-006`, `SC-CMP-009`, `SC-ENV-005`).
- **Severity Guidelines:**
  - `critical`: Guaranteed deployment crash, container breakout, or credential exposure.
  - `high`: Broken configuration that fails orchestrator startup.
  - `medium`: Production anti-pattern, drift, or unhardened default.
  - `low`: Advisory with low operational risk.
- **Confidence Guidelines:**
  - `confirmed`: Structural fact (syntax error, missing file).
  - `probable`: Strong evidence; edge cases possible.
  - `possible`: Heuristic observation.

---

## Step 2: Implement the Detection Check

In the appropriate rules file (e.g. `internal/rules/dockerfile.go`, `internal/rules/compose.go`, or `internal/rules/rules.go`), implement your check:

```go
func (e *engine) checkMyNewCondition() {
    for _, df := range e.in.Disc.Dockerfiles {
        // Inspect parsed instructions or raw file contents
        content, ok := e.contentOf(df.RelPath)
        if !ok {
            continue
        }

        // Evaluate your logic
        if conditionDetected {
            e.emit("SC-DCK-006", func(f *model.Finding) {
                f.File = df.RelPath
                f.Line = 12
                f.Evidence = "evidence snippet"
            })
        }
    }
}
```

Call your function from the appropriate stage in `Run` (e.g., `e.runDockerfileRules()`).

---

## Step 3: Add Positive & Negative Tests

Open [`internal/rules/rules_test.go`](internal/rules/rules_test.go). Add two test cases:

1. **Positive Test:** Verifies that the rule fires when the defect is present.
2. **Negative Test:** Verifies that clean code **never** triggers the rule.

```go
func TestSC_DCK_006_FiresOnMissingArtifactCopy(t *testing.T) {
    // Setup synthetic input...
    out := Run(inputWithDefect)
    assertHasRule(t, out.Findings, "SC-DCK-006")
}

func TestSC_DCK_006_StaysSilentOnValidMultiStage(t *testing.T) {
    // Setup clean input...
    out := Run(cleanInput)
    assertNotHasRule(t, out.Findings, "SC-DCK-006")
}
```

---

## Step 4: Regenerate Documentation and Validate

Run the local verification gate:

```bash
# 1. Regenerate docs/rules.md automatically from your registry entry:
make docs

# 2. Run the full test and formatting gate:
make check
```

Once `make check` passes, submit your pull request!
