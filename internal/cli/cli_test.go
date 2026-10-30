package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/deployxa/shipcheck/internal/model"
)

func fixture(name string) string {
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		panic(err)
	}
	return abs
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestExitCodesContract(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"healthy below threshold", []string{fixture("healthy-node")}, ExitOK},
		{"broken breaches threshold", []string{fixture("broken-node")}, ExitFindings},
		{"broken with info threshold", []string{fixture("broken-node"), "--severity-threshold", "info"}, ExitFindings},
		{"empty below threshold", []string{fixture("empty")}, ExitOK},
		{"usage: no args", []string{}, ExitUsage},
		{"usage: bad flag", []string{fixture("healthy-node"), "--nope"}, ExitUsage},
		{"usage: missing dir", []string{fixture("does-not-exist")}, ExitUsage},
		{"usage: bad format", []string{fixture("healthy-node"), "--format", "xml"}, ExitUsage},
		{"usage: bad threshold", []string{fixture("healthy-node"), "--severity-threshold", "extreme"}, ExitUsage},
		{"version", []string{"version"}, ExitOK},
		{"help", []string{"help"}, ExitOK},
		{"rules", []string{"rules"}, ExitOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, _ := runCLI(t, c.args...)
			if code != c.want {
				t.Errorf("exit = %d, want %d", code, c.want)
			}
		})
	}
}

func TestThresholdControlsExitCode(t *testing.T) {
	// broken-node has no critical findings? It has SC-CMP-007 critical.
	// Threshold low must also exit 1; healthy must stay 0 at any threshold.
	for _, th := range []string{"critical", "high", "low", "info"} {
		code, _, _ := runCLI(t, fixture("broken-node"), "--severity-threshold", th)
		if code != ExitFindings {
			t.Errorf("broken-node at threshold %s: exit %d, want 1", th, code)
		}
	}
	// healthy-node at threshold low/info: zero findings at all → 0.
	for _, th := range []string{"critical", "info"} {
		code, _, _ := runCLI(t, fixture("healthy-node"), "--severity-threshold", th)
		if code != ExitOK {
			t.Errorf("healthy-node at threshold %s: exit %d, want 0", th, code)
		}
	}
}

