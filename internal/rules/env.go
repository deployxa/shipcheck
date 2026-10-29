package rules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/deployxa/shipcheck/internal/discovery"
	"github.com/deployxa/shipcheck/internal/envvar"
	"github.com/deployxa/shipcheck/internal/fsutil"
	"github.com/deployxa/shipcheck/internal/model"
	"github.com/deployxa/shipcheck/internal/parse"
	"github.com/deployxa/shipcheck/internal/secrets"
)

type discoveryNodeApp = discovery.NodeApp
type discoveryPythonApp = discovery.PythonApp

// analyzeEnv builds the env-var name analysis once, before any rule runs.
// Declaration sources: .env.* templates, Dockerfile ENV/ARG, compose
// environment maps, and NAMES-ONLY from files referenced by compose
// env_file. Real .env files are skipped unless the project config opted in.
func (e *engine) analyzeEnv() {
	e.env = envvar.ScanProject(e.in.Files, e.in.Cfg.SkipDotenv())
	for _, ctx := range e.composeContexts() {
		if !ctx.parsed {
			continue
		}
		for _, svc := range sortedServices(ctx.doc) {
			var names []string
			for k := range svc.Env {
				names = append(names, k)
			}
			sort.Strings(names)
			e.env.DeclaredFromKV("compose", ctx.rel, names, 0)
			// env_file: read names only; values are discarded here. Kind is
			// "envfile" so ENV-002 does not flag deployment-tooling inputs.
			for _, ef := range svc.EnvFiles {
				data, ok := e.readFileNamesOnly(ef, ctx.dir)
				if !ok {
					continue
				}
				var names []string
				for k := range parse.EnvFile(data) {
					names = append(names, k)
				}
				e.env.DeclaredFromKV("envfile", "env_file:"+ef, names, 0)
			}
		}
	}
}

// readFileNamesOnly reads a file inside the project for env-file NAME
// collection. Values never leave this function: the content is parsed and
// discarded, and only the raw text is passed to the names-only parser.
func (e *engine) readFileNamesOnly(rel, dir string) (string, bool) {
	if strings.HasPrefix(rel, "$") {
		return "", false
	}
	base := e.in.Root
	if dir != "" {
		base = filepath.Join(e.in.Root, filepath.FromSlash(dir))
	}
	abs, ok := fsutil.Resolve(base, rel)
	if !ok {
		return "", false
	}
	for _, f := range e.in.Files {
		if f.Path == abs {
			data, _, err := fsutil.ReadFile(f.Path)
			if err != nil {
				return "", false
			}
			return data, true
		}
	}
	return "", false
}

// analyzeEnv needs parsed compose docs before runComposeRules; cache them.
func (e *engine) composeContexts() []composeCtx {
	var out []composeCtx
	for _, cf := range e.in.Disc.Composes {
		c, ok := e.contentOf(cf.RelPath)
		if !ok {
			continue
		}
		doc, err := parse.ComposeFile([]byte(c))
		dir := filepath.ToSlash(filepath.Dir(cf.RelPath))
		if dir == "." {
			dir = ""
		}
		if err != nil {
			continue // runComposeRules records the parse issue
		}
		out = append(out, composeCtx{rel: cf.RelPath, dir: dir, doc: doc, raw: c, parsed: true})
	}
	return out
}

// ---- env findings -----------------------------------------------------------

var placeholderValueRe = regexp.MustCompile(`(?i)^(\$\{.*\}|<.*>|\*.*|xx*|changeme|example|sample|your[-_].*|my[-_].*|test.*|todo.*)$`)

func (e *engine) runEnvFindings() {
	// ENV-001: referenced but nowhere declared.
	for _, name := range e.env.Unresolved() {
		refs := e.env.Referenced[name]
		first := refs[0]
		e.emit("SC-ENV-001", func(f *model.Finding) {
			f.File, f.Line = first.File, first.Line
			f.Evidence = name + " is read in " + countWord(refs) + " source location(s) but declared in no template"
		})
	}
	// ENV-002: declared but never referenced. Declarations consumed by
	// deployment tooling itself (compose environment keys, env_file names)
	// are skipped: the code scan cannot see their consumers.
	for _, name := range e.env.Unreferenced() {
		refs := e.env.Declared[name]
		codeFacing := false
		for _, r := range refs {
			if r.Kind == "template" || r.Kind == "dockerfile" {
				codeFacing = true
				break
			}
		}
		if !codeFacing {
			continue
		}
		first := refs[0]
		e.emit("SC-ENV-002", func(f *model.Finding) {
			f.File, f.Line = first.File, first.Line
			f.Evidence = name + " declared here is never read by the scanned code"
		})
	}
	// ENV-003: same name, different literal values across templates.
	tplNames := map[string][]string{} // name -> relPaths (sorted by scan order)
	values := map[string]map[string]string{}
	for _, f := range e.in.Files {
		base := strings.ToLower(filepath.Base(f.RelPath))
		if !isEnvTemplate(base) {
			continue
		}
		c, ok := e.contentOf(f.RelPath)
		if !ok {
			continue
		}
		for k, v := range parse.EnvFile(c) {
			tplNames[k] = append(tplNames[k], f.RelPath)
			if values[k] == nil {
				values[k] = map[string]string{}
			}
			if !placeholderValueRe.MatchString(v) && v != "" {
				values[k][f.RelPath] = v
			}
		}
	}
	for name, files := range tplNames {
		if len(files) < 2 {
			continue
		}
		vs := values[name]
		firstVal := ""
		conflict := false
		for _, file := range files {
			v, ok := vs[file]
			if !ok {
				continue
			}
			if firstVal == "" {
				firstVal = v
			} else if v != firstVal {
				conflict = true
				break
			}
		}
		if conflict {
			e.emit("SC-ENV-003", func(f *model.Finding) {
				f.File = files[0]
				f.Evidence = name + " has different values in " + strings.Join(files, ", ")
			})
		}
	}
}

