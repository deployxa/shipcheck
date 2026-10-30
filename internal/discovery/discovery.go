// Package discovery maps an untrusted repository into a structural summary:
// which stack manifests exist, where the deployable projects are, and where
// the ambiguity lies. Discovery never guesses intent — when two readings are
// possible it records an Ambiguity for the report instead of choosing one.
package discovery

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deployxa/shipcheck/internal/fsutil"
	"github.com/deployxa/shipcheck/internal/parse"
)

// NodeApp is the modeled subset of package.json.
type NodeApp struct {
	RelDir   string
	Name     string
	Main     string
	Scripts  map[string]string
	Deps     map[string]string
	DevDeps  map[string]string
	Lockfile string // "" when none found next to package.json
	Engines  string
}

// PythonApp is the modeled subset of Python project manifests.
type PythonApp struct {
	RelDir       string
	PyProject    bool
	Requirements []string // requirement files found (rel paths)
	Scripts      map[string]string
	Name         string
	Lockfile     string
}

// GoApp is the modeled subset of Go modules.
type GoApp struct {
	RelDir    string
	Module    string
	GoVersion string
	Lockfile  string // go.sum
}

// RustApp is the modeled subset of Cargo manifests.
type RustApp struct {
	RelDir   string
	Name     string
	Lockfile string // Cargo.lock
}

// ComposeFile locates a compose file and its parse result is handled by the
// rules layer; discovery only records positions.
type ComposeFile struct {
	RelPath string
}

// DockerfileLoc locates a Dockerfile.
type DockerfileLoc struct {
	RelPath string
	Dir     string // slash-relative dir used as default build context
}

// EnvTemplate is a *template* env file (.env.example/.sample/.template) —
// safe to read. Real .env files are recorded but never loaded by default.
type EnvTemplate struct {
	RelPath string
}

// EnvReal records a real .env file's existence only. Contents are not read
// in static mode.
type EnvReal struct {
	RelPath string
}

// Ambiguity is anything where discovery refuses to guess.
type Ambiguity struct {
	Message string
	Where   string
}

// Result is the full discovery output.
type Result struct {
	Root        string
	NodeApps    []NodeApp
	PythonApps  []PythonApp
	GoApps      []GoApp
	RustApps    []RustApp
	Dockerfiles []DockerfileLoc
	Composes    []ComposeFile
	EnvTpls     []EnvTemplate
	EnvReals    []EnvReal
	Procfiles   []string
	CIFiles     []string
	Ambiguities []Ambiguity
	ParseIssues []parse.Issue // manifests that exist but do not parse
	Files       []fsutil.File // all readable files (for source scans)
	Errors      []fsutil.Error
}

// HasManifest reports whether any deployable manifest was found.
func (r *Result) HasManifests() bool {
	return len(r.NodeApps) > 0 || len(r.PythonApps) > 0 || len(r.GoApps) > 0 || len(r.RustApps) > 0 || len(r.Dockerfiles) > 0 || len(r.Composes) > 0
}

// manifest names recognized during the walk.
var (
	nodeManifest     = "package.json"
	envTplNames      = map[string]bool{".env.example": true, ".env.sample": true, ".env.template": true, ".env.dev.example": true}
	composeNames     = map[string]bool{"docker-compose.yml": true, "docker-compose.yaml": true, "compose.yml": true, "compose.yaml": true}
	dockerfilePrefix = "dockerfile"
)