func TestJSONFormatIsParseableAndSchemaShaped(t *testing.T) {
	code, out, _ := runCLI(t, fixture("broken-node"), "--format", "json")
	if code != ExitFindings {
		t.Fatalf("exit = %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc["$schema"] != "https://deployxa.dev/schemas/shipcheck-report.v1.json" {
		t.Errorf("unexpected schema field: %v", doc["$schema"])
	}
	meta, _ := doc["meta"].(map[string]any)
	if meta == nil || meta["tool"] != "shipcheck" {
		t.Fatalf("meta missing: %v", doc["meta"])
	}
	findings, _ := doc["findings"].([]any)
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	for _, f := range findings {
		fm := f.(map[string]any)
		for _, k := range []string{"rule_id", "title", "severity", "confidence", "check", "description", "remediation"} {
			if _, ok := fm[k]; !ok {
				t.Errorf("finding missing key %q: %v", k, fm)
			}
		}
	}
}

func TestSARIFFormatIsValid(t *testing.T) {
	code, out, _ := runCLI(t, fixture("broken-node"), "--format", "sarif")
	if code != ExitFindings {
		t.Fatalf("exit = %d", code)
	}
	var sarif struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
				Locs   []struct {
					Phys struct {
						Artifact struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &sarif); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if sarif.Version != "2.1.0" || len(sarif.Runs) != 1 {
		t.Fatalf("unexpected SARIF envelope")
	}
	if len(sarif.Runs[0].Results) == 0 {
		t.Fatal("expected SARIF results")
	}
	if sarif.Runs[0].Tool.Driver.Name != "Shipcheck" {
		t.Fatal("driver name wrong")
	}
}

func TestScanIsDeterministic(t *testing.T) {
	stripped := func(s string) string {
		var doc map[string]any
		if err := json.Unmarshal([]byte(s), &doc); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		meta := doc["meta"].(map[string]any)
		meta["duration_seconds"] = 0
		b, _ := json.Marshal(doc)
		return string(b)
	}
	_, a, _ := runCLI(t, fixture("broken-node"), "--format", "json")
	_, b, _ := runCLI(t, fixture("broken-node"), "--format", "json")
	if stripped(a) != stripped(b) {
		t.Fatal("repeated scans of an unchanged project must produce identical JSON (modulo duration)")
	}
}

func TestDiagnosticsSeparatedFromMachineOutput(t *testing.T) {
	_, out, errOut := runCLI(t, fixture("broken-node"), "--format", "json")
	if strings.Contains(out, "config warning") || strings.Contains(out, "error:") {
		t.Fatal("diagnostics leaked into machine-readable stdout")
	}
	// stderr is allowed to carry diagnostics; JSON on stdout must stay pure.
	if !json.Valid([]byte(out)) {
		t.Fatal("stdout is not pure JSON")
	}
	_ = errOut
}

// TestStaticModeNeverExecutesProjectCode is the canary proof: the fixture's
// package.json defines lifecycle scripts and a start command that would
// create canary files if anything executed them. Static mode must leave no
// trace, locally or in /tmp.
func TestStaticModeNeverExecutesProjectCode(t *testing.T) {
	root := fixture("security-traps")
	tmpCanary := filepath.Join(os.TempDir(), "shipcheck-canary-tmp")
	os.Remove(tmpCanary)
	defer os.Remove(tmpCanary)

	code, _, _ := runCLI(t, root, "--format", "json")
	if code != ExitOK && code != ExitFindings && code != ExitIncomplete {
		t.Fatalf("unexpected exit %d", code)
	}
	for _, canary := range []string{
		filepath.Join(root, "canary-lifecycle.txt"),
		tmpCanary,
	} {
		if _, err := os.Stat(canary); err == nil {
			t.Fatalf("canary %s exists: project code was EXECUTED during static scan", canary)
		}
	}
}

func dockerPresent() bool {
	return exec.Command("docker", "version").Run() == nil
}

func TestScanWithUnreadableFilesReportsIncomplete(t *testing.T) {
	code, out, _ := runCLI(t, fixture("security-traps"), "--format", "json")
	var doc struct {
		Meta struct {
			Incomplete bool     `json:"incomplete"`
			Errors     []string `json:"errors"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !doc.Meta.Incomplete {
		t.Fatal("fixture with huge/binary/symlink files must mark scan incomplete")
	}
	if len(doc.Meta.Errors) == 0 {
		t.Fatal("expected recorded errors for skipped files")
	}
	if code != ExitIncomplete && code != ExitOK {
		t.Fatalf("exit = %d, want 4 (or 0 if threshold logic wins)", code)
	}
}

func TestEscapingSymlinkIsSkippedNotFollowed(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("TOP SECRET"), 0o600)
	os.Symlink(secret, filepath.Join(root, "leak.txt"))

	code, out, _ := runCLI(t, root, "--format", "json")
	var doc struct {
		Meta struct {
			Errors []string `json:"errors"`
		} `json:"meta"`
		Findings []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	joined := strings.Join(doc.Meta.Errors, "\n")
	if !strings.Contains(joined, "leak.txt") {
		t.Fatalf("escaping symlink must be reported as skipped, errors: %s", joined)
	}
	if !strings.Contains(joined, "escapes project root") {
		t.Fatalf("reason must say why: %s", joined)
	}
	_ = code
}

func TestOutputFileFlag(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.json")
	code, _, _ := runCLI(t, fixture("healthy-node"), "--format", "json", "--output", outPath)
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(outPath)
	if err != nil || !json.Valid(b) {
		t.Fatalf("output file missing or invalid: %v", err)
	}
}

func TestConfigFileStrictValidation(t *testing.T) {
	// The config must live inside the analyzed root (confinement rule), so
	// build a minimal project in a temp dir.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"cfgtest","scripts":{"start":"node index.js"}}`), 0o600)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte(`process.on("SIGTERM", () => {}); console.log(process.env.PORT);`), 0o600)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("PORT=3000\n"), 0o600)

	bad := filepath.Join(dir, ".shipcheck.yml")
	os.WriteFile(bad, []byte("severity_threshold: high\ntypo_key: true\n"), 0o600)
	code, _, errOut := runCLI(t, dir, "--config", bad)
	if code != ExitUsage {
		t.Fatalf("unknown config key must be a usage error, got %d", code)
	}
	if !strings.Contains(errOut, "typo_key") {
		t.Fatalf("error should name the unknown key: %s", errOut)
	}
	os.Remove(bad)
	good := filepath.Join(dir, ".shipcheck.yml")
	os.WriteFile(good, []byte("severity_threshold: low\ndisable_rules: [SC-GRD-001]\n"), 0o600)
	if code, _, _ := runCLI(t, dir, "--config", good); code != ExitOK {
		t.Fatalf("valid config rejected: %d", code)
	}
	// A config path outside the project root is rejected.
	code, _, _ = runCLI(t, fixture("healthy-node"), "--config", good)
	if code != ExitUsage {
		t.Fatalf("config outside project root must be usage error, got %d", code)
	}
}

