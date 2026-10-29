package envvar

import (
	"os"
	"strings"
	"testing"

	"github.com/deployxa/shipcheck/internal/fsutil"
)

func TestScanSourceJSAndPython(t *testing.T) {
	js := ScanSource("a.js", "const x = process.env.PORT;\nconst y = process.env['API_URL'];\n")
	if _, ok := js["PORT"]; !ok {
		t.Fatal("process.env.PORT missed")
	}
	if _, ok := js["API_URL"]; !ok {
		t.Fatal("bracket access missed")
	}
	py := ScanSource("b.py", "import os\nA = os.environ['TOKEN']\nB = os.environ.get('OPTIONAL', 'x')\nC = os.getenv('DEBUG', '0')\n")
	for _, want := range []string{"TOKEN", "OPTIONAL", "DEBUG"} {
		if _, ok := py[want]; !ok {
			t.Errorf("python ref %s missed", want)
		}
	}
}

func TestDynamicNamesAreInvisible(t *testing.T) {
	refs := ScanSource("a.js", `const name = "DY" + "NAMIC"; const v = process.env[name];`)
	if len(refs) != 0 {
		t.Fatalf("dynamic names must not be guessed: %v", refs)
	}
}

func TestTemplateDeclaration(t *testing.T) {
	a := New()
	a.DeclaredFromTemplate(".env.example", "PORT=3000\n# comment\nBAD LINE\nexport OK=yes\n_underscore=1\n")
	if _, ok := a.Declared["PORT"]; !ok {
		t.Fatal("PORT missed")
	}
	if _, ok := a.Declared["OK"]; !ok {
		t.Fatal("export prefix missed")
	}
	if _, ok := a.Declared["_underscore"]; !ok {
		t.Fatal("underscore names allowed")
	}
}

func TestUnresolvedAndUnreferenced(t *testing.T) {
	a := New()
	a.DeclaredFromTemplate(".env.example", "KNOWN=1\nSTALE=1\n")
	a.Referenced["KNOWN"] = []Ref{{File: "a.js", Kind: "source"}}
	a.Referenced["MISSING"] = []Ref{{File: "a.js", Kind: "source"}}
	a.Required["MISSING"] = a.Referenced["MISSING"]

	if got := a.Unresolved(); len(got) != 1 || got[0] != "MISSING" {
		t.Fatalf("unresolved = %v", got)
	}
	if got := a.Unreferenced(); len(got) != 1 || got[0] != "STALE" {
		t.Fatalf("unreferenced = %v", got)
	}
}

func TestDockerfileEnvNames(t *testing.T) {
	kvs := DockerfileEnvNames("FROM alpine\nENV A=1 B=2\nARG TOKEN\nENV QUOTED=\"x y\" OTHER=z\n")
	if len(kvs) != 3 {
		t.Fatalf("want 3 env/arg instructions, got %d", len(kvs))
	}
	if len(kvs[0].Names) != 2 || kvs[0].Names[0] != "A" {
		t.Fatalf("multi-var ENV wrong: %v", kvs[0].Names)
	}
	if len(kvs[1].Names) != 1 || kvs[1].Names[0] != "TOKEN" {
		t.Fatalf("bare ARG wrong: %v", kvs[1].Names)
	}
}

func TestScanProjectSkipsRealDotenv(t *testing.T) {
	dir := t.TempDir()
	fs := []fsutil.File{
		{Path: dir + "/.env", RelPath: ".env"},
		{Path: dir + "/a.js", RelPath: "a.js"},
	}
	// .env declares UNIQUE_VAR; source references MISSING_VAR.
	write(t, dir+"/.env", "UNIQUE_VAR=1\nDB_PASS=topsecret\n")
	write(t, dir+"/a.js", "const x = process.env.MISSING_VAR;\n")

	a := ScanProject(fs, true)
	if _, ok := a.Declared["UNIQUE_VAR"]; ok {
		t.Fatal("real .env must be skipped in static mode")
	}
	a2 := ScanProject(fs, false)
	if _, ok := a2.Declared["UNIQUE_VAR"]; !ok {
		t.Fatal("opt-in .env reading broken")
	}
	if got := a2.Unresolved(); len(got) != 1 || got[0] != "MISSING_VAR" {
		t.Fatalf("unresolved = %v", got)
	}
}

func TestNamesOnlyNoValuesSurvive(t *testing.T) {
	// The declared map must never carry values, only refs.
	a := New()
	a.DeclaredFromTemplate(".env.example", "SECRET_TOKEN=super-secret-value-999\n")
	for _, refs := range a.Declared {
		for _, r := range refs {
			blob := strings.Join([]string{r.File, r.Kind}, "|")
			if strings.Contains(blob, "super-secret") {
				t.Fatal("value leaked into refs")
			}
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
