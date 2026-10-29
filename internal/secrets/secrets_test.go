package secrets

import (
	"strings"
	"testing"
)

func TestKnownTokenFormatsAreConfirmed(t *testing.T) {
	cases := []struct {
		ruleID string
		line   string
	}{
		{"SC-SEC-001", `key = "` + "AK" + `IAIOSFODNN7EXAMPLE"`},
		{"SC-SEC-002", `token: ` + "gh" + `p_0123456789abcdefghijklmnopqrstuvwxyzABC`},
		{"SC-SEC-003", "xo" + `xb-123456789-abcdefghij`},
		{"SC-SEC-004", "sk_" + `live_abcdefghijklmnop1234`},
		{"SC-SEC-005", "AI" + `zaSyD-1234567890abcdefghijklmnopqrstu`},
		{"SC-SEC-006", "-----" + "BEGIN RSA PRIVATE KEY-----"},
		{"SC-SEC-007", `NPM_TOKEN=` + "np" + `m_0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKL`},
	}
	for _, c := range cases {
		ms := ScanFile("src/app.js", c.line+"\n")
		found := false
		for _, m := range ms {
			if m.RuleID == c.ruleID {
				found = true
				if m.Tier != Confirmed {
					t.Errorf("%s: tier = %s, want confirmed", c.ruleID, m.Tier)
				}
			}
		}
		if !found {
			t.Errorf("%s not detected for %q", c.ruleID, c.line)
		}
	}
}

func TestValuesAreAlwaysMasked(t *testing.T) {
	raw := "AK" + "IAIOSFODNN7EXAMPLE"
	ms := ScanFile("config.py", `aws_key = "`+raw+`"`)
	if len(ms) == 0 {
		t.Fatal("expected a match")
	}
	all := ""
	for _, m := range ms {
		all += m.Masked
	}
	if strings.Contains(all, raw) {
		t.Fatal("full credential leaked into finding")
	}
	if !strings.Contains(all, "AKIA") && !strings.Contains(all, "*") {
		t.Fatal("mask should keep a short prefix and asterisks")
	}
}

func TestPlaceholdersAreFiltered(t *testing.T) {
	lines := []string{
		`password = "changeme"`,
		`api_key = "your-api-key-here"`,
		`secret = "${SECRET}"`,
		`token = "xxx"`,
		`password = process.env.PASSWORD`,
		`db_password = "<replace-me>"`,
	}
	for _, l := range lines {
		for _, m := range ScanFile("src/index.js", l+"\n") {
			if m.RuleID == "SC-SEC-009" {
				t.Errorf("placeholder line flagged: %q -> %q", l, m.Masked)
			}
		}
	}
}

func TestGenericAssignmentNeedsReview(t *testing.T) {
	ms := ScanFile("src/config.js", `const db_password = "s3cr3t-value-long";`)
	found := false
	for _, m := range ms {
		if m.RuleID == "SC-SEC-009" && m.Tier == NeedsReview {
			found = true
		}
	}
	if !found {
		t.Fatal("generic credential assignment should be needsReview")
	}
}

func TestDocsDowngradeConfirmedMatches(t *testing.T) {
	ms := ScanFile("README.md", `example: `+"AK"+`IAIOSFODNN7EXAMPLE`)
	if len(ms) == 0 {
		t.Fatal("docs should still be scanned")
	}
	for _, m := range ms {
		if m.Tier == Confirmed {
			t.Fatal("confirmed matches in docs must be downgraded")
		}
	}
}

func TestScanAllDeterministic(t *testing.T) {
	files := []struct{ RelPath, Content string }{
		{"b.py", `token = "` + "gh" + `p_0123456789abcdefghijklmnopqrstuvwxyzABC"`},
		{"a.js", `key = "` + "AK" + `IAIOSFODNN7EXAMPLE"`},
		{"a.js", "xo" + `xb-123456789-abcdefghij`},
	}
	r1 := ScanAll(files)
	r2 := ScanAll(files)
	if len(r1) == 0 || len(r1) != len(r2) {
		t.Fatalf("unstable scan: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i].File != r2[i].File || r1[i].Line != r2[i].Line || r1[i].RuleID != r2[i].RuleID {
			t.Fatalf("order differs at %d", i)
		}
	}
	if r1[0].File != "a.js" {
		t.Fatalf("expected a.js first, got %s", r1[0].File)
	}
}

func TestBearerAndAwsSecret(t *testing.T) {
	ms := ScanFile("src/http.js", `headers["Authorization"] = "Bearer abcdefghijklmnopqrstuvwxyz123456"`)
	foundBearer := false
	for _, m := range ms {
		if m.RuleID == "SC-SEC-010" {
			foundBearer = true
		}
	}
	if !foundBearer {
		t.Fatal("bearer token not detected")
	}
}
