package rules

import (
	"testing"

	"github.com/deployxa/shipcheck/internal/config"
	"github.com/deployxa/shipcheck/internal/discovery"
	"github.com/deployxa/shipcheck/internal/fsutil"
	"github.com/deployxa/shipcheck/internal/model"
)

func TestInlineSuppressions(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		ruleID     string
		line       int
		suppressed bool
	}{
		{
			name:       "same-line single rule ignore",
			content:    "const x = process.env.API_KEY; // shipcheck:ignore SC-ENV-001\nconst y = 1;",
			ruleID:     "SC-ENV-001",
			line:       1,
			suppressed: true,
		},
		{
			name:       "same-line other rule ignore",
			content:    "const x = process.env.API_KEY; // shipcheck:ignore SC-DCK-002\nconst y = 1;",
			ruleID:     "SC-ENV-001",
			line:       1,
			suppressed: false,
		},
		{
			name:       "same-line blanket ignore",
			content:    "FROM alpine:3.19 # shipcheck:ignore\nUSER root",
			ruleID:     "SC-DCK-001",
			line:       1,
			suppressed: true,
		},
		{
			name:       "next-line single rule ignore",
			content:    "# shipcheck:ignore-next-line SC-DCK-002\nFROM node:20\nCMD ['node']",
			ruleID:     "SC-DCK-002",
			line:       2,
			suppressed: true,
		},
		{
			name:       "next-line unmatching line",
			content:    "# shipcheck:ignore-next-line SC-DCK-002\nFROM node:20\nCMD ['node']",
			ruleID:     "SC-DCK-002",
			line:       3,
			suppressed: false,
		},
		{
			name:       "file-level single rule ignore",
			content:    "# shipcheck:ignore-file SC-DCK-002\nFROM node:20\nCMD ['node']",
			ruleID:     "SC-DCK-002",
			line:       2,
			suppressed: true,
		},
		{
			name:       "file-level blanket ignore",
			content:    "// shipcheck:ignore-file\nconsole.log(1);\nconsole.log(2);",
			ruleID:     "SC-SEC-001",
			line:       2,
			suppressed: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sm := parseSuppressions(c.content)
			got := sm.isSuppressed(c.ruleID, c.line)
			if got != c.suppressed {
				t.Errorf("isSuppressed(%q, %d) = %v; want %v", c.ruleID, c.line, got, c.suppressed)
			}
		})
	}
}

func TestCustomRules(t *testing.T) {
	cfg := &config.Config{
		CustomRules: []config.CustomRule{
			{
				ID:          "CUSTOM-001",
				Title:       "No hardcoded database passwords in compose",
				Severity:    "high",
				Confidence:  "confirmed",
				Files:       []string{"*compose*.yml"},
				Pattern:     `POSTGRES_PASSWORD:\s*['"]?[a-zA-Z0-9]+`,
				Remediation: "Use variable interpolation or Docker secrets.",
			},
		},
	}

	files := []fsutil.File{
		{
			Path:    "/tmp/docker-compose.yml",
			RelPath: "docker-compose.yml",
		},
	}

	in := Input{
		Root:  "/tmp",
		Disc:  &discovery.Result{Files: files},
		Files: files,
		Cfg:   cfg,
	}

	e := &engine{
		in: in,
		content: map[string]string{
			"docker-compose.yml": "services:\n  db:\n    environment:\n      POSTGRES_PASSWORD: mysecretpassword\n",
		},
	}

	e.runCustomRules()
	if len(e.findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(e.findings))
	}
	f := e.findings[0]
	if f.RuleID != "CUSTOM-001" {
		t.Errorf("RuleID = %q, want CUSTOM-001", f.RuleID)
	}
	if f.Severity != model.SevHigh {
		t.Errorf("Severity = %q, want high", f.Severity)
	}
	if f.Line != 4 {
		t.Errorf("Line = %d, want 4", f.Line)
	}
}
