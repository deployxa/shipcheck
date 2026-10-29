// Package output renders scan results as terminal text, versioned JSON, or
// SARIF 2.1.0. Machine-readable formats go to stdout (or --output file);
// human diagnostics never mix into them.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/deployxa/shipcheck/internal/model"
	"github.com/deployxa/shipcheck/internal/rules"
)

// Text renders the human-readable report.
func Text(w io.Writer, r *model.Report, threshold model.Severity) {
	m := r.Meta
	fmt.Fprintf(w, "Shipcheck %s — open-source production-readiness checks\n", m.Version)
	fmt.Fprintf(w, "Project: %s\n", m.ProjectRoot)
	if len(m.Projects) > 0 {
		fmt.Fprintf(w, "Detected projects: %s\n", strings.Join(m.Projects, ", "))
	}
	fmt.Fprintf(w, "Scanned %d files in %ds\n\n", m.FilesScanned, m.DurationSec)

	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "No findings. This is not a guarantee that a deploy will succeed.")
	} else {
		for _, f := range r.Findings {
			loc := ""
			if f.File != "" {
				loc = f.File
				if f.Line > 0 {
					loc += fmt.Sprintf(":%d", f.Line)
				}
				loc += " — "
			}
			fmt.Fprintf(w, "[%s/%s] %s %s%s\n", strings.ToUpper(string(f.Severity)), f.Confidence, f.RuleID, loc, f.Title)
			if f.Evidence != "" {
				fmt.Fprintf(w, "    evidence: %s\n", f.Evidence)
			}
			fmt.Fprintf(w, "    %s\n", f.Description)
			fmt.Fprintf(w, "    fix: %s\n", f.Remediation)
			fmt.Fprintln(w)
		}
	}

	// Summary sorted by severity rank, descending.
	counts := map[model.Severity]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	order := []model.Severity{model.SevCritical, model.SevHigh, model.SevMedium, model.SevLow, model.SevInfo}
	var parts []string
	for _, s := range order {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	fmt.Fprintf(w, "Summary: %s\n", strings.Join(parts, ", "))
	if len(m.Notes) > 0 {
		fmt.Fprintln(w, "Notes:")
		for _, n := range m.Notes {
			fmt.Fprintf(w, "  - %s\n", n)
		}
	}
	if m.Incomplete {
		fmt.Fprintf(w, "WARNING: analysis incomplete — %d skipped/unreadable item(s)\n", len(m.Errors))
		for _, e := range m.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}
	at := r.CountAtOrAbove(threshold)
	if at > 0 {
		fmt.Fprintf(w, "Exit condition: %d finding(s) at or above %s threshold (exit 1)\n", at, threshold)
	} else {
		fmt.Fprintf(w, "Exit condition: no findings at or above %s threshold (exit 0)\n", threshold)
	}
}

// JSON renders the versioned report document.
func JSON(w io.Writer, r *model.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// SARIF renders SARIF 2.1.0 with one rule definition per registry entry and
// one result per finding.
func SARIF(w io.Writer, r *model.Report) error {
	type ruleDef struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		ShortDescription struct {
			Text string `json:"text"`
		} `json:"shortDescription"`
		FullDescription struct {
			Text string `json:"text"`
		} `json:"fullDescription"`
		Help struct {
			Text string `json:"text"`
		} `json:"help"`
		DefaultConfiguration struct {
			Level string `json:"level"`
		} `json:"defaultConfiguration"`
		Properties map[string]string `json:"properties,omitempty"`
	}
	type result struct {
		RuleID  string `json:"ruleId"`
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
		Locations []struct {
			PhysicalLocation struct {
				ArtifactLocation struct {
					URI string `json:"uri"`
				} `json:"artifactLocation"`
				Region struct {
					StartLine int `json:"startLine"`
				} `json:"region"`
			} `json:"physicalLocation"`
		} `json:"locations,omitempty"`
		PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
		Properties          map[string]string `json:"properties,omitempty"`
	}
	type run struct {
		Tool struct {
			Driver struct {
				Name           string    `json:"name"`
				Version        string    `json:"version"`
				InformationURI string    `json:"informationUri"`
				Rules          []ruleDef `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []result `json:"results"`
	}
	type sarifDoc struct {
		Version string `json:"version"`
		Schema  string `json:"$schema"`
		Runs    []run  `json:"runs"`
	}

	// Rules used by this report (sorted for deterministic output).
	used := map[string]bool{}
	for _, f := range r.Findings {
		used[f.RuleID] = true
	}
	var ruleIDs []string
	for id := range used {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)

	var defs []ruleDef
	for _, id := range ruleIDs {
		spec := rules.SpecByID(id)
		if spec == nil {
			continue
		}
		d := ruleDef{ID: spec.ID, Name: spec.ID}
		d.ShortDescription.Text = spec.Title
		d.FullDescription.Text = spec.Description
		d.Help.Text = spec.Remediation
		d.DefaultConfiguration.Level = sarifLevel(spec.Severity)
		d.Properties = map[string]string{
			"category":   string(spec.Category),
			"confidence": string(spec.Confidence),
		}
		if spec.Limitation != "" {
			d.Properties["limitation"] = spec.Limitation
		}
		defs = append(defs, d)
	}

	var results []result
	for _, f := range r.Findings {
		res := result{
			RuleID: f.RuleID,
			Level:  sarifLevel(f.Severity),
			PartialFingerprints: map[string]string{
				"shipcheckRuleId/v1": f.RuleID,
			},
			Properties: map[string]string{
				"confidence":  string(f.Confidence),
				"check":       string(f.Check),
				"remediation": f.Remediation,
			},
		}
		if f.Evidence != "" {
			res.Properties["evidence"] = f.Evidence
		}
		res.Message.Text = f.Title
		if f.Description != "" {
			res.Message.Text += ": " + f.Description
		}
		if f.File != "" {
			loc := struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			}{}
			loc.PhysicalLocation.ArtifactLocation.URI = filepathToURI(f.File)
			loc.PhysicalLocation.Region.StartLine = f.Line
			res.Locations = append(res.Locations, loc)
		}
		results = append(results, res)
	}

	doc := sarifDoc{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
	}
	run0 := run{}
	run0.Tool.Driver.Name = "Shipcheck"
	run0.Tool.Driver.Version = r.Meta.Version
	run0.Tool.Driver.InformationURI = "https://github.com/deployxa/shipcheck"
	run0.Tool.Driver.Rules = defs
	run0.Results = results
	doc.Runs = []run{run0}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

func sarifLevel(s model.Severity) string {
	switch s {
	case model.SevCritical, model.SevHigh:
		return "error"
	case model.SevMedium:
		return "warning"
	default:
		return "note"
	}
}

// filepathToURI converts a slash-relative path to a SARIF artifact URI.
func filepathToURI(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