func TestRulesCommandStableIDs(t *testing.T) {
	code, out, _ := runCLI(t, "rules")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	re := regexp.MustCompile(`SC-[A-Z]+-\d{3}`)
	ids := re.FindAllString(out, -1)
	if len(ids) < 30 {
		t.Fatalf("expected 30+ rules listed, got %d", len(ids))
	}
	code, jout, _ := runCLI(t, "rules", "--format", "json")
	if code != ExitOK || !json.Valid([]byte(jout)) {
		t.Fatal("rules --format json must be valid JSON")
	}
	code, mout, _ := runCLI(t, "rules", "--format", "markdown")
	if code != ExitOK || !strings.HasPrefix(mout, "# Shipcheck rule catalogue") {
		t.Fatal("rules --format markdown malformed")
	}
}

func TestVersionOutput(t *testing.T) {
	_, out, _ := runCLI(t, "version")
	if !strings.Contains(out, "shipcheck version") || !strings.Contains(out, "Deployxa") {
		t.Fatalf("version output wrong: %s", out)
	}
}

func TestDockerValidateWithoutDockerIsHonest(t *testing.T) {
	// compose-stack has compose files; if Docker is absent the scan must
	// report inconclusive validation, never pretend success.
	code, out, _ := runCLI(t, fixture("compose-stack"), "--docker-validate", "--format", "json")
	var doc struct {
		Meta struct {
			Incomplete bool     `json:"incomplete"`
			Errors     []string `json:"errors"`
		} `json:"meta"`
		Findings []struct {
			RuleID string `json:"rule_id"`
			Check  string `json:"check"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if dockerPresent() {
		t.Skip("docker available in environment; honest-unavailable path not exercised")
	}
	sawVAL := false
	for _, f := range doc.Findings {
		if f.RuleID == "SC-VAL-001" {
			sawVAL = true
			if f.Check != string(model.ChkInconclusive) {
				t.Fatalf("SC-VAL-001 must be inconclusive, got %s", f.Check)
			}
		}
		if f.RuleID == "SC-VAL-003" {
			t.Fatal("SC-VAL-003 (validated ok) must not appear when docker is unavailable")
		}
	}
	if !sawVAL {
		t.Fatal("expected honest SC-VAL-001 when docker is unavailable")
	}
	if !doc.Meta.Incomplete {
		t.Fatal("meta.incomplete must be true when validation could not run")
	}
	_ = code
}

func TestBaselineFilter(t *testing.T) {
	tmp := t.TempDir()
	baselinePath := filepath.Join(tmp, "baseline.json")

	// Step 1: Scan broken-node without baseline, saving output to baseline.json -> exit 1
	code1, _, _ := runCLI(t, fixture("broken-node"), "--format", "json", "--output", baselinePath)
	if code1 != ExitFindings {
		t.Fatalf("first scan: exit = %d, want 1", code1)
	}

	// Step 2: Re-run scan passing --baseline baseline.json -> all findings filtered -> exit 0
	code2, out2, _ := runCLI(t, fixture("broken-node"), "--format", "json", "--baseline", baselinePath)
	if code2 != ExitOK {
		t.Fatalf("baseline scan: exit = %d, want 0", code2)
	}

	var doc struct {
		Findings []json.RawMessage `json:"findings"`
		Meta     struct {
			Notes []string `json:"notes"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out2), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Findings) != 0 {
		t.Fatalf("expected 0 new findings, got %d", len(doc.Findings))
	}
	sawNote := false
	for _, n := range doc.Meta.Notes {
		if strings.Contains(n, "baseline:") {
			sawNote = true
			break
		}
	}
	if !sawNote {
		t.Fatalf("expected note about baseline filtering in meta.notes, got %v", doc.Meta.Notes)
	}

	// Step 3: Bad baseline file -> exit 2 (ExitUsage)
	code3, _, _ := runCLI(t, fixture("broken-node"), "--baseline", filepath.Join(tmp, "nonexistent.json"))
	if code3 != ExitUsage {
		t.Fatalf("nonexistent baseline: exit = %d, want 2", code3)
	}
}
