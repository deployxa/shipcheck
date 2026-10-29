// Package model defines the shared finding and report types used by every
// stage of Shipcheck. Severity, confidence and check-nature vocabularies are
// deliberately small, closed sets so that machine-readable output stays
// stable across releases.
package model

import "time"

// Severity of a finding, ordered from most to least urgent.
type Severity string

const (
	SevCritical Severity = "critical"
	SevHigh     Severity = "high"
	SevMedium   Severity = "medium"
	SevLow      Severity = "low"
	SevInfo     Severity = "info"
)

// Rank returns a numeric ordering for severity comparisons.
func (s Severity) Rank() int {
	switch s {
	case SevCritical:
		return 5
	case SevHigh:
		return 4
	case SevMedium:
		return 3
	case SevLow:
		return 2
	case SevInfo:
		return 1
	}
	return 0
}

// ParseSeverity maps a user-supplied string to a Severity, defaulting to info.
func ParseSeverity(s string) Severity {
	switch Severity(s) {
	case SevCritical, SevHigh, SevMedium, SevLow:
		return Severity(s)
	}
	return SevInfo
}

// ValidSeverities lists accepted --severity-threshold values.
func ValidSeverities() []string { return []string{"critical", "high", "medium", "low", "info"} }

// Confidence describes how strongly the evidence supports the finding.
//
//	confirmed — the defect is verifiable from the analyzed files alone
//	probable  — a strong indicator; real projects may legitimately differ
//	possible  — best-practice hint; absence of evidence is not evidence of a fault
type Confidence string

const (
	ConfConfirmed Confidence = "confirmed"
	ConfProbable  Confidence = "probable"
	ConfPossible  Confidence = "possible"
)

// Check describes the nature of the analysis that produced the finding.
//
//	static      — derived from file contents without executing anything
//	validated   — confirmed by an explicit external tool (e.g. --docker-validate)
//	inconclusive— the check could not be performed; never a silent pass
type Check string

const (
	ChkStatic       Check = "static"
	ChkValidated    Check = "validated"
	ChkInconclusive Check = "inconclusive"
)

// Finding is a single, self-contained result. Every field required to act on
// the finding travels with it so that JSON/SARIF consumers need no extra
// context.
type Finding struct {
	RuleID      string     `json:"rule_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Severity    Severity   `json:"severity"`
	Confidence  Confidence `json:"confidence"`
	Check       Check      `json:"check"`
	File        string     `json:"file,omitempty"`
	Line        int        `json:"line,omitempty"`
	Evidence    string     `json:"evidence,omitempty"`
	Remediation string     `json:"remediation"`
}

// ScanMeta summarises how a scan ran. Deterministic fields only — the
// duration is rounded to whole seconds and repeated scans of an unchanged
// project produce identical JSON except for this field.
type ScanMeta struct {
	Tool         string      `json:"tool"`
	Version      string      `json:"version"`
	Schema       string      `json:"schema"`
	ProjectRoot  string      `json:"project_root"`
	Projects     []string    `json:"projects"`
	FilesScanned int         `json:"files_scanned"`
	DurationSec  int64       `json:"duration_seconds"`
	Incomplete   bool        `json:"incomplete"`
	Errors       []string    `json:"errors,omitempty"`
	EnvSummary   *EnvSummary `json:"environment_variables,omitempty"`
	// Notes carry discovery ambiguities and other observations that lower
	// certainty WITHOUT marking the scan incomplete.
	Notes []string `json:"notes,omitempty"`
}

// EnvSummary is the referenced/declared bookkeeping from the env-var module.
// It reports names only — never values.
type EnvSummary struct {
	Referenced []string `json:"referenced"`
	Declared   []string `json:"declared"`
	Required   []string `json:"required"`
	Optional   []string `json:"optional"`
	Unresolved []string `json:"unresolved"`
}

// Report is the top-level document emitted in JSON format. The schema
// version in Meta.Schema is bumped on every incompatible change.
type Report struct {
	Schema   string    `json:"$schema"`
	Meta     ScanMeta  `json:"meta"`
	Findings []Finding `json:"findings"`
}

// NewReport returns a report with the fixed schema identifier pre-filled.
func NewReport(root, version string) *Report {
	return &Report{
		Schema: "https://deployxa.dev/schemas/shipcheck-report.v1.json",
		Meta: ScanMeta{
			Tool: "shipcheck", Version: version, Schema: "v1",
			ProjectRoot: root, Projects: []string{}, Errors: []string{}, Notes: []string{},
		},
		Findings: []Finding{},
	}
}

// CountAtOrAbove returns how many findings meet the given threshold.
func (r *Report) CountAtOrAbove(th Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity.Rank() >= th.Rank() {
			n++
		}
	}
	return n
}

// Now is overridable in tests; production code reads the wall clock once.
var Now = time.Now