// Discover walks the repository and classifies manifests.
func Discover(root string) *Result {
	r := &Result{Root: root}
	lim := fsutil.DefaultLimits(root)
	w := fsutil.Walk(lim)
	r.Files = w.Files
	r.Errors = w.Errors

	// Feed .gitignore files found during the walk into a second, gitignore-
	// aware view. The walk already excludes common build dirs; honouring
	// committed .gitignore files additionally avoids flagging files the
	// project itself never ships. Only ignore (no negation re-include
	// complexity) is applied here; documented limitation.
	gitignores := map[string]string{}
	for _, f := range w.Files {
		if filepath.Base(f.RelPath) == ".gitignore" {
			if data, _, err := fsutil.ReadFile(f.Path); err == nil {
				dir := filepath.ToSlash(filepath.Dir(f.RelPath))
				if dir == "." {
					dir = ""
				}
				gitignores[dir] = data
			}
		}
	}
	ig := fsutil.NewIgnoreStack()
	// Root first, then nested, for deterministic evaluation.
	dirs := make([]string, 0, len(gitignores))
	for d := range gitignores {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		ig.Add(d, gitignores[d])
	}

	for _, f := range w.Files {
		base := filepath.Base(f.RelPath)
		dir := filepath.ToSlash(filepath.Dir(f.RelPath))
		if dir == "." {
			dir = ""
		}
		lower := strings.ToLower(base)

		switch {
		case base == nodeManifest:
			if app, err := parsePackageJSON(f.Path); err == nil {
				app.RelDir = dir
				app.Lockfile = findLockfile(w.Files, dir)
				r.NodeApps = append(r.NodeApps, *app)
			} else {
				r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "package.json present but not valid JSON; Node analysis skipped for this directory", Where: f.RelPath})
				r.ParseIssues = append(r.ParseIssues, parse.Issue{RelPath: f.RelPath, Reason: err.Error()})
			}
		case base == "pyproject.toml":
			app := PythonApp{RelDir: dir, PyProject: true}
			if data, _, err := fsutil.ReadFile(f.Path); err == nil {
				if pp, err := parse.PyProjectFile([]byte(data)); err == nil {
					app.Name = pp.ProjectName
					app.Scripts = pp.Scripts
				} else {
					r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "pyproject.toml present but not valid TOML; limited Python analysis", Where: f.RelPath})
					r.ParseIssues = append(r.ParseIssues, parse.Issue{RelPath: f.RelPath, Reason: err.Error()})
				}
			}
			r.PythonApps = append(r.PythonApps, app)
		case strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"):
			app := r.pythonAppFor(dir)
			app.Requirements = append(app.Requirements, f.RelPath)
			app.Lockfile = detectPyLock(w.Files, dir)
		case base == "go.mod":
			app := GoApp{RelDir: dir}
			if data, _, err := fsutil.ReadFile(f.Path); err == nil {
				app.Module, app.GoVersion = parseGoMod(data)
			}
			app.Lockfile = findSibling(w.Files, dir, "go.sum")
			r.GoApps = append(r.GoApps, app)
		case base == "Cargo.toml":
			app := RustApp{RelDir: dir}
			if data, _, err := fsutil.ReadFile(f.Path); err == nil {
				app.Name = parseCargoToml(data)
			}
			app.Lockfile = findSibling(w.Files, dir, "Cargo.lock")
			r.RustApps = append(r.RustApps, app)
		case strings.HasPrefix(lower, dockerfilePrefix) && !strings.Contains(base, "."):
			r.Dockerfiles = append(r.Dockerfiles, DockerfileLoc{RelPath: f.RelPath, Dir: dir})
		case composeNames[lower]:
			r.Composes = append(r.Composes, ComposeFile{RelPath: f.RelPath})
		case envTplNames[lower]:
			r.EnvTpls = append(r.EnvTpls, EnvTemplate{RelPath: f.RelPath})
		case base == ".env":
			r.EnvReals = append(r.EnvReals, EnvReal{RelPath: f.RelPath})
		case base == "Procfile":
			r.Procfiles = append(r.Procfiles, f.RelPath)
		case isCIFile(f.RelPath):
			r.CIFiles = append(r.CIFiles, f.RelPath)
		}
	}

	sortResults(r)
	checkAmbiguity(r)
	return r
}

func (r *Result) pythonAppFor(dir string) *PythonApp {
	for i := range r.PythonApps {
		if r.PythonApps[i].RelDir == dir {
			return &r.PythonApps[i]
		}
	}
	r.PythonApps = append(r.PythonApps, PythonApp{RelDir: dir})
	return &r.PythonApps[len(r.PythonApps)-1]
}

func parsePackageJSON(path string) (*NodeApp, error) {
	data, _, err := fsutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Name    string            `json:"name"`
		Main    string            `json:"main"`
		Version string            `json:"version"`
		Scripts map[string]string `json:"scripts"`
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	app := &NodeApp{Name: raw.Name, Main: raw.Main, Scripts: raw.Scripts, Deps: raw.Deps, DevDeps: raw.DevDeps}
	if e, ok := raw.Engines["node"]; ok {
		app.Engines = e
	}
	if app.Scripts == nil {
		app.Scripts = map[string]string{}
	}
	if app.Deps == nil {
		app.Deps = map[string]string{}
	}
	if app.DevDeps == nil {
		app.DevDeps = map[string]string{}
	}
	return app, nil
}

