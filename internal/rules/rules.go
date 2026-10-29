package rules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/deployxa/shipcheck/internal/config"
	"github.com/deployxa/shipcheck/internal/discovery"
	"github.com/deployxa/shipcheck/internal/envvar"
	"github.com/deployxa/shipcheck/internal/fsutil"
	"github.com/deployxa/shipcheck/internal/model"
	"github.com/deployxa/shipcheck/internal/parse"
)

// Input is everything the engine needs. The engine never touches the
// filesystem outside the files discovered by the walker.
type Input struct {
	Root  string
	Disc  *discovery.Result
	Files []fsutil.File
	Cfg   *config.Config
}

// Output carries findings and the env-var analysis for the report meta.
type Output struct {
	Findings []model.Finding
	Env      *envvar.Analysis
}

type engine struct {
	in       Input
	findings []model.Finding
	content  map[string]string // relPath -> content (lazy, cached)
	issues   []parse.Issue
	env      *envvar.Analysis
}

// Run executes every enabled rule. Order of findings follows rule
// registration and file iteration; both are deterministic.
func Run(in Input) Output {
	e := &engine{in: in, content: map[string]string{}}
	e.analyzeEnv() // populate e.env first; several rule groups consult it
	e.runDiscoveryRules()
	e.runDockerfileRules()
	e.runComposeRules()
	e.runAppRules()
	e.runEnvFindings()
	e.runSourceRules()
	e.runSecretRules()
	sortFindings(e.findings)
	return Output{Findings: e.findings, Env: e.env}
}

func sortFindings(fs []model.Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		if fs[i].Line != fs[j].Line {
			return fs[i].Line < fs[j].Line
		}
		return fs[i].RuleID < fs[j].RuleID
	})
}

func (e *engine) enabled(id string) bool { return !e.in.Cfg.IsDisabled(id) }

// emit adds a finding derived from a registered spec. Overrides let the
// call-site refine file/line/evidence without mutating the registry.
func (e *engine) emit(id string, ov func(*model.Finding)) {
	if !e.enabled(id) {
		return
	}
	spec := SpecByID(id)
	if spec == nil {
		return // unknown id: programming error; drop instead of guessing
	}
	f := spec.NewFinding()
	if ov != nil {
		ov(&f)
	}
	e.findings = append(e.findings, f)
}

// contentOf returns the (cached) text of a discovered file.
func (e *engine) contentOf(rel string) (string, bool) {
	if c, ok := e.content[rel]; ok {
		return c, true
	}
	for _, f := range e.in.Files {
		if f.RelPath == rel {
			data, _, err := fsutil.ReadFile(f.Path)
			if err != nil {
				e.issues = append(e.issues, parse.Issue{RelPath: rel, Reason: "unreadable: " + err.Error()})
				e.content[rel] = ""
				return "", false
			}
			e.content[rel] = data
			return data, true
		}
	}
	return "", false
}

// ---- discovery rules --------------------------------------------------------

func (e *engine) runDiscoveryRules() {
	d := e.in.Disc
	if !d.HasManifests() {
		e.emit("SC-DIS-001", nil)
	}
	// Real .env files: flag when no .gitignore mentions .env anywhere.
	if len(d.EnvReals) > 0 {
		mentioned := false
		for _, f := range e.in.Files {
			if filepath.Base(f.RelPath) == ".gitignore" {
				if c, ok := e.contentOf(f.RelPath); ok && envMentioned(c) {
					mentioned = true
					break
				}
			}
		}
		if !mentioned {
			for _, env := range d.EnvReals {
				e.emit("SC-DIS-002", func(f *model.Finding) {
					f.File = env.RelPath
					f.Evidence = "file present; no .gitignore pattern covers .env"
				})
			}
		}
	}
	// Manifests that exist but do not parse surface as SC-DIS-003 so the
	// problem is actionable in every output format.
	seen := map[string]bool{}
	for _, iss := range d.ParseIssues {
		if seen[iss.RelPath] {
			continue
		}
		seen[iss.RelPath] = true
		e.emit("SC-DIS-003", func(f *model.Finding) {
			f.File = iss.RelPath
			f.Evidence = truncate(iss.Reason, 160)
		})
	}
}

