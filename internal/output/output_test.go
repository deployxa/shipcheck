package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deployxa/shipcheck/internal/model"
)

func sampleReport() *model.Report {
	r := model.NewReport("/tmp/demo", "0.1.0")
	r.Meta.FilesScanned = 12
	r.Meta.Projects = []string{"node:."}
	r.Findings = append(r.Findings, model.Finding{
		RuleID: "SC-CTX-001", Title: "COPY target missing",
		Description: "d", Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Check: model.ChkStatic, File: "Dockerfile", Line: 4,
		Evidence: "COPY config/x.json", Remediation: "add the file",
	})
	r.Findings = append(r.Findings, model.Finding{
		RuleID: "SC-HLT-001", Title: "No healthcheck",
		Description: "d", Severity: model.SevInfo, Confidence: model.ConfPossible,
		Check: model.ChkStatic, Remediation: "add one",
	})
	return r
}

func TestJSONRoundTrip(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sampleReport()); err != nil {
		t.Fatal(err)
	}
	var doc model.Report
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(doc.Findings) != 2 || doc.Findings[0].RuleID != "SC-CTX-001" {
		t.Fatalf("round trip mismatch: %+v", doc)
	}
	if doc.Meta.Tool != "shipcheck" {
		t.Fatal("meta lost")
	}
}

func TestSARIFStructure(t *testing.T) {
	var b bytes.Buffer
	if err := SARIF(&b, sampleReport()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID  string `json:"id"`
						Def struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleId string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("invalid sarif: %v", err)
	}
	if doc.Version != "2.1.0" || len(doc.Runs) != 1 {
		t.Fatal("envelope wrong")
	}
	if len(doc.Runs[0].Results) != 2 {
		t.Fatal("results lost")
	}
	levels := map[string]string{}
	for _, r := range doc.Runs[0].Results {
		levels[r.RuleId] = r.Level
	}
	if levels["SC-CTX-001"] != "error" || levels["SC-HLT-001"] != "note" {
		t.Fatalf("levels wrong: %v", levels)
	}
}

func TestTextReportContent(t *testing.T) {
	var b bytes.Buffer
	Text(&b, sampleReport(), model.SevHigh)
	s := b.String()
	for _, want := range []string{
		"Shipcheck 0.1.0",
		"SC-CTX-001",
		"Summary: 1 high, 1 info",
		"Exit condition: 1 finding(s) at or above high threshold (exit 1)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("text output missing %q\n---\n%s", want, s)
		}
	}
}

func TestTextIncompleteWarning(t *testing.T) {
	r := sampleReport()
	r.Meta.Incomplete = true
	r.Meta.Errors = []string{"big.txt: file exceeds size limit; skipped"}
	var b bytes.Buffer
	Text(&b, r, model.SevHigh)
	if !strings.Contains(b.String(), "analysis incomplete") || !strings.Contains(b.String(), "big.txt") {
		t.Fatal("incomplete warning missing")
	}
}

func TestEmptyReportText(t *testing.T) {
	var b bytes.Buffer
	Text(&b, model.NewReport("/x", "0.1.0"), model.SevHigh)
	if !strings.Contains(b.String(), "No findings") {
		t.Fatal("empty report wording missing")
	}
}
