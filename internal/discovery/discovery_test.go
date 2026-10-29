package discovery

import (
	"path/filepath"
	"testing"
)

func discover(t *testing.T, name string) *Result {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return Discover(abs)
}

func TestHealthyNodeDiscovery(t *testing.T) {
	r := discover(t, "healthy-node")
	if len(r.NodeApps) != 1 {
		t.Fatalf("want 1 node app, got %d", len(r.NodeApps))
	}
	app := r.NodeApps[0]
	if app.Lockfile != "package-lock.json" {
		t.Fatalf("lockfile = %q", app.Lockfile)
	}
	if _, ok := app.Scripts["start"]; !ok {
		t.Fatal("start script missing")
	}
	if len(r.Dockerfiles) != 1 || len(r.Composes) != 1 || len(r.EnvTpls) != 1 {
		t.Fatalf("manifests wrong: df=%d cmp=%d tpl=%d", len(r.Dockerfiles), len(r.Composes), len(r.EnvTpls))
	}
	if len(r.EnvReals) != 0 {
		t.Fatal("no real .env expected")
	}
}

func TestBrokenNodeDiscovery(t *testing.T) {
	r := discover(t, "broken-node")
	if len(r.EnvReals) != 1 {
		t.Fatalf("real .env not found: %d", len(r.EnvReals))
	}
	if len(r.NodeApps) != 1 || r.NodeApps[0].Lockfile != "" {
		t.Fatalf("node app/lockfile wrong: %+v", r.NodeApps)
	}
}

func TestPythonDiscovery(t *testing.T) {
	r := discover(t, "python-app")
	if len(r.PythonApps) != 1 {
		t.Fatalf("want 1 python app, got %d", len(r.PythonApps))
	}
	if len(r.PythonApps[0].Requirements) != 1 {
		t.Fatalf("requirements not tracked: %+v", r.PythonApps[0])
	}
	if len(r.Dockerfiles) != 1 {
		t.Fatal("Dockerfile missed")
	}
}

func TestMonorepoDiscovery(t *testing.T) {
	r := discover(t, "monorepo")
	if len(r.NodeApps) != 2 {
		t.Fatalf("want 2 node apps, got %d", len(r.NodeApps))
	}
	multi := false
	for _, a := range r.Ambiguities {
		if len(a.Message) > 0 {
			multi = true
		}
	}
	if !multi {
		t.Fatal("monorepo ambiguity not recorded")
	}
}

func TestDiscoveryExcludesBuildDirs(t *testing.T) {
	r := discover(t, "healthy-node")
	for _, f := range r.Files {
		if f.RelPath == "node_modules/x/index.js" {
			t.Fatal("node_modules walked")
		}
	}
}

func TestCIFileDetection(t *testing.T) {
	if !isCIFile(".github/workflows/ci.yml") || !isCIFile(".gitlab-ci.yml") || isCIFile("docs/ci.yml") {
		t.Fatal("CI file classification wrong")
	}
}