func envMentioned(gitignore string) bool {
	for _, line := range strings.Split(gitignore, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "!")
		line = strings.TrimPrefix(line, "/")
		if line == ".env" || line == ".env*" || line == "*.env" || strings.HasPrefix(line, ".env") {
			return true
		}
	}
	return false
}

// ---- Dockerfile rules -------------------------------------------------------

var devServerRe = regexp.MustCompile(
	`(\bnext dev\b|\bvite\b|\bnodemon\b|webpack-dev-server|\bwebpack serve\b|ts-node-dev|\bflask run\b|manage\.py runserver|uvicorn[^\n]*--reload|\bnpm run dev\b|\byarn dev\b|\bpnpm (run )?dev\b|\bastro dev\b|\bhugo server\b|php artisan serve)`)
var pipeToShellRe = regexp.MustCompile(`\|\s*(sudo\s+)?(ba)?sh\b`)
var buildOutputRe = regexp.MustCompile(`(dist/|build/|\.next/|\.nuxt/)`)

func (e *engine) runDockerfileRules() {
	for _, df := range e.in.Disc.Dockerfiles {
		content, ok := e.contentOf(df.RelPath)
		if !ok {
			continue
		}
		insts := parse.Dockerfile(content)
		e.dockerfileChecks(df, content, insts)
	}
}

func (e *engine) dockerfileChecks(df discovery.DockerfileLoc, content string, insts []parse.DockerInstruction) {
	hasUser, hasHealthcheck := false, false
	fromSeen := false
	for _, inst := range insts {
		switch inst.Cmd {
		case "USER":
			hasUser = true
		case "HEALTHCHECK":
			hasHealthcheck = true
		case "FROM":
			fromSeen = true
			if id, miss := unpinnedBaseImage(inst.Args); miss {
				e.emit("SC-DCK-001", func(f *model.Finding) {
					f.File, f.Line = df.RelPath, inst.Line
					f.Evidence = "FROM " + id
				})
			}
		case "RUN":
			if pipeToShellRe.MatchString(inst.Args) {
				e.emit("SC-DCK-004", func(f *model.Finding) {
					f.File, f.Line = df.RelPath, inst.Line
					f.Evidence = truncate("RUN "+inst.Args, 120)
				})
			}
		case "COPY", "ADD":
			e.checkCopyTargets(df, inst)
		case "CMD", "ENTRYPOINT":
			if inst.ShellForm {
				if devServerRe.MatchString(inst.Args) {
					e.emit("SC-BLD-001", func(f *model.Finding) {
						f.File, f.Line = df.RelPath, inst.Line
						f.Evidence = truncate(inst.Cmd+" "+inst.Args, 120)
					})
				}
				if inst.Args != "" {
					e.emit("SC-DCK-003", func(f *model.Finding) {
						f.File, f.Line = df.RelPath, inst.Line
						f.Evidence = truncate(inst.Cmd+" "+inst.Args, 120)
					})
				}
			} else {
				var args []string
				if arr, err := parse.DockerJSONArgs(inst.Args); err == nil && len(arr) > 0 {
					args = arr
				}
				joined := strings.Join(args, " ")
				if devServerRe.MatchString(joined) || (len(args) > 1 && devScriptRe.MatchString(strings.Trim(args[0], `"'`)) && isDevScriptName(args)) {
					e.emit("SC-BLD-001", func(f *model.Finding) {
						f.File, f.Line = df.RelPath, inst.Line
						f.Evidence = truncate(inst.Cmd+" "+inst.Args, 120)
					})
				}
			}
		}
	}
	if fromSeen && !hasUser {
		e.emit("SC-DCK-002", func(f *model.Finding) { f.File = df.RelPath })
	}
	if fromSeen && !hasHealthcheck {
		e.emit("SC-DCK-005", func(f *model.Finding) { f.File = df.RelPath })
	}
}

