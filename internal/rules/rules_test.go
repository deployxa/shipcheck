package rules

import (
	"path/filepath"
	"testing"

	"github.com/deployxa/shipcheck/internal/config"
	"github.com/deployxa/shipcheck/internal/discovery"
)

// fixtureRoot resolves a testdata fixture relative to this package.
func fixtureRoot(name string) string {
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		panic(err)
	}
	return abs
}

func runFixture(t *testing.T, name string) (map[string]bool, int) {
	t.Helper()
	root := fixtureRoot(name)
	disc := discovery.Discover(root)
	out := Run(Input{Root: root, Disc: disc, Files: disc.Files, Cfg: &config.Config{}})
	ids := map[string]bool{}
	for _, f := range out.Findings {
		ids[f.RuleID] = true
		if f.RuleID == "" {
			t.Fatalf("finding without rule id: %+v", f)
		}
		if SpecByID(f.RuleID) == nil {
			t.Fatalf("finding references unregistered rule %s", f.RuleID)
		}
		if f.Title == "" || f.Description == "" || f.Remediation == "" {
			t.Fatalf("finding %s missing actionable fields: %+v", f.RuleID, f)
		}
	}
	return ids, len(out.Findings)
}

func assertHas(t *testing.T, ids map[string]bool, want ...string) {
	t.Helper()
	for _, w := range want {
		if !ids[w] {
			t.Errorf("expected rule %s to fire; fired rules: %v", w, keys(ids))
		}
	}
}

func assertNotHas(t *testing.T, ids map[string]bool, forbidden ...string) {
	t.Helper()
	for _, f := range forbidden {
		if ids[f] {
			t.Errorf("rule %s must NOT fire on this fixture; fired: %v", f, keys(ids))
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHealthyNodeProducesNoFindings(t *testing.T) {
	ids, n := runFixture(t, "healthy-node")
	if n != 0 {
		t.Fatalf("healthy fixture must produce zero findings, got %d: %v", n, keys(ids))
	}
}

func TestBrokenNodeTriggersExpectedRules(t *testing.T) {
	ids, _ := runFixture(t, "broken-node")
	assertHas(t, ids,
		"SC-BLD-001",  // nodemon as start
		"SC-SCR-001",  // scripts/migrate.js missing
		"SC-DEP-001",  // "*" dependency
		"SC-ENV-001",  // API_KEY/DATABASE_URL undeclared
		"SC-ENV-002",  // LEGACY_FLAG unreferenced
		"SC-ENV-004",  // ${LOG_LEVEL} passthrough undeclared
		"SC-DCK-001",  // node:latest
		"SC-DCK-002",  // no USER
		"SC-DCK-003",  // shell-form CMD
		"SC-DCK-004",  // curl | sh
		"SC-DCK-005",  // no HEALTHCHECK
		"SC-CTX-001",  // config/production.json missing
		"SC-CMP-001",  // depends_on db2 undefined
		"SC-CMP-002",  // appdata volume undeclared
		"SC-CMP-003",  // env_file .env.missing
		"SC-CMP-005",  // privileged
		"SC-CMP-006",  // network_mode host
		"SC-CMP-007",  // docker.sock mount
		"SC-PORT-001", // hardcoded host port
		"SC-PORT-002", // EXPOSE 3000 vs maps 4000
		"SC-GRD-001",  // no SIGTERM handler
		"SC-HLT-001",  // no healthcheck
		"SC-DIS-002",  // real .env present
		"SC-SEC-001",  // AKIA key
		"SC-SEC-009",  // hardcoded password
	)
	// Volumes ARE declared, so the no-volume persistence rule must stay off.
	assertNotHas(t, ids, "SC-FS-001", "SC-DIS-001", "SC-START-001", "SC-DIS-003")
}

func TestPythonFixture(t *testing.T) {
	ids, _ := runFixture(t, "python-app")
	assertHas(t, ids, "SC-DEP-001", "SC-DCK-002", "SC-ENV-002", "SC-GRD-001")
	// SECRET_TOKEN is declared in .env.example; ENV-001 must NOT fire.
	assertNotHas(t, ids, "SC-ENV-001", "SC-BLD-001")
}

func TestComposeStackFixture(t *testing.T) {
	ids, n := runFixture(t, "compose-stack")
	if n != 0 {
		t.Fatalf("clean compose stack must produce zero findings, got %v", keys(ids))
	}
}

func TestFsPersistFixture(t *testing.T) {
	ids, _ := runFixture(t, "fs-persist")
	assertHas(t, ids, "SC-FS-001", "SC-ENV-001")
}

func TestEmptyFixture(t *testing.T) {
	ids, _ := runFixture(t, "empty")
	assertHas(t, ids, "SC-DIS-001")
	assertNotHas(t, ids, "SC-GRD-001", "SC-START-001")
}

func TestMonorepoFixture(t *testing.T) {
	root := fixtureRoot("monorepo")
	disc := discovery.Discover(root)
	if len(disc.NodeApps) != 2 {
		t.Fatalf("monorepo: want 2 node apps, got %d", len(disc.NodeApps))
	}
	foundMonorepoNote := false
	for _, a := range disc.Ambiguities {
		if len(a.Message) > 0 {
			foundMonorepoNote = true
		}
	}
	if !foundMonorepoNote {
		t.Fatalf("monorepo ambiguity must be recorded")
	}
	ids, _ := runFixture(t, "monorepo")
	assertHas(t, ids, "SC-ENV-001") // API_PORT undeclared
}

func TestSecurityTrapsDoNotCrashAndFlagParseIssues(t *testing.T) {
	ids, _ := runFixture(t, "security-traps")
	assertHas(t, ids, "SC-DIS-003") // malformed JSON + YAML
}

func TestRuleIDsAreStableAndDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Registry {
		if seen[s.ID] {
			t.Fatalf("duplicate rule id %s", s.ID)
		}
		seen[s.ID] = true
		if len(s.ID) < 8 || s.ID[:3] != "SC-" {
			t.Fatalf("rule id %s violates SC-xxx-nnn shape", s.ID)
		}
		if s.Description == "" || s.Remediation == "" {
			t.Fatalf("rule %s missing description/remediation", s.ID)
		}
	}
	if len(seen) < 30 {
		t.Fatalf("expected 30+ rules, have %d", len(seen))
	}
}

func TestDisabledRuleViaConfig(t *testing.T) {
	root := fixtureRoot("broken-node")
	disc := discovery.Discover(root)
	cfg := &config.Config{DisableRules: []string{"SC-CMP-007"}}
	out := Run(Input{Root: root, Disc: disc, Files: disc.Files, Cfg: cfg})
	for _, f := range out.Findings {
		if f.RuleID == "SC-CMP-007" {
			t.Fatal("disabled rule still fired")
		}
	}
}

func TestDeterministicOutput(t *testing.T) {
	root := fixtureRoot("broken-node")
	fingerprint := func() []string {
		disc := discovery.Discover(root)
		out := Run(Input{Root: root, Disc: disc, Files: disc.Files, Cfg: &config.Config{}})
		var out2 []string
		for _, f := range out.Findings {
			out2 = append(out2, f.RuleID+"|"+f.File+"|"+itoa(f.Line)+"|"+f.Evidence)
		}
		return out2
	}
	a, b := fingerprint(), fingerprint()
	if len(a) != len(b) {
		t.Fatalf("nondeterministic finding count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic finding %d: %q vs %q", i, a[i], b[i])
		}
	}
}