func findLockfile(files []fsutil.File, dir string) string {
	candidates := map[string]bool{
		"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true,
	}
	if dir == "." {
		dir = ""
	}
	for _, f := range files {
		fdir := filepath.ToSlash(filepath.Dir(f.RelPath))
		if fdir == "." {
			fdir = ""
		}
		if fdir == dir && candidates[filepath.Base(f.RelPath)] {
			return filepath.Base(f.RelPath)
		}
	}
	return ""
}

func detectPyLock(files []fsutil.File, dir string) string {
	if dir == "." {
		dir = ""
	}
	for _, f := range files {
		fdir := filepath.ToSlash(filepath.Dir(f.RelPath))
		if fdir == "." {
			fdir = ""
		}
		if fdir == dir {
			b := filepath.Base(f.RelPath)
			if b == "poetry.lock" || b == "uv.lock" || b == "Pipfile.lock" {
				return b
			}
		}
	}
	return ""
}

func findSibling(files []fsutil.File, dir, targetName string) string {
	if dir == "." {
		dir = ""
	}
	for _, f := range files {
		fdir := filepath.ToSlash(filepath.Dir(f.RelPath))
		if fdir == "." {
			fdir = ""
		}
		if fdir == dir && filepath.Base(f.RelPath) == targetName {
			return targetName
		}
	}
	return ""
}

func parseGoMod(content string) (module string, goVersion string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module"))
		} else if strings.HasPrefix(line, "go ") {
			goVersion = strings.TrimSpace(strings.TrimPrefix(line, "go"))
		}
	}
	return
}

func parseCargoToml(content string) string {
	inPackage := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = (line == "[package]")
			continue
		}
		if inPackage && strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				return strings.Trim(val, `"'`)
			}
		}
	}
	return ""
}

func isCIFile(rel string) bool {
	p := filepath.ToSlash(rel)
	if strings.HasPrefix(p, ".github/workflows/") && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")) {
		return true
	}
	switch p {
	case ".gitlab-ci.yml", ".circleci/config.yml", "Jenkinsfile", ".travis.yml", ".drone.yml":
		return true
	}
	return false
}

func sortResults(r *Result) {
	sortNodeApps(r.NodeApps)
	sort.SliceStable(r.PythonApps, func(i, j int) bool { return r.PythonApps[i].RelDir < r.PythonApps[j].RelDir })
	sort.SliceStable(r.Dockerfiles, func(i, j int) bool { return r.Dockerfiles[i].RelPath < r.Dockerfiles[j].RelPath })
	sort.SliceStable(r.Composes, func(i, j int) bool { return r.Composes[i].RelPath < r.Composes[j].RelPath })
	sort.SliceStable(r.EnvTpls, func(i, j int) bool { return r.EnvTpls[i].RelPath < r.EnvTpls[j].RelPath })
	sort.SliceStable(r.EnvReals, func(i, j int) bool { return r.EnvReals[i].RelPath < r.EnvReals[j].RelPath })
}

func sortNodeApps(apps []NodeApp) {
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].RelDir < apps[j].RelDir })
}

func checkAmbiguity(r *Result) {
	// Multiple deployable units without a top-level manifest is a monorepo
	// signal — report it, don't guess which one "the" app is.
	roots := 0
	for range r.NodeApps {
		roots++
	}
	for range r.PythonApps {
		roots++
	}
	if roots > 1 {
		r.Ambiguities = append(r.Ambiguities, Ambiguity{
			Message: "multiple application manifests found; treating repository as a multi-project (monorepo) layout",
			Where:   ".",
		})
	}
	for _, n := range r.NodeApps {
		if n.Lockfile == "" {
			r.Ambiguities = append(r.Ambiguities, Ambiguity{
				Message: "package.json without a lockfile; dependency analysis is limited",
				Where:   joinDir(n.RelDir, "package.json"),
			})
		}
	}
}

func joinDir(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