// isDevScriptName reports whether a JSON-form CMD/ENTRYPOINT runs a package
// script that looks like a dev script ("npm run dev" / ["npm","run","dev"]).
func isDevScriptName(args []string) bool {
	joined := strings.Join(args, " ")
	return regexp.MustCompile(`\b(dev|develop|serve|start:dev|dev:server)\b`).MatchString(joined) &&
		strings.HasPrefix(strings.Trim(args[0], `"'`), "npm|yarn|pnpm") == false &&
		(strings.Contains(joined, "npm ") || strings.Contains(joined, "yarn ") || strings.Contains(joined, "pnpm "))
}

var unpinnedTagRe = regexp.MustCompile(`^(.*?)(?::([^@]+))?(@sha256:[a-f0-9]+)?$`)

func unpinnedBaseImage(args string) (string, bool) {
	fields := strings.Fields(args)
	var img string
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue
		}
		img = f
		break
	}
	if img == "" || img == "scratch" {
		return "", false
	}
	if strings.Contains(img, "@") { // digest-pinned
		return "", false
	}
	if strings.Contains(img, ":") {
		tag := img[strings.LastIndex(img, ":")+1:]
		if tag != "" && tag != "latest" {
			return "", false
		}
	}
	return img, true
}

// checkCopyTargets validates COPY/ADD sources against the build context.
func (e *engine) checkCopyTargets(df discovery.DockerfileLoc, inst parse.DockerInstruction) {
	if strings.Contains(inst.Args, "--from=") {
		return // multi-stage copy; static resolution documented as skipped
	}
	tokens := strings.Fields(inst.Args)
	var srcs []string
	for i, t := range tokens {
		if strings.HasPrefix(t, "--") || strings.HasPrefix(t, `"`) && strings.HasSuffix(t, `"`) && i == 0 {
			continue
		}
		t = strings.Trim(t, `"'`)
		if strings.HasPrefix(t, "--") {
			continue
		}
		srcs = append(srcs, t)
	}
	if len(srcs) < 2 {
		return // malformed; SC-DIS-003 territory, skip quietly
	}
	srcs = srcs[:len(srcs)-1] // last token is destination

	ctxDir := df.Dir
	root := e.in.Root
	dockerIgnore := newDockerIgnore(e, root, ctxDir)

	for _, src := range srcs {
		if src == "" || strings.HasPrefix(src, "$") || src == "." || src == "/" {
			continue
		}
		if dockerIgnore.matched(src) {
			e.emit("SC-CTX-001", func(f *model.Finding) {
				f.File, f.Line = df.RelPath, inst.Line
				f.Evidence = src + " is excluded by .dockerignore but referenced by " + inst.Cmd
			})
			continue
		}
		if strings.ContainsAny(src, "*?[]") {
			continue // glob sources: existence verified only at build time
		}
		abs, ok := fsutil.Resolve(filepath.Join(root, filepath.FromSlash(ctxDir)), src)
		if !ok {
			continue // escaped the project: reported elsewhere, don't double-flag
		}
		if _, err := osStat(abs); err != nil {
			e.emit("SC-CTX-001", func(f *model.Finding) {
				f.File, f.Line = df.RelPath, inst.Line
				f.Evidence = inst.Cmd + " " + src + " — not present in build context " + ctxDir
			})
		}
	}
}

// ---- package scripts & app-level rules --------------------------------------

var devScriptRe = regexp.MustCompile(`\b(dev|develop|serve|start:dev)\b`)
var scriptRunnersRe = regexp.MustCompile(`^(node|ts-node|tsx|ts-node-dev|babel-node|python3?|sh|bash)$`)
var pathExtRe = regexp.MustCompile(`\.(js|mjs|cjs|ts|tsx|jsx|py|sh)$`)

