package rules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/deployxa/shipcheck/internal/fsutil"
	"github.com/deployxa/shipcheck/internal/model"
	"github.com/deployxa/shipcheck/internal/parse"
)

var interpolationRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:[+-?][^}]*)?\}`)
var dockerSockRe = regexp.MustCompile(`/var/run/docker\.sock`)
var dangerousHostPaths = []string{"/", "/etc", "/var/run", "/root", "/proc", "/sys", "/usr", "/lib"}

// composeCtx carries one parsed compose file through the checks.
type composeCtx struct {
	rel    string // compose file rel path
	dir    string // its directory ("" = root)
	doc    *parse.Compose
	raw    string
	parsed bool
}

func (e *engine) runComposeRules() {
	var ctxs []composeCtx
	for _, cf := range e.in.Disc.Composes {
		c, ok := e.contentOf(cf.RelPath)
		if !ok {
			continue
		}
		doc, err := parse.ComposeFile([]byte(c))
		ctx := composeCtx{rel: cf.RelPath, dir: filepath.ToSlash(filepath.Dir(cf.RelPath)), raw: c}
		if dir := filepath.ToSlash(filepath.Dir(cf.RelPath)); dir == "." {
			ctx.dir = ""
		}
		if err != nil {
			e.issues = append(e.issues, parse.Issue{RelPath: cf.RelPath, Reason: err.Error()})
			continue
		}
		ctx.doc, ctx.parsed = doc, true
		ctxs = append(ctxs, ctx)
	}
	e.parseIssueFindings()

	for _, ctx := range ctxs {
		e.composeInternalRefs(ctx)
		e.composeSecurity(ctx)
		e.composePortsAndEnv(ctx)
	}
	e.composeHealthAndFS(ctxs)
}

// parseIssueFindings turns collected parse errors into SC-DIS-003 findings.
func (e *engine) parseIssueFindings() {
	seen := map[string]bool{}
	for _, iss := range e.issues {
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

func (e *engine) composeInternalRefs(ctx composeCtx) {
	doc := ctx.doc
	// Top-level declared volumes.
	declared := map[string]bool{}
	for name := range doc.Volumes {
		declared[name] = true
	}
	for _, svc := range sortedServices(doc) {
		for _, dep := range svc.DependsOn {
			if _, ok := doc.Services[dep]; !ok {
				e.emit("SC-CMP-001", func(f *model.Finding) {
					f.File = ctx.rel
					f.Evidence = "service \"" + svc.Name + "\" depends_on \"" + dep + "\" — not defined in this file"
				})
			}
		}
		for _, vol := range svc.Volumes {
			parts := strings.Split(vol, ":")
			if len(parts) < 2 {
				continue
			}
			src := parts[0]
			switch {
			case strings.HasPrefix(src, "$"):
				// interpolated; ENV-004 covers undeclared names
			case strings.HasPrefix(src, "/"):
				if dockerSockRe.MatchString(vol) {
					e.emit("SC-CMP-007", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\": " + vol
					})
					continue
				}
				for _, d := range dangerousHostPaths {
					if src == d || strings.HasPrefix(src, d+"/") {
						e.emit("SC-CMP-008", func(f *model.Finding) {
							f.File = ctx.rel
							f.Evidence = "service \"" + svc.Name + "\": " + vol
						})
						break
					}
				}
			case strings.HasPrefix(src, ".") || strings.HasPrefix(src, "~"):
				// Bind mount relative to compose dir.
				if strings.HasPrefix(src, "~") {
					continue
				}
				base := e.in.Root
				if ctx.dir != "" {
					base = filepath.Join(e.in.Root, filepath.FromSlash(ctx.dir))
				}
				abs, ok := fsutil.Resolve(base, src)
				if !ok {
					continue
				}
				if _, err := osStat(abs); err != nil {
					e.emit("SC-CMP-004", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\": bind source " + src + " does not exist"
					})
				}
			default:
				// Named volume.
				if !declared[src] {
					e.emit("SC-CMP-002", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\" mounts volume \"" + src + "\" — not declared under top-level volumes"
					})
				}
			}
		}
		for _, ef := range svc.EnvFiles {
			if strings.HasPrefix(ef, "$") {
				continue
			}
			base := e.in.Root
			if ctx.dir != "" {
				base = filepath.Join(e.in.Root, filepath.FromSlash(ctx.dir))
			}
			abs, ok := fsutil.Resolve(base, ef)
			if !ok {
				continue
			}
			if _, err := osStat(abs); err != nil {
				e.emit("SC-CMP-003", func(f *model.Finding) {
					f.File = ctx.rel
					f.Evidence = "service \"" + svc.Name + "\": env_file " + ef + " not found"
				})
			}
		}
		if svc.Build != nil {
			e.composeBuildChecks(ctx, svc)
		}
	}
}

func (e *engine) composeBuildChecks(ctx composeCtx, svc parse.ComposeService) {
	bctx := svc.Build.Context
	if bctx == "" || strings.HasPrefix(bctx, "$") {
		return
	}
	if strings.Contains(bctx, "://") {
		return // remote context: skipped by design
	}
	base := e.in.Root
	if ctx.dir != "" {
		base = filepath.Join(e.in.Root, filepath.FromSlash(ctx.dir))
	}
	abs, ok := fsutil.Resolve(base, bctx)
	if !ok {
		return
	}
	st, err := osStat(abs)
	if err != nil || !st.IsDir() {
		e.emit("SC-CTX-002", func(f *model.Finding) {
			f.File = ctx.rel
			f.Evidence = "service \"" + svc.Name + "\": build context " + bctx + " does not exist"
		})
		return
	}
	df := svc.Build.Dockerfile
	if df == "" {
		df = "Dockerfile"
	}
	if strings.HasPrefix(df, "$") {
		return
	}
	dfabs, ok := fsutil.Resolve(abs, df)
	if !ok {
		return
	}
	if _, err := osStat(dfabs); err != nil {
		e.emit("SC-CTX-003", func(f *model.Finding) {
			f.File = ctx.rel
			f.Evidence = "service \"" + svc.Name + "\": " + bctx + "/" + df + " not found"
		})
	}
}

func (e *engine) composeSecurity(ctx composeCtx) {
	for _, svc := range sortedServices(ctx.doc) {
		if svc.Privileged {
			e.emit("SC-CMP-005", func(f *model.Finding) {
				f.File = ctx.rel
				f.Evidence = "service \"" + svc.Name + "\""
			})
		}
		if svc.NetworkMode == "host" {
			e.emit("SC-CMP-006", func(f *model.Finding) {
				f.File = ctx.rel
				f.Evidence = "service \"" + svc.Name + "\""
			})
		}
	}
}

func (e *engine) composePortsAndEnv(ctx composeCtx) {
	// EXPOSE vs ports: gather container ports per Dockerfile dir.
	exposed := map[string][]int{} // dir -> exposed container ports
	for _, df := range e.in.Disc.Dockerfiles {
		c, ok := e.contentOf(df.RelPath)
		if !ok {
			continue
		}
		for _, inst := range parse.Dockerfile(c) {
			if inst.Cmd != "EXPOSE" {
				continue
			}
			for _, f := range strings.Fields(inst.Args) {
				if p := parse.ContainerPortOf(f); p > 0 {
					exposed[df.Dir] = append(exposed[df.Dir], p)
				}
			}
		}
	}
	for _, svc := range sortedServices(ctx.doc) {
		hostFlagged := false
		for _, p := range svc.Ports {
			if !strings.Contains(strings.Split(p, "/")[0], ":") && !strings.HasPrefix(p, "$") {
				// "8080" publishes nothing; it is container-side only.
				continue
			}
			if host := parse.PortInHostPart(p); host > 0 && !strings.HasPrefix(p, "$") && !interpolationRe.MatchString(p) {
				if !hostFlagged {
					e.emit("SC-PORT-001", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\": ports \"" + p + "\""
					})
					hostFlagged = true
				}
			}
			// Mismatch with the service's own Dockerfile EXPOSE.
			bdir := ""
			if svc.Build != nil {
				bdir = strings.TrimPrefix(svc.Build.Context, "./")
				if bdir == "." {
					bdir = ""
				}
			}
			if ports, ok := exposed[bdir]; ok && len(ports) > 0 {
				ctr := parse.ContainerPortOf(p)
				if ctr > 0 && !containsInt(ports, ctr) {
					e.emit("SC-PORT-002", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\": maps container port " + itoa(ctr) +
							" but its Dockerfile EXPOSEs " + intsJoin(ports)
					})
				}
			}
		}
	}
	// ENV-004: ${VAR} interpolation without declaration. Vars that carry an
	// inline default (${VAR:-x}) resolve deterministically and are skipped.
	// Compose environment keys are NOT counted as declarations here: an
	// entry like LOG_LEVEL=${LOG_LEVEL} consumes the variable rather than
	// declaring it. Only templates, Dockerfiles and env_files declare.
	declared := e.declaredNamesExceptCompose()
	found := map[string]bool{}
	for _, m := range interpolationRe.FindAllStringSubmatch(ctx.raw, -1) {
		name := m[1]
		if m[2] != "" && (strings.HasPrefix(m[2], ":-") || strings.HasPrefix(m[2], ":+")) {
			continue // inline default present
		}
		if found[name] || declared[name] {
			continue
		}
		found[name] = true
		e.emit("SC-ENV-004", func(f *model.Finding) {
			f.File = ctx.rel
			f.Evidence = "${" + name + "} used in compose file"
		})
	}
}

// declaredNames aggregates every env-var declaration site known so far.
func (e *engine) declaredNames() map[string]bool {
	out := map[string]bool{}
	if e.env == nil {
		return out
	}
	for name := range e.env.Declared {
		out[name] = true
	}
	return out
}

// declaredNamesExceptCompose excludes compose-environment declarations:
// they routinely pass through the same-named host variable.
func (e *engine) declaredNamesExceptCompose() map[string]bool {
	out := map[string]bool{}
	if e.env == nil {
		return out
	}
	for name, refs := range e.env.Declared {
		for _, r := range refs {
			if r.Kind != "compose" {
				out[name] = true
				break
			}
		}
	}
	return out
}

func (e *engine) composeHealthAndFS(ctxs []composeCtx) {
	anyVolumes := false
	for _, ctx := range ctxs {
		for _, svc := range ctx.doc.Services {
			if len(svc.Volumes) > 0 {
				anyVolumes = true
			}
		}
	}
	for _, df := range e.in.Disc.Dockerfiles {
		if c, ok := e.contentOf(df.RelPath); ok {
			if strings.Contains(c, "VOLUME ") || strings.Contains(c, "VOLUME\t") {
				anyVolumes = true
			}
		}
	}

	for _, ctx := range ctxs {
		for _, svc := range sortedServices(ctx.doc) {
			networked := len(svc.Ports) > 0 || len(svc.Expose) > 0
			if networked && svc.Healthcheck == nil {
				dockerHasHC := false
				if svc.Build != nil {
					bctx := strings.TrimPrefix(svc.Build.Context, "./")
					for _, df := range e.in.Disc.Dockerfiles {
						if df.Dir != bctx {
							continue
						}
						if c, ok := e.contentOf(df.RelPath); ok {
							for _, inst := range parse.Dockerfile(c) {
								if inst.Cmd == "HEALTHCHECK" {
									dockerHasHC = true
								}
							}
						}
					}
				}
				if !dockerHasHC {
					e.emit("SC-HLT-001", func(f *model.Finding) {
						f.File = ctx.rel
						f.Evidence = "service \"" + svc.Name + "\" has no healthcheck"
					})
				}
			}
		}
	}

	// FS-001: persistence evidence with no volumes anywhere.
	if anyVolumes || !e.enabled("SC-FS-001") {
		return
	}
	if e.persistEvidence() {
		e.emit("SC-FS-001", func(f *model.Finding) {
			f.Evidence = "persistence-related dependency or code pattern found; no compose volume or Dockerfile VOLUME declared"
		})
	}
}

var sqliteDepRe = regexp.MustCompile(`(?i)(sqlite3|better-sqlite3|sqlalchemy.*sqlite|sqlite)`)
var sqliteCodeRe = regexp.MustCompile(`(?i)(sqlite3\.connect|better-sqlite3|dialect['"]?\s*[:=]\s*['"]sqlite|sqlite://|sqlite:/)`)
var uploadsDirRe = regexp.MustCompile(`(?i)(uploads?|upload_dir|multer\()`)

func (e *engine) persistEvidence() bool {
	for _, app := range e.in.Disc.NodeApps {
		for dep := range app.Deps {
			if sqliteDepRe.MatchString(dep) {
				return true
			}
		}
	}
	for _, f := range e.in.Files {
		base := filepath.Base(f.RelPath)
		if strings.HasSuffix(base, ".go") || base == "go.mod" {
			continue // analyzing self-like Go repos: out of scope for node/python rules
		}
		if c, ok := e.contentOf(f.RelPath); ok {
			if sqliteCodeRe.MatchString(c) || uploadsDirRe.MatchString(c) {
				return true
			}
		}
	}
	return false
}

func sortedServices(doc *parse.Compose) []parse.ComposeService {
	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]parse.ComposeService, 0, len(names))
	for _, n := range names {
		out = append(out, doc.Services[n])
	}
	return out
}

func containsInt(xs []int, y int) bool {
	for _, x := range xs {
		if x == y {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func intsJoin(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = itoa(x)
	}
	return strings.Join(parts, ", ")
}