func isEnvTemplate(base string) bool {
	switch base {
	case ".env.example", ".env.sample", ".env.template", ".env.dev.example":
		return true
	}
	return false
}

func countWord(refs []envvar.Ref) string {
	switch len(refs) {
	case 1:
		return "1"
	case 2:
		return "2"
	default:
		return itoa(len(refs))
	}
}

// ---- source-level rules (graceful shutdown) ----------------------------------

var sigtermNodeRe = regexp.MustCompile(`(process\.on\s*\(\s*['"]SIGTERM|process\.on\s*\(\s*['"]SIGINT)`)
var sigtermPyRe = regexp.MustCompile(`(signal\.SIGTERM|SIGTERM|add_signal_handler|graceful[_-]?timeout)`)

func (e *engine) runSourceRules() {
	// Applicable only when application source exists AND a deployment unit
	// (Dockerfile / compose / Procfile / start script) was found. Image-only
	// compose stacks carry their runtime inside the image, which static
	// analysis cannot inspect - no finding rather than a guess.
	if !e.in.Disc.HasManifests() || !e.hasServiceEvidence() || !e.hasAppSource() {
		return
	}
	found := false
	for _, f := range e.in.Files {
		base := filepath.Base(f.RelPath)
		if !isSourceExt(base) {
			continue
		}
		c, ok := e.contentOf(f.RelPath)
		if !ok {
			continue
		}
		if sigtermNodeRe.MatchString(c) || sigtermPyRe.MatchString(c) {
			found = true
			break
		}
	}
	if !found {
		e.emit("SC-GRD-001", nil)
	}
}

func (e *engine) hasServiceEvidence() bool {
	if len(e.in.Disc.Dockerfiles) > 0 || len(e.in.Disc.Composes) > 0 || len(e.in.Disc.Procfiles) > 0 {
		return true
	}
	for _, app := range e.in.Disc.NodeApps {
		if _, ok := app.Scripts["start"]; ok {
			return true
		}
	}
	return false
}

// hasAppSource reports whether any application manifest (Node or Python)
// was discovered - i.e. whether there is source code to reason about.
func (e *engine) hasAppSource() bool {
	return len(e.in.Disc.NodeApps) > 0 || len(e.in.Disc.PythonApps) > 0
}

func isSourceExt(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".py":
		return true
	}
	return false
}

// ---- dependency pinning -------------------------------------------------------

func (e *engine) depPinningNode(app discoveryNodeApp) {
	var floating []string
	for name, ver := range app.Deps {
		if isFloatingNpm(ver) {
			floating = append(floating, name+"@"+ver)
		}
	}
	for name, ver := range app.DevDeps {
		if isFloatingNpm(ver) {
			floating = append(floating, "(dev) "+name+"@"+ver)
		}
	}
	if len(floating) == 0 {
		return
	}
	sort.Strings(floating)
	dir := app.RelDir
	e.emit("SC-DEP-001", func(f *model.Finding) {
		f.File = joinRel(dir, "package.json")
		f.Evidence = truncate(strings.Join(floating, ", "), 200)
	})
}

func isFloatingNpm(v string) bool {
	v = strings.TrimSpace(v)
	if v == "*" || v == "latest" || v == "" {
		return true
	}
	if strings.HasPrefix(v, "git") && !strings.Contains(v, "#") {
		return true
	}
	return false
}

func (e *engine) depPinningPython(app discoveryPythonApp) {
	var floating []string
	for _, reqFile := range app.Requirements {
		c, ok := e.contentOf(reqFile)
		if !ok {
			continue
		}
		for _, line := range strings.Split(c, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			if !strings.ContainsAny(line, "=<>!~") {
				name := line
				if i := strings.IndexAny(name, " ;["); i > 0 {
					name = name[:i]
				}
				floating = append(floating, name)
			}
		}
	}
	if len(floating) == 0 {
		return
	}
	sort.Strings(floating)
	e.emit("SC-DEP-001", func(f *model.Finding) {
		f.File = app.Requirements[0]
		f.Evidence = truncate(strings.Join(floating, ", "), 200)
	})
}

// ---- secrets -------------------------------------------------------------------

func (e *engine) runSecretRules() {
	var files []struct{ RelPath, Content string }
	for _, f := range e.in.Files {
		if c, ok := e.contentOf(f.RelPath); ok {
			files = append(files, struct{ RelPath, Content string }{f.RelPath, c})
		}
	}
	for _, m := range secrets.ScanAll(files) {
		e.emit(m.RuleID, func(f *model.Finding) {
			f.File, f.Line = m.File, m.Line
			f.Evidence = m.Masked + " — " + m.Kind
			if m.Downgrade {
				f.Confidence = model.ConfProbable
			} else {
				switch m.Tier {
				case secrets.Probable:
					f.Confidence = model.ConfProbable
				case secrets.NeedsReview:
					f.Confidence = model.ConfPossible
				}
			}
		})
	}
}