func (e *engine) runAppRules() {
	startFound := false
	// Dockerfile CMD/ENTRYPOINT counts as a start command.
	for _, df := range e.in.Disc.Dockerfiles {
		if c, ok := e.contentOf(df.RelPath); ok {
			for _, inst := range parse.Dockerfile(c) {
				if inst.Cmd == "CMD" || inst.Cmd == "ENTRYPOINT" {
					startFound = true
				}
			}
		}
	}
	for _, pf := range e.in.Disc.Procfiles {
		startFound = true
		_ = pf
		e.procfileChecks(pf)
	}

	for _, app := range e.in.Disc.NodeApps {
		if s, ok := app.Scripts["start"]; ok && s != "" {
			startFound = true
			if devServerRe.MatchString(s) || (devScriptRe.MatchString(s) && strings.HasPrefix(s, "npm run")) {
				e.emit("SC-BLD-001", func(f *model.Finding) {
					f.File = joinRel(app.RelDir, "package.json")
					f.Evidence = truncate(`"start": "`+s+`"`, 120)
				})
			}
		}
		e.scriptFileChecks(app)
		e.depPinningNode(app)
		e.buildOutputChecks(app)
	}
	for _, py := range e.in.Disc.PythonApps {
		if len(py.Requirements) > 0 || py.PyProject {
			if cmd, ok := py.Scripts["start"]; ok && cmd != "" {
				startFound = true
			}
		}
		e.depPinningPython(py)
	}
	// START-001 applies only when application manifests exist; image-only
	// compose services declare their entrypoint inside the image.
	if !startFound && (len(e.in.Disc.NodeApps) > 0 || len(e.in.Disc.PythonApps) > 0) {
		e.emit("SC-START-001", nil)
	}
}

func (e *engine) procfileChecks(rel string) {
	c, ok := e.contentOf(rel)
	if !ok {
		return
	}
	for _, line := range strings.Split(c, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "web:") || strings.HasPrefix(trimmed, "web ") {
			cmd := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "web:"), "web"))
			if devServerRe.MatchString(cmd) {
				e.emit("SC-BLD-001", func(f *model.Finding) {
					f.File = rel
					f.Evidence = truncate("web: "+cmd, 120)
				})
			}
		}
	}
}

func (e *engine) scriptFileChecks(app discovery.NodeApp) {
	pkgRel := joinRel(app.RelDir, "package.json")
	for name, script := range app.Scripts {
		fields := strings.FieldsFunc(script, func(r rune) bool { return r == ' ' || r == '\t' })
		if len(fields) < 2 {
			continue
		}
		runner := strings.Trim(fields[0], `"`)
		if !scriptRunnersRe.MatchString(runner) {
			continue
		}
		cand := fields[1]
		if strings.HasPrefix(cand, "-") || strings.HasPrefix(cand, "$") || strings.HasPrefix(cand, `"`) {
			continue
		}
		if !strings.Contains(cand, "/") && !pathExtRe.MatchString(cand) {
			continue
		}
		root := e.in.Root
		base := root
		if app.RelDir != "" {
			base = filepath.Join(root, filepath.FromSlash(app.RelDir))
		}
		abs, ok := fsutil.Resolve(base, cand)
		if !ok {
			continue
		}
		if _, err := osStat(abs); err != nil {
			appDir := app.RelDir
			e.emit("SC-SCR-001", func(f *model.Finding) {
				f.File = pkgRel
				f.Evidence = `scripts["` + name + `"] -> ` + cand
				f.Remediation = "Fix the path in the script or add the missing file (relative to " + orDot(appDir) + ")."
			})
		}
	}
}

func (e *engine) buildOutputChecks(app discovery.NodeApp) {
	start, ok := app.Scripts["start"]
	if !ok {
		return
	}
	if !buildOutputRe.MatchString(start) {
		return
	}
	buildScript, hasBuild := app.Scripts["build"]
	if !hasBuild || strings.TrimSpace(buildScript) == "" {
		return
	}
	// Any Dockerfile in the same dir that runs the build script?
	for _, df := range e.in.Disc.Dockerfiles {
		if df.Dir != app.RelDir {
			continue
		}
		if c, ok := e.contentOf(df.RelPath); ok {
			for _, inst := range parse.Dockerfile(c) {
				if inst.Cmd == "RUN" && strings.Contains(inst.Args, "build") {
					return
				}
			}
		}
	}
	e.emit("SC-BLD-002", func(f *model.Finding) {
		f.File = joinRel(app.RelDir, "package.json")
		f.Evidence = truncate(`"start": "`+start+`"`, 120)
	})
}

func orDot(d string) string {
	if d == "" {
		return "."
	}
	return d
}

func joinRel(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
