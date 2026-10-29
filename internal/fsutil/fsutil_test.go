package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContains(t *testing.T) {
	root := t.TempDir()
	if !Contains(root, filepath.Join(root, "a", "b")) {
		t.Fatal("child should be contained")
	}
	if Contains(root, filepath.Join(root, "..", "escape")) {
		t.Fatal("parent traversal must not be contained")
	}
	if Contains(root, root) == false {
		t.Fatal("root itself is contained")
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if p, ok := Resolve(root, "../../etc/passwd"); ok {
		t.Fatalf("traversal accepted: %q", p)
	}
	if p, ok := Resolve(root, "app/main.py"); !ok || p != filepath.Join(root, "app", "main.py") {
		t.Fatalf("normal path rejected: %q %v", p, ok)
	}
	// Absolute-style references are confined INTO the root (Docker COPY
	// semantics: paths are context-relative), never resolved on the host.
	p, ok := Resolve(root, "/etc/passwd")
	if !ok || !Contains(root, p) {
		t.Fatalf("absolute path must be confined inside root, got %q ok=%v", p, ok)
	}
}

func TestWalkSkipsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("fine"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := Walk(DefaultLimits(root))
	rels := map[string]bool{}
	for _, f := range res.Files {
		rels[f.RelPath] = true
	}
	if rels["leak.txt"] {
		t.Fatal("symlink escaping root was followed")
	}
	if !rels["ok.txt"] {
		t.Fatal("regular file missing from walk")
	}
	found := false
	for _, e := range res.Errors {
		if e.Path == "leak.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("escaping symlink must be recorded as a skipped error")
	}
}

func TestWalkRespectsExclusionsAndDepth(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "pkg", "index.js"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(root, "app.js"), []byte("x"), 0o600)

	res := Walk(DefaultLimits(root))
	for _, f := range res.Files {
		if filepath.HasPrefix(f.RelPath, "node_modules") {
			t.Fatal("excluded directory was walked")
		}
	}
}

func TestWalkFlagsOversizeAndBinary(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "bin.dat"), []byte{'P', 'N', 0, 0, 'G'}, 0o600)
	big := make([]byte, MaxFileSize+1)
	os.WriteFile(filepath.Join(root, "big.txt"), big, 0o600)

	res := Walk(DefaultLimits(root))
	if len(res.Files) != 0 {
		t.Fatalf("expected no readable files, got %d", len(res.Files))
	}
	kinds := map[string]bool{}
	for _, e := range res.Errors {
		kinds[e.Path] = true
	}
	if !kinds["bin.dat"] || !kinds["big.txt"] {
		t.Fatalf("expected errors for binary and oversize files, got %v", kinds)
	}
}

func TestGitignoreMatch(t *testing.T) {
	ig := NewIgnoreStack()
	ig.Add("", "build/\n*.log\n!important.log\n")
	if !ig.match("build/out.js", true) {
		t.Fatal("build dir should be ignored")
	}
	if !ig.match("build", true) {
		t.Fatal("build itself should be ignored")
	}
	if !ig.match("debug.log", false) {
		t.Fatal("*.log should be ignored")
	}
	if ig.match("important.log", false) {
		t.Fatal("negation should un-ignore important.log")
	}
	if ig.match("src/app.js", false) {
		t.Fatal("unrelated file should not be ignored")
	}
}
